package main

import (
	"context"
	"database/sql"
	"encoding/csv"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
)

// ---------- budgets ----------

type BudgetLine struct {
	Category string `json:"category"`
	Limit    *int64 `json:"limit"`
	Spent    int64  `json:"spent"`
	InBudget bool   `json:"in_budget"`
}

type Upcoming struct {
	RecurringID int64  `json:"recurring_id"`
	Date        string `json:"date"`
	Merchant    string `json:"merchant"`
	Category    string `json:"category"`
	Type        string `json:"type"`
	Amount      int64  `json:"amount"` // IDR, estimated at today's rate for foreign amounts
	Auto        bool   `json:"auto"`
}

func today() string { return time.Now().In(wib).Format(time.DateOnly) }

// budget returns limits vs spending for one month, plus bills still to come in it.
func (a *app) budget(w http.ResponseWriter, r *http.Request) {
	from, to, err := monthRange(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	lines := map[string]*BudgetLine{}
	var order []string
	line := func(cat string) *BudgetLine {
		if l := lines[cat]; l != nil {
			return l
		}
		l := &BudgetLine{Category: cat, InBudget: true}
		lines[cat] = l
		order = append(order, cat)
		return l
	}

	// effective limit = latest row at or before this month
	rows, err := a.db.Query(`SELECT b.category, b.amount FROM budgets b
		WHERE b.month = (SELECT MAX(month) FROM budgets WHERE category = b.category AND month <= ?) AND b.amount > 0
		ORDER BY b.amount DESC`, from)
	if err != nil {
		a.fail(w, err)
		return
	}
	var overall *int64
	for rows.Next() {
		var cat string
		var amt int64
		if err := rows.Scan(&cat, &amt); err != nil {
			rows.Close()
			a.fail(w, err)
			return
		}
		if cat == "" {
			overall = &amt
		} else {
			line(cat).Limit = &amt
		}
	}
	rows.Close()

	rows, err = a.db.Query(`SELECT t.category, SUM(t.amount), c.in_budget FROM transactions t
		JOIN categories c ON c.name = t.category AND c.type = t.type
		WHERE t.type = 'out' AND t.occurred_at >= ? AND t.occurred_at < ?
		GROUP BY t.category ORDER BY SUM(t.amount) DESC`, from, to)
	if err != nil {
		a.fail(w, err)
		return
	}
	var spent int64
	for rows.Next() {
		var cat string
		var sum int64
		var in bool
		if err := rows.Scan(&cat, &sum, &in); err != nil {
			rows.Close()
			a.fail(w, err)
			return
		}
		l := line(cat)
		l.Spent, l.InBudget = sum, in
		if in {
			spent += sum
		}
	}
	rows.Close()

	// excluded categories that only have a limit still need their flag
	for _, l := range lines {
		if l.Spent == 0 {
			a.db.QueryRow(`SELECT in_budget FROM categories WHERE type = 'out' AND name = ?`, l.Category).Scan(&l.InBudget)
		}
	}

	upcoming, err := a.upcoming(r.Context(), from, to)
	if err != nil {
		a.fail(w, err)
		return
	}
	var upTotal int64
	for _, u := range upcoming {
		if u.Type == "out" {
			upTotal += u.Amount
		}
	}

	daysLeft := 0
	if now := time.Now().In(wib); now.Format("2006-01") == from {
		end, _ := time.ParseInLocation("2006-01", to, wib)
		daysLeft = int(end.Sub(now).Hours()/24) + 1
	}
	out := make([]BudgetLine, 0, len(order))
	for _, c := range order {
		out = append(out, *lines[c])
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"month": from, "overall": overall, "spent": spent, "upcoming_total": upTotal,
		"days_left": daysLeft, "categories": out, "upcoming": upcoming,
	})
}

// setBudget body: {"month":"2026-11","category":"" (overall) | "<out category>","amount":3000000}; 0 removes it.
func (a *app) setBudget(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Month    string `json:"month"`
		Category string `json:"category"`
		Amount   int64  `json:"amount"`
	}
	if !decode(w, r, &b) {
		return
	}
	if _, err := time.Parse("2006-01", b.Month); err != nil {
		writeErr(w, http.StatusBadRequest, "month must be YYYY-MM")
		return
	}
	if b.Amount < 0 {
		writeErr(w, http.StatusBadRequest, "amount must be >= 0")
		return
	}
	if b.Category != "" && !a.categoryExists(b.Category, "out") {
		writeErr(w, http.StatusBadRequest, "unknown expense category")
		return
	}
	if _, err := a.db.Exec(`INSERT OR REPLACE INTO budgets (month, category, amount) VALUES (?, ?, ?)`, b.Month, b.Category, b.Amount); err != nil {
		a.fail(w, err)
		return
	}
	a.hub.publish("budget.changed", map[string]any{})
	w.WriteHeader(http.StatusNoContent)
}

// ---------- recurring ----------

type Recurring struct {
	ID         int64    `json:"id"`
	Type       string   `json:"type"`
	Category   string   `json:"category"`
	Merchant   string   `json:"merchant"`
	Account    string   `json:"account"`
	Note       string   `json:"note"`
	Amount     int64    `json:"amount"`
	Currency   string   `json:"currency"`
	OrigAmount *float64 `json:"orig_amount"`
	Freq       string   `json:"freq"`
	NextDate   string   `json:"next_date"`
	Auto       bool     `json:"auto"`
	day        int
}

const recCols = `id, type, category, merchant, account, note, amount, currency, orig_amount, freq, next_date, auto, day`

func scanRec(s scanner) (Recurring, error) {
	var x Recurring
	err := s.Scan(&x.ID, &x.Type, &x.Category, &x.Merchant, &x.Account, &x.Note, &x.Amount, &x.Currency, &x.OrigAmount,
		&x.Freq, &x.NextDate, &x.Auto, &x.day)
	return x, err
}

// nextDate steps one period, keeping the anchor day (31 -> last day of shorter months).
func nextDate(date, freq string, day int) string {
	t, _ := time.Parse(time.DateOnly, date)
	y, m := t.Year(), t.Month()
	switch freq {
	case "weekdays":
		t = t.AddDate(0, 0, 1)
		for t.Weekday() == time.Saturday || t.Weekday() == time.Sunday {
			t = t.AddDate(0, 0, 1)
		}
		return t.Format(time.DateOnly)
	case "weekly":
		return t.AddDate(0, 0, 7).Format(time.DateOnly)
	case "yearly":
		y++
	default:
		m++
	}
	last := time.Date(y, m+1, 0, 0, 0, 0, 0, time.UTC).Day()
	return time.Date(y, m, min(day, last), 0, 0, 0, 0, time.UTC).Format(time.DateOnly)
}

// tx builds the transaction for one occurrence; Amount 0 + foreign currency converts at that date's rate.
func (x Recurring) tx(date string) Tx {
	ref := fmt.Sprintf("recurring:%d:%s", x.ID, date)
	return Tx{OccurredAt: date + "T09:00:00+07:00", Amount: x.Amount, Type: x.Type, Category: x.Category,
		Merchant: x.Merchant, Account: x.Account, Note: x.Note, Source: "manual", SourceRef: &ref,
		Currency: x.Currency, OrigAmount: x.OrigAmount}
}

func (a *app) recurrings(ctx context.Context, where string, args ...any) ([]Recurring, error) {
	rows, err := a.db.QueryContext(ctx, `SELECT `+recCols+` FROM recurring `+where+` ORDER BY next_date, id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Recurring{}
	for rows.Next() {
		x, err := scanRec(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// upcoming lists occurrences not yet posted in [from, to) months; for the current month overdue ones are included.
func (a *app) upcoming(ctx context.Context, from, to string) ([]Upcoming, error) {
	recs, err := a.recurrings(ctx, "")
	if err != nil {
		return nil, err
	}
	start := from + "-01"
	if from == time.Now().In(wib).Format("2006-01") {
		start = "0000-00-00"
	}
	end := to + "-01"
	out := []Upcoming{}
	for _, x := range recs {
		amt := x.Amount
		if amt == 0 && x.OrigAmount != nil {
			if rate, err := a.idrRate(ctx, x.Currency, ""); err == nil {
				amt = int64(math.Round(*x.OrigAmount * rate))
			}
		}
		for d := x.NextDate; d < end; d = nextDate(d, x.Freq, x.day) {
			if d >= start {
				out = append(out, Upcoming{x.ID, d, x.Merchant, x.Category, x.Type, amt, x.Auto})
			}
		}
	}
	slices.SortStableFunc(out, func(x, y Upcoming) int { return strings.Compare(x.Date, y.Date) })
	return out, nil
}

func (a *app) listRecurring(w http.ResponseWriter, r *http.Request) {
	recs, err := a.recurrings(r.Context(), "")
	if err != nil {
		a.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, recs)
}

// checkRec validates x by building its first occurrence as a transaction.
func (a *app) checkRec(w http.ResponseWriter, r *http.Request, x *Recurring) bool {
	if !slices.Contains([]string{"weekdays", "weekly", "monthly", "yearly"}, x.Freq) {
		writeErr(w, http.StatusBadRequest, "freq must be weekdays, weekly, monthly or yearly")
		return false
	}
	d, err := time.Parse(time.DateOnly, x.NextDate)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "next_date must be YYYY-MM-DD")
		return false
	}
	if x.Freq == "weekdays" && (d.Weekday() == time.Saturday || d.Weekday() == time.Sunday) {
		x.NextDate = nextDate(x.NextDate, "weekdays", 0) // a weekend start moves to Monday
	}
	t := x.tx(x.NextDate)
	if !a.validateTx(w, r, &t) {
		return false
	}
	x.Merchant, x.Account, x.Note, x.Currency, x.OrigAmount = t.Merchant, t.Account, t.Note, t.Currency, t.OrigAmount
	if x.Currency == "IDR" {
		x.Amount = t.Amount
	}
	x.day, _ = strconv.Atoi(x.NextDate[8:])
	return true
}

func (a *app) createRecurring(w http.ResponseWriter, r *http.Request) {
	var x Recurring
	if !decode(w, r, &x) || !a.checkRec(w, r, &x) {
		return
	}
	res, err := a.db.Exec(`INSERT INTO recurring (type, category, merchant, account, note, amount, currency, orig_amount, freq, day, next_date, auto)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, x.Type, x.Category, x.Merchant, x.Account, x.Note, x.Amount, x.Currency,
		x.OrigAmount, x.Freq, x.day, x.NextDate, x.Auto)
	if err != nil {
		a.fail(w, err)
		return
	}
	x.ID, _ = res.LastInsertId()
	a.postDue(r.Context()) // an auto item dated today or earlier posts right away
	a.hub.publish("budget.changed", map[string]any{})
	writeJSON(w, http.StatusCreated, x)
}

func (a *app) getRec(w http.ResponseWriter, r *http.Request) (Recurring, bool) {
	id, ok := pathID(w, r)
	if !ok {
		return Recurring{}, false
	}
	x, err := scanRec(a.db.QueryRow(`SELECT `+recCols+` FROM recurring WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, http.StatusNotFound, "not found")
		return x, false
	} else if err != nil {
		a.fail(w, err)
		return x, false
	}
	return x, true
}

func (a *app) patchRecurring(w http.ResponseWriter, r *http.Request) {
	x, ok := a.getRec(w, r)
	if !ok || !decode(w, r, &x) || !a.checkRec(w, r, &x) {
		return
	}
	_, err := a.db.Exec(`UPDATE recurring SET type = ?, category = ?, merchant = ?, account = ?, note = ?, amount = ?, currency = ?,
		orig_amount = ?, freq = ?, day = ?, next_date = ?, auto = ? WHERE id = ?`, x.Type, x.Category, x.Merchant, x.Account, x.Note,
		x.Amount, x.Currency, x.OrigAmount, x.Freq, x.day, x.NextDate, x.Auto, x.ID)
	if err != nil {
		a.fail(w, err)
		return
	}
	a.postDue(r.Context())
	a.hub.publish("budget.changed", map[string]any{})
	writeJSON(w, http.StatusOK, x)
}

func (a *app) deleteRecurring(w http.ResponseWriter, r *http.Request) {
	x, ok := a.getRec(w, r)
	if !ok {
		return
	}
	if _, err := a.db.Exec(`DELETE FROM recurring WHERE id = ?`, x.ID); err != nil {
		a.fail(w, err)
		return
	}
	a.hub.publish("budget.changed", map[string]any{})
	w.WriteHeader(http.StatusNoContent)
}

// advanceRecurring handles the next occurrence: records it (default) or skips it with ?skip=1.
func (a *app) advanceRecurring(w http.ResponseWriter, r *http.Request) {
	x, ok := a.getRec(w, r)
	if !ok {
		return
	}
	if r.URL.Query().Get("skip") == "" {
		if err := a.postOccurrence(r.Context(), x); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if _, err := a.db.Exec(`UPDATE recurring SET next_date = ? WHERE id = ?`, nextDate(x.NextDate, x.Freq, x.day), x.ID); err != nil {
		a.fail(w, err)
		return
	}
	a.hub.publish("budget.changed", map[string]any{})
	w.WriteHeader(http.StatusNoContent)
}

// postOccurrence records x's next occurrence; already-recorded occurrences are ignored.
func (a *app) postOccurrence(ctx context.Context, x Recurring) error {
	t := x.tx(x.NextDate)
	if body := a.checkTx(ctx, &t); body != nil {
		return fmt.Errorf("%v", body["error"])
	}
	if err := a.insertTx(&t); err != nil && !strings.Contains(err.Error(), "UNIQUE") {
		return err
	}
	return nil
}

// postDue records every auto occurrence dated today or earlier.
func (a *app) postDue(ctx context.Context) {
	recs, err := a.recurrings(ctx, `WHERE auto = 1 AND next_date <= ?`, today())
	if err != nil {
		slog.Error("recurring", "err", err)
		return
	}
	for _, x := range recs {
		for x.NextDate <= today() {
			if err := a.postOccurrence(ctx, x); err != nil {
				slog.Error("recurring post", "id", x.ID, "err", err) // retried next hour
				break
			}
			x.NextDate = nextDate(x.NextDate, x.Freq, x.day)
			if _, err := a.db.ExecContext(ctx, `UPDATE recurring SET next_date = ? WHERE id = ?`, x.NextDate, x.ID); err != nil {
				slog.Error("recurring advance", "id", x.ID, "err", err)
				break
			}
		}
	}
}

// ---------- export ----------

func (a *app) exportCSV(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(`SELECT ` + txCols + ` FROM transactions ORDER BY occurred_at, id`)
	if err != nil {
		a.fail(w, err)
		return
	}
	defer rows.Close()
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="kantong-`+today()+`.csv"`)
	c := csv.NewWriter(w)
	c.Write([]string{"date", "type", "category", "merchant", "account", "amount_idr", "currency", "orig_amount", "note", "source"})
	for rows.Next() {
		t, err := scanTx(rows)
		if err != nil {
			slog.Error("export", "err", err)
			break
		}
		orig := ""
		if t.OrigAmount != nil {
			orig = strconv.FormatFloat(*t.OrigAmount, 'f', -1, 64)
		}
		c.Write([]string{t.OccurredAt, t.Type, t.Category, csvSafe(t.Merchant), csvSafe(t.Account), strconv.FormatInt(t.Amount, 10),
			t.Currency, orig, csvSafe(t.Note), t.Source})
	}
	c.Flush()
}

// csvSafe stops spreadsheet apps from running email-derived text as a formula.
func csvSafe(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}
