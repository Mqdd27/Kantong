package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Tx struct {
	ID          int64    `json:"id"`
	OccurredAt  string   `json:"occurred_at"`
	Amount      int64    `json:"amount"`
	Type        string   `json:"type"`
	Category    string   `json:"category"`
	Merchant    string   `json:"merchant"`
	Account     string   `json:"account"`
	ToAccount   string   `json:"to_account"` // transfers only: destination account, '' = leaves your accounts
	Note        string   `json:"note"`
	Source      string   `json:"source"`
	SourceRef   *string  `json:"source_ref"`
	Confidence  *float64 `json:"confidence"`
	NeedsReview bool     `json:"needs_review"`
	Currency    string   `json:"currency"`    // default IDR
	OrigAmount  *float64 `json:"orig_amount"` // amount in Currency when not IDR; Amount is then its IDR value
}

const txCols = `id, occurred_at, amount, type, category, merchant, account, to_account, note, source, source_ref, confidence, needs_review, currency, orig_amount`

type scanner interface{ Scan(...any) error }

func scanTx(s scanner) (Tx, error) {
	var t Tx
	err := s.Scan(&t.ID, &t.OccurredAt, &t.Amount, &t.Type, &t.Category, &t.Merchant, &t.Account, &t.ToAccount,
		&t.Note, &t.Source, &t.SourceRef, &t.Confidence, &t.NeedsReview, &t.Currency, &t.OrigAmount)
	return t, err
}

type Category struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type Rule struct {
	ID       int64  `json:"id"`
	Pattern  string `json:"pattern"`
	Match    string `json:"match"`
	Type     string `json:"type"`
	Category string `json:"category"`
}

// ---------- helpers ----------

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return false
	}
	return true
}

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
	}
	return id, err == nil
}

// monthRange returns [start, end) as "YYYY-MM" strings; they compare correctly
// against occurred_at because every stored timestamp is RFC3339 in +07:00.
func monthRange(r *http.Request) (string, string, error) {
	m := r.URL.Query().Get("month")
	if m == "" {
		m = time.Now().In(wib).Format("2006-01")
	}
	t, err := time.Parse("2006-01", m)
	if err != nil {
		return "", "", errors.New("month must be YYYY-MM")
	}
	return t.Format("2006-01"), t.AddDate(0, 1, 0).Format("2006-01"), nil
}

func validType(t string) bool { return t == "in" || t == "out" || t == "transfer" }

func (a *app) categoryExists(name, typ string) bool {
	var n int
	a.db.QueryRow(`SELECT COUNT(*) FROM categories WHERE name = ? AND type = ?`, name, typ).Scan(&n)
	return n > 0
}

func (a *app) categories() ([]Category, error) {
	rows, err := a.db.Query(`SELECT name, type FROM categories ORDER BY type, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Category{}
	for rows.Next() {
		var c Category
		if err := rows.Scan(&c.Name, &c.Type); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// validateTx normalizes and checks t; it writes the error response and returns false on failure.
func (a *app) validateTx(w http.ResponseWriter, r *http.Request, t *Tx) bool {
	if body := a.checkTx(r.Context(), t); body != nil {
		writeJSON(w, http.StatusBadRequest, body)
		return false
	}
	return true
}

func badTx(msg string) map[string]any { return map[string]any{"error": msg} }

// checkTx normalizes t and returns an error body, or nil when t is valid.
func (a *app) checkTx(ctx context.Context, t *Tx) map[string]any {
	if t.OccurredAt == "" {
		t.OccurredAt = time.Now().In(wib).Format(time.RFC3339)
	}
	ts, err := time.Parse(time.RFC3339, t.OccurredAt)
	if err != nil {
		return badTx("occurred_at must be ISO 8601 / RFC3339")
	}
	t.OccurredAt = ts.In(wib).Format(time.RFC3339)
	t.Merchant, t.Account, t.Note = strings.TrimSpace(t.Merchant), strings.TrimSpace(t.Account), strings.TrimSpace(t.Note)
	if t.ToAccount = strings.TrimSpace(t.ToAccount); t.Type != "transfer" {
		t.ToAccount = ""
	}

	var ok bool
	if t.Currency, ok = normCurrency(t.Currency); !ok {
		return badTx("currency must be a 3-letter ISO code, e.g. USD")
	}
	if t.Currency == "IDR" {
		t.OrigAmount = nil
	} else {
		if t.OrigAmount == nil || *t.OrigAmount <= 0 {
			return badTx("orig_amount (amount in currency) is required when currency is not IDR")
		}
		if t.Amount <= 0 { // no charged IDR amount given: convert at that day's rate
			rate, err := a.idrRate(ctx, t.Currency, t.OccurredAt[:10])
			if err != nil {
				return badTx(err.Error())
			}
			t.Amount = int64(math.Round(*t.OrigAmount * rate))
		}
	}

	var msg string
	switch {
	case t.Amount <= 0:
		msg = "amount must be a positive integer"
	case !validType(t.Type):
		msg = "type must be in, out or transfer"
	case t.Source != "email" && t.Source != "manual":
		msg = "source must be email or manual"
	case t.Source == "email" && (t.SourceRef == nil || *t.SourceRef == ""):
		msg = "source_ref is required for source=email"
	case t.Confidence != nil && (*t.Confidence < 0 || *t.Confidence > 1):
		msg = "confidence must be between 0 and 1"
	}
	if msg != "" {
		return badTx(msg)
	}
	if !a.categoryExists(t.Category, t.Type) {
		cats, _ := a.categories()
		return map[string]any{"error": fmt.Sprintf("unknown category %q for type %q", t.Category, t.Type), "valid": cats}
	}
	if t.SourceRef != nil && *t.SourceRef == "" {
		t.SourceRef = nil
	}
	if t.Confidence != nil && *t.Confidence < 0.7 {
		t.NeedsReview = true
	}
	return nil
}

func (a *app) getTx(id int64) (Tx, error) {
	return scanTx(a.db.QueryRow(`SELECT `+txCols+` FROM transactions WHERE id = ?`, id))
}

// ---------- transactions ----------

func (a *app) createTx(w http.ResponseWriter, r *http.Request) {
	var t Tx
	if !decode(w, r, &t) {
		return
	}
	if t.Source == "email" { // rules win even if the agent forgot to apply them
		a.applyRule(&t)
	}
	if !a.validateTx(w, r, &t) {
		return
	}
	if err := a.insertTx(&t); err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed: transactions.source_ref") {
			var id int64
			a.db.QueryRow(`SELECT id FROM transactions WHERE source_ref = ?`, *t.SourceRef).Scan(&id)
			writeJSON(w, http.StatusConflict, map[string]any{"error": "duplicate source_ref", "id": id})
			return
		}
		a.fail(w, err)
		return
	}
	if t.Source == "email" {
		a.notify(t)
	}
	writeJSON(w, http.StatusCreated, t) // includes id, plus the computed IDR amount
}

// insertTx stores a validated t, sets its ID and notifies clients.
func (a *app) insertTx(t *Tx) error {
	res, err := a.db.Exec(`INSERT INTO transactions
		(occurred_at, amount, type, category, merchant, account, to_account, note, source, source_ref, confidence, needs_review, currency, orig_amount)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.OccurredAt, t.Amount, t.Type, t.Category, t.Merchant, t.Account, t.ToAccount, t.Note, t.Source, t.SourceRef, t.Confidence, t.NeedsReview,
		t.Currency, t.OrigAmount)
	if err != nil {
		return err
	}
	t.ID, _ = res.LastInsertId()
	a.hub.publish("transaction.created", *t)
	return nil
}

func (a *app) listTx(w http.ResponseWriter, r *http.Request) {
	from, to, err := monthRange(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var rows *sql.Rows
	if q := strings.TrimSpace(r.URL.Query().Get("q")); q != "" { // search all months
		like := "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(q) + "%"
		rows, err = a.db.Query(`SELECT `+txCols+` FROM transactions
			WHERE merchant LIKE ?1 ESCAPE '\' OR note LIKE ?1 ESCAPE '\' OR category LIKE ?1 ESCAPE '\' OR account LIKE ?1 ESCAPE '\'
			ORDER BY occurred_at DESC, id DESC LIMIT 200`, like)
	} else {
		rows, err = a.db.Query(`SELECT `+txCols+` FROM transactions
			WHERE occurred_at >= ? AND occurred_at < ? ORDER BY occurred_at DESC, id DESC`, from, to)
	}
	if err != nil {
		a.fail(w, err)
		return
	}
	defer rows.Close()
	out := []Tx{}
	for rows.Next() {
		t, err := scanTx(rows)
		if err != nil {
			a.fail(w, err)
			return
		}
		out = append(out, t)
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *app) patchTx(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var p struct {
		OccurredAt  *string  `json:"occurred_at"`
		Amount      *int64   `json:"amount"`
		Type        *string  `json:"type"`
		Category    *string  `json:"category"`
		Merchant    *string  `json:"merchant"`
		Account     *string  `json:"account"`
		ToAccount   *string  `json:"to_account"`
		Note        *string  `json:"note"`
		NeedsReview *bool    `json:"needs_review"`
		Currency    *string  `json:"currency"`
		OrigAmount  *float64 `json:"orig_amount"`
	}
	if !decode(w, r, &p) {
		return
	}
	t, err := a.getTx(id)
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, http.StatusNotFound, "not found")
		return
	} else if err != nil {
		a.fail(w, err)
		return
	}
	set := func(dst *string, src *string) {
		if src != nil {
			*dst = *src
		}
	}
	set(&t.OccurredAt, p.OccurredAt)
	set(&t.Type, p.Type)
	set(&t.Category, p.Category)
	set(&t.Merchant, p.Merchant)
	set(&t.Account, p.Account)
	set(&t.ToAccount, p.ToAccount)
	set(&t.Note, p.Note)
	set(&t.Currency, p.Currency)
	if p.OrigAmount != nil {
		t.OrigAmount = p.OrigAmount
	}
	if p.Amount != nil {
		t.Amount = *p.Amount
	} else if c, _ := normCurrency(t.Currency); c != "IDR" && (p.Currency != nil || p.OrigAmount != nil || p.OccurredAt != nil) {
		t.Amount = 0 // foreign amount, currency or date changed: recompute the IDR value
	}
	conf := t.Confidence
	t.Confidence = nil // don't let old low confidence re-flag a manual edit
	if !a.validateTx(w, r, &t) {
		return
	}
	t.Confidence = conf
	if p.NeedsReview != nil {
		t.NeedsReview = *p.NeedsReview
	}
	_, err = a.db.Exec(`UPDATE transactions SET occurred_at = ?, amount = ?, type = ?, category = ?, merchant = ?,
		account = ?, to_account = ?, note = ?, needs_review = ?, currency = ?, orig_amount = ?,
		updated_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now') WHERE id = ?`,
		t.OccurredAt, t.Amount, t.Type, t.Category, t.Merchant, t.Account, t.ToAccount, t.Note, t.NeedsReview, t.Currency, t.OrigAmount, id)
	if err != nil {
		a.fail(w, err)
		return
	}
	a.hub.publish("transaction.updated", t)
	writeJSON(w, http.StatusOK, t)
}

func (a *app) deleteTx(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	t, err := a.getTx(id)
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, http.StatusNotFound, "not found")
		return
	} else if err != nil {
		a.fail(w, err)
		return
	}
	if _, err := a.db.Exec(`DELETE FROM transactions WHERE id = ?`, id); err != nil {
		a.fail(w, err)
		return
	}
	a.hub.publish("transaction.deleted", t)
	w.WriteHeader(http.StatusNoContent)
}

func (a *app) checkRefs(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Refs []string `json:"refs"`
	}
	if !decode(w, r, &body) {
		return
	}
	if len(body.Refs) > 1000 {
		writeErr(w, http.StatusBadRequest, "max 1000 refs")
		return
	}
	unknown := []string{}
	for _, ref := range body.Refs {
		var n int
		if err := a.db.QueryRow(`SELECT COUNT(*) FROM transactions WHERE source_ref = ?`, ref).Scan(&n); err != nil {
			a.fail(w, err)
			return
		}
		if n == 0 {
			unknown = append(unknown, ref)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"unknown": unknown})
}

func (a *app) summary(w http.ResponseWriter, r *http.Request) {
	from, to, err := monthRange(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	rows, err := a.db.Query(`SELECT type, category, SUM(amount), COUNT(*) FROM transactions
		WHERE occurred_at >= ? AND occurred_at < ? GROUP BY type, category ORDER BY SUM(amount) DESC`, from, to)
	if err != nil {
		a.fail(w, err)
		return
	}
	defer rows.Close()
	type line struct {
		Type     string `json:"type"`
		Category string `json:"category"`
		Total    int64  `json:"total"`
		Count    int64  `json:"count"`
	}
	var in, out int64
	lines := []line{}
	for rows.Next() {
		var l line
		if err := rows.Scan(&l.Type, &l.Category, &l.Total, &l.Count); err != nil {
			a.fail(w, err)
			return
		}
		switch l.Type {
		case "in":
			in += l.Total
		case "out":
			out += l.Total
		}
		lines = append(lines, l)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"month": from, "in": in, "out": out, "net": in - out, "by_category": lines,
	})
}

func (a *app) listCategories(w http.ResponseWriter, _ *http.Request) {
	cats, err := a.categories()
	if err != nil {
		a.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, cats)
}

// ---------- rules ----------

func (a *app) listRules(w http.ResponseWriter, _ *http.Request) {
	rows, err := a.db.Query(`SELECT id, pattern, match, type, category FROM rules ORDER BY id`)
	if err != nil {
		a.fail(w, err)
		return
	}
	defer rows.Close()
	out := []Rule{}
	for rows.Next() {
		var ru Rule
		if err := rows.Scan(&ru.ID, &ru.Pattern, &ru.Match, &ru.Type, &ru.Category); err != nil {
			a.fail(w, err)
			return
		}
		out = append(out, ru)
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *app) createRule(w http.ResponseWriter, r *http.Request) {
	var ru Rule
	if !decode(w, r, &ru) {
		return
	}
	ru.Pattern = strings.TrimSpace(ru.Pattern)
	if ru.Match == "" {
		ru.Match = "contains"
	}
	switch {
	case ru.Pattern == "":
		writeErr(w, http.StatusBadRequest, "pattern is required")
		return
	case ru.Match != "contains":
		writeErr(w, http.StatusBadRequest, "match must be contains")
		return
	case !validType(ru.Type) || !a.categoryExists(ru.Category, ru.Type):
		writeErr(w, http.StatusBadRequest, "unknown type/category, see /api/categories")
		return
	}
	// one rule per pattern: a newer correction replaces the old one
	a.db.Exec(`DELETE FROM rules WHERE lower(pattern) = lower(?)`, ru.Pattern)
	res, err := a.db.Exec(`INSERT INTO rules (pattern, match, type, category) VALUES (?, ?, ?, ?)`,
		ru.Pattern, ru.Match, ru.Type, ru.Category)
	if err != nil {
		a.fail(w, err)
		return
	}
	ru.ID, _ = res.LastInsertId()
	writeJSON(w, http.StatusCreated, ru)
}

func (a *app) deleteRule(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	res, err := a.db.Exec(`DELETE FROM rules WHERE id = ?`, id)
	if err != nil {
		a.fail(w, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *app) fail(w http.ResponseWriter, err error) {
	slog.Error("internal error", "err", err)
	writeErr(w, http.StatusInternalServerError, "internal error")
}
