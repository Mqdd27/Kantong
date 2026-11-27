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
	"unicode"
	"unicode/utf8"
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
	SplitOf     *int64   `json:"split_of"`    // id of the original row when this is part of a split
	CreatedBy   *int64   `json:"created_by"`  // account that entered it; null once that account is gone
	uid         int64    // account the row belongs to: its author, or a kantong bersama's holder after the author left
}

const txCols = `id, occurred_at, amount, type, category, merchant, account, to_account, note, source, source_ref, confidence, needs_review, currency, orig_amount, split_of, user_id, created_by`

// txScope limits transactions to the ones the caller may see and change: their own plus those of
// any kantong bersama they belong to (kantong_tx). Arg: @uid.
const txScope = `(user_id = @uid OR id IN (SELECT k.tx_id FROM kantong_tx k JOIN shared_members m ON m.shared_id = k.shared_id WHERE m.user_id = @uid))`

type scanner interface{ Scan(...any) error }

func scanTx(s scanner) (Tx, error) {
	var t Tx
	err := s.Scan(&t.ID, &t.OccurredAt, &t.Amount, &t.Type, &t.Category, &t.Merchant, &t.Account, &t.ToAccount,
		&t.Note, &t.Source, &t.SourceRef, &t.Confidence, &t.NeedsReview, &t.Currency, &t.OrigAmount, &t.SplitOf, &t.uid, &t.CreatedBy)
	return t, err
}

type Category struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Type   string `json:"type"`
	Custom bool   `json:"custom"` // user-created: can be renamed
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

func (a *app) categoryExists(uid int64, name, typ string) bool {
	var n int
	a.db.QueryRow(`SELECT COUNT(*) FROM categories WHERE user_id = ? AND name = ? AND type = ?`, uid, name, typ).Scan(&n)
	return n > 0
}

func (a *app) categories(uid int64) ([]Category, error) {
	rows, err := a.db.Query(`SELECT id, name, type, custom FROM categories WHERE user_id = ? ORDER BY type, id`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Category{}
	for rows.Next() {
		var c Category
		if err := rows.Scan(&c.ID, &c.Name, &c.Type, &c.Custom); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// validateTx normalizes and checks t; it writes the error response and returns false on failure.
// uid is the account the row belongs to, whose categories apply.
func (a *app) validateTx(w http.ResponseWriter, r *http.Request, uid int64, t *Tx) bool {
	if body := a.checkTx(r.Context(), uid, t); body != nil {
		writeJSON(w, http.StatusBadRequest, body)
		return false
	}
	return true
}

func badTx(msg string) map[string]any { return map[string]any{"error": msg} }

// checkTx normalizes t and returns an error body, or nil when t is valid.
func (a *app) checkTx(ctx context.Context, uid int64, t *Tx) map[string]any {
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
	if !a.categoryExists(uid, t.Category, t.Type) {
		cats, _ := a.categories(uid)
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

func (a *app) getTx(uid, id int64) (Tx, error) {
	return scanTx(a.db.QueryRow(`SELECT `+txCols+` FROM transactions WHERE id = @id AND `+txScope, sql.Named("id", id), sql.Named("uid", uid)))
}

// ---------- transactions ----------

func (a *app) createTx(w http.ResponseWriter, r *http.Request) {
	var t Tx
	if !decode(w, r, &t) {
		return
	}
	uid := userID(r)
	if t.Source == "email" { // rules win even if the agent forgot to apply them
		a.applyRule(uid, &t)
	}
	// An email can't tell a move between own accounts from a payment to a person (friend, split bill),
	// so a transfer without a destination waits in Review instead of silently leaving the books.
	if t.Source == "email" && t.Type == "transfer" && strings.TrimSpace(t.ToAccount) == "" {
		t.NeedsReview = true
	}
	if !a.validateTx(w, r, uid, &t) {
		return
	}
	if err := a.insertTx(uid, &t); err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") && strings.Contains(err.Error(), "transactions.source_ref") {
			var id int64
			a.db.QueryRow(`SELECT id FROM transactions WHERE user_id = ? AND source_ref = ?`, uid, *t.SourceRef).Scan(&id)
			writeJSON(w, http.StatusConflict, map[string]any{"error": "duplicate source_ref", "id": id})
			return
		}
		a.fail(w, err)
		return
	}
	if t.Source == "email" {
		a.notify(uid, t)
	}
	writeJSON(w, http.StatusCreated, t) // includes id, plus the computed IDR amount
}

// insertTx stores a validated t entered by uid, sets its ID and notifies clients.
func (a *app) insertTx(uid int64, t *Tx) error {
	res, err := a.db.Exec(`INSERT INTO transactions
		(user_id, created_by, occurred_at, amount, type, category, merchant, account, to_account, note, source, source_ref, confidence, needs_review, currency, orig_amount)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		uid, uid, t.OccurredAt, t.Amount, t.Type, t.Category, t.Merchant, t.Account, t.ToAccount, t.Note, t.Source, t.SourceRef, t.Confidence, t.NeedsReview,
		t.Currency, t.OrigAmount)
	if err != nil {
		return err
	}
	t.ID, _ = res.LastInsertId()
	t.uid, t.CreatedBy = uid, &uid
	a.publishTx(uid, "transaction.created", *t)
	return nil
}

func (a *app) listTx(w http.ResponseWriter, r *http.Request) {
	from, to, err := monthRange(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var rows *sql.Rows
	uid := userID(r)
	if q := strings.TrimSpace(r.URL.Query().Get("q")); q != "" { // search all months
		like := "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(q) + "%"
		rows, err = a.db.Query(`SELECT `+txCols+` FROM transactions WHERE `+txScope+` AND
			(merchant LIKE @q ESCAPE '\' OR note LIKE @q ESCAPE '\' OR category LIKE @q ESCAPE '\' OR account LIKE @q ESCAPE '\')
			ORDER BY occurred_at DESC, id DESC LIMIT 200`, sql.Named("q", like), sql.Named("uid", uid))
	} else {
		rows, err = a.db.Query(`SELECT `+txCols+` FROM transactions
			WHERE `+txScope+` AND occurred_at >= @from AND occurred_at < @to ORDER BY occurred_at DESC, id DESC`,
			sql.Named("uid", uid), sql.Named("from", from), sql.Named("to", to))
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
	t, err := a.getTx(userID(r), id)
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, http.StatusNotFound, "not found")
		return
	} else if err != nil {
		a.fail(w, err)
		return
	}
	uid := t.uid // a kantong member works on another member's entry as that entry's account
	orig := t
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
	if !a.validateTx(w, r, uid, &t) {
		return
	}
	if uid != userID(r) && (t.Account != orig.Account || t.ToAccount != orig.ToAccount) {
		writeErr(w, http.StatusBadRequest, "only the author can move this transaction to another account")
		return
	}
	t.Confidence = conf
	if p.NeedsReview != nil {
		t.NeedsReview = *p.NeedsReview
	}
	_, err = a.db.Exec(`UPDATE transactions SET occurred_at = ?, amount = ?, type = ?, category = ?, merchant = ?,
		account = ?, to_account = ?, note = ?, needs_review = ?, currency = ?, orig_amount = ?,
		updated_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now') WHERE id = ? AND user_id = ?`,
		t.OccurredAt, t.Amount, t.Type, t.Category, t.Merchant, t.Account, t.ToAccount, t.Note, t.NeedsReview, t.Currency, t.OrigAmount, id, uid)
	if err != nil {
		a.fail(w, err)
		return
	}
	a.publishTx(uid, "transaction.updated", t)
	writeJSON(w, http.StatusOK, t)
}

func (a *app) deleteTx(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	t, err := a.getTx(userID(r), id)
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, http.StatusNotFound, "not found")
		return
	} else if err != nil {
		a.fail(w, err)
		return
	}
	uid := t.uid // a kantong member works on another member's entry as that entry's account
	// one transaction: a failure halfway must not leave a split with no head or a lost email ref
	tx, err := a.db.Begin()
	if err != nil {
		a.fail(w, err)
		return
	}
	defer tx.Rollback()
	var siblings []int64
	if t.SplitOf != nil {
		rows, err := tx.Query(`SELECT id FROM transactions WHERE split_of = ? AND id != ?`, *t.SplitOf, t.ID)
		if err != nil {
			a.fail(w, err)
			return
		}
		for rows.Next() {
			var sid int64
			rows.Scan(&sid)
			siblings = append(siblings, sid)
		}
		rows.Close()
	}
	if _, err := tx.Exec(`DELETE FROM transactions WHERE id = ? AND user_id = ?`, id, uid); err != nil {
		a.fail(w, err)
		return
	}
	if t.SplitOf != nil {
		kept, err := unsplitAfterDelete(tx, t)
		if err != nil {
			a.fail(w, err)
			return
		}
		if kept {
			t.SourceRef = nil // a remaining part now carries the email's ref
		}
	}
	if t.SourceRef != nil && *t.SourceRef != "" {
		// otherwise the email cron sees the Message-ID as new again and re-imports it
		if _, err := tx.Exec(`INSERT OR IGNORE INTO deleted_refs (user_id, ref) VALUES (?, ?)`, uid, *t.SourceRef); err != nil {
			a.fail(w, err)
			return
		}
	}
	if err := tx.Commit(); err != nil {
		a.fail(w, err)
		return
	}
	for _, sid := range siblings { // their split_of / source_ref may have changed
		if x, err := a.getTx(uid, sid); err == nil {
			a.publishTx(uid, "transaction.updated", x)
		}
	}
	a.publishTx(uid, "transaction.deleted", t)
	w.WriteHeader(http.StatusNoContent)
}

// splitTx spreads a transaction (or the split it belongs to) over several categories:
// {"parts": [{"category": "Jajan", "amount": 35000, "note": "..."}, ...]}. The parts must add up to the total.
// The original row becomes the first part and keeps its source_ref; one part merges a split back into one row.
func (a *app) splitTx(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var b struct {
		Parts []struct {
			Category string  `json:"category"`
			Amount   int64   `json:"amount"`
			Note     *string `json:"note"`
		} `json:"parts"`
	}
	if !decode(w, r, &b) {
		return
	}
	t, err := a.getTx(userID(r), id)
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, http.StatusNotFound, "not found")
		return
	} else if err != nil {
		a.fail(w, err)
		return
	}
	uid := t.uid // a kantong member works on another member's entry as that entry's account
	head := t
	if t.SplitOf != nil && *t.SplitOf != t.ID {
		if head, err = a.getTx(uid, *t.SplitOf); err != nil { // same owner as t
			a.fail(w, err)
			return
		}
	}
	var total int64
	if err := a.db.QueryRow(`SELECT SUM(amount) FROM transactions WHERE id = ?1 OR split_of = ?1`, head.ID).Scan(&total); err != nil {
		a.fail(w, err)
		return
	}
	var sum int64
	for _, p := range b.Parts {
		if p.Amount <= 0 {
			writeErr(w, http.StatusBadRequest, "every part needs a positive amount")
			return
		}
		if !a.categoryExists(uid, p.Category, head.Type) {
			writeErr(w, http.StatusBadRequest, fmt.Sprintf("unknown category %q for type %q", p.Category, head.Type))
			return
		}
		sum += p.Amount
	}
	switch {
	case head.Type == "transfer":
		writeErr(w, http.StatusBadRequest, "transfers can't be split")
		return
	case head.Currency != "IDR": // ponytail: IDR only, split orig_amount pro rata if foreign splits are ever needed
		writeErr(w, http.StatusBadRequest, "only IDR transactions can be split")
		return
	case len(b.Parts) == 0:
		writeErr(w, http.StatusBadRequest, "parts is required")
		return
	case sum != total:
		writeErr(w, http.StatusBadRequest, fmt.Sprintf("parts add up to %d, the transaction is %d", sum, total))
		return
	}

	tx, err := a.db.Begin()
	if err != nil {
		a.fail(w, err)
		return
	}
	defer tx.Rollback()
	var removed []int64
	rows, err := tx.Query(`SELECT id FROM transactions WHERE split_of = ? AND id != ?`, head.ID, head.ID)
	if err != nil {
		a.fail(w, err)
		return
	}
	for rows.Next() {
		var rid int64
		rows.Scan(&rid)
		removed = append(removed, rid)
	}
	rows.Close()
	if _, err := tx.Exec(`DELETE FROM transactions WHERE split_of = ? AND id != ?`, head.ID, head.ID); err != nil {
		a.fail(w, err)
		return
	}
	var splitOf *int64
	if len(b.Parts) > 1 {
		splitOf = &head.ID
	}
	note := func(n *string) string {
		if n != nil {
			return strings.TrimSpace(*n)
		}
		return head.Note
	}
	out := make([]Tx, len(b.Parts))
	for i, p := range b.Parts {
		x := head
		x.Amount, x.Category, x.Note, x.SplitOf = p.Amount, p.Category, note(p.Note), splitOf
		if i == 0 {
			_, err = tx.Exec(`UPDATE transactions SET amount = ?, category = ?, note = ?, split_of = ?,
				updated_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now') WHERE id = ?`, x.Amount, x.Category, x.Note, x.SplitOf, x.ID)
		} else {
			x.SourceRef = nil // unique; the original row keeps the email's ref
			var res sql.Result
			res, err = tx.Exec(`INSERT INTO transactions
				(user_id, created_by, occurred_at, amount, type, category, merchant, account, to_account, note, source, source_ref, confidence, needs_review, currency, orig_amount, split_of)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, ?, ?, ?, NULL, ?)`,
				uid, x.CreatedBy, x.OccurredAt, x.Amount, x.Type, x.Category, x.Merchant, x.Account, x.ToAccount, x.Note, x.Source, x.Confidence, x.NeedsReview, x.Currency, x.SplitOf)
			if err == nil {
				x.ID, _ = res.LastInsertId()
			}
		}
		if err != nil {
			a.fail(w, err)
			return
		}
		out[i] = x
	}
	if err := tx.Commit(); err != nil {
		a.fail(w, err)
		return
	}
	to := a.audience(uid, head)
	for _, rid := range removed {
		a.publishAll(to, "transaction.deleted", Tx{ID: rid})
	}
	a.publishAll(to, "transaction.updated", out[0])
	for _, x := range out[1:] {
		a.publishAll(to, "transaction.created", x)
	}
	writeJSON(w, http.StatusOK, out)
}

// unsplitAfterDelete keeps a split consistent after one part was deleted: when the original row goes,
// the next part takes over its id role and source_ref; a single remaining part is a plain row again.
// It reports whether another part still holds the deleted row's source_ref.
func unsplitAfterDelete(tx *sql.Tx, t Tx) (bool, error) {
	g, moved := *t.SplitOf, false
	if g == t.ID {
		var next int64
		err := tx.QueryRow(`SELECT COALESCE(MIN(id), 0) FROM transactions WHERE split_of = ?`, g).Scan(&next)
		if err != nil || next == 0 {
			return false, err
		}
		if _, err := tx.Exec(`UPDATE transactions SET split_of = ? WHERE split_of = ?`, next, g); err != nil {
			return false, err
		}
		if _, err := tx.Exec(`UPDATE transactions SET source_ref = ? WHERE id = ?`, t.SourceRef, next); err != nil {
			return false, err
		}
		g, moved = next, true
	}
	_, err := tx.Exec(`UPDATE transactions SET split_of = NULL WHERE split_of = ?1 AND (SELECT COUNT(*) FROM transactions WHERE split_of = ?1) = 1`, g)
	return moved && t.SourceRef != nil, err
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
	uid := userID(r)
	for _, ref := range body.Refs {
		var n int
		if err := a.db.QueryRow(`SELECT (SELECT COUNT(*) FROM transactions WHERE user_id = ?2 AND source_ref = ?1)
			+ (SELECT COUNT(*) FROM deleted_refs WHERE user_id = ?2 AND ref = ?1)`, ref, uid).Scan(&n); err != nil {
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
		WHERE `+txScope+` AND occurred_at >= @from AND occurred_at < @to GROUP BY type, category ORDER BY SUM(amount) DESC`,
		sql.Named("uid", userID(r)), sql.Named("from", from), sql.Named("to", to))
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

func (a *app) listCategories(w http.ResponseWriter, r *http.Request) {
	cats, err := a.categories(userID(r))
	if err != nil {
		a.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, cats)
}

const categoryMaxLen = 30

// cleanCategoryName trims and collapses whitespace and validates the result. The second value is an
// error message, empty when the name is fine.
func cleanCategoryName(raw string) (string, string) {
	// reject control characters (newline, tab, NUL) on the raw input: collapsing whitespace first would turn them into spaces
	hasControl := strings.IndexFunc(raw, unicode.IsControl) >= 0
	name := strings.Join(strings.Fields(raw), " ")
	switch {
	case hasControl:
		return "", "name must not contain control characters"
	case name == "":
		return "", "name is required"
	case utf8.RuneCountInString(name) > categoryMaxLen:
		return "", fmt.Sprintf("name must be at most %d characters", categoryMaxLen)
	}
	return name, ""
}

// createCategory adds a user category. Only in/out categories can be added: transfer has a single built-in
// category, and several code paths rely on it.
func (a *app) createCategory(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name string `json:"name"`
		Type string `json:"type"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Type != "in" && in.Type != "out" {
		writeErr(w, http.StatusBadRequest, "type must be in or out")
		return
	}
	name, msg := cleanCategoryName(in.Name)
	if msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	// the table is unique on (user_id, name, type) but case-sensitive: "makan" next to "Makan" would only confuse
	uid := userID(r)
	var n int
	if err := a.db.QueryRow(`SELECT COUNT(*) FROM categories WHERE user_id = ? AND lower(name) = lower(?) AND type = ?`, uid, name, in.Type).Scan(&n); err != nil {
		a.fail(w, err)
		return
	}
	if n > 0 {
		writeErr(w, http.StatusConflict, "category already exists")
		return
	}
	res, err := a.db.Exec(`INSERT INTO categories (user_id, name, type, custom) VALUES (?, ?, ?, 1)`, uid, name, in.Type)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") { // lost a race with the same request
			writeErr(w, http.StatusConflict, "category already exists")
			return
		}
		a.fail(w, err)
		return
	}
	id, _ := res.LastInsertId()
	a.hub.publish(uid, "categories.changed", map[string]any{})
	writeJSON(w, http.StatusCreated, Category{ID: id, Name: name, Type: in.Type, Custom: true})
}

// customCategory loads a user-created category, writing 404/403 when it is missing or built in.
func (a *app) customCategory(w http.ResponseWriter, uid, id int64, verb string) (Category, bool) {
	var c Category
	err := a.db.QueryRow(`SELECT id, name, type, custom FROM categories WHERE id = ? AND user_id = ?`, id, uid).Scan(&c.ID, &c.Name, &c.Type, &c.Custom)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		writeErr(w, http.StatusNotFound, "not found")
	case err != nil:
		a.fail(w, err)
	case !c.Custom:
		writeErr(w, http.StatusForbidden, "built-in categories cannot be "+verb)
	default:
		return c, true
	}
	return c, false
}

// patchCategory renames a user-created category. Transactions and rules follow through ON UPDATE CASCADE;
// recurring items (their foreign key has no cascade) and budgets (no foreign key) are updated here, all in one
// transaction so a failure leaves the old name everywhere.
func (a *app) patchCategory(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in struct {
		Name *string `json:"name"`
	}
	if !decode(w, r, &in) {
		return
	}
	uid := userID(r)
	cur, ok := a.customCategory(w, uid, id, "renamed")
	if !ok {
		return
	}
	if in.Name == nil {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}
	name, msg := cleanCategoryName(*in.Name)
	if msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if name == cur.Name {
		writeJSON(w, http.StatusOK, cur)
		return
	}
	var other int64
	if err := a.db.QueryRow(`SELECT id FROM categories WHERE user_id = ? AND lower(name) = lower(?) AND type = ? AND id != ?`, uid, name, cur.Type, id).Scan(&other); err == nil {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "category already exists", "id": other})
		return
	} else if !errors.Is(err, sql.ErrNoRows) {
		a.fail(w, err)
		return
	}

	tx, err := a.db.Begin()
	if err != nil {
		a.fail(w, err)
		return
	}
	defer tx.Rollback()
	steps := []struct {
		q    string
		args []any
	}{
		{`PRAGMA defer_foreign_keys = ON`, nil}, // recurring points at the old name until its own UPDATE below: check at commit
		{`UPDATE categories SET name = ? WHERE id = ?`, []any{name, id}},
		{`UPDATE recurring SET category = ? WHERE user_id = ? AND category = ? AND type = ?`, []any{name, uid, cur.Name, cur.Type}},
	}
	if cur.Type == "out" { // budgets only exist for expense categories
		steps = append(steps, struct {
			q    string
			args []any
		}{`UPDATE budgets SET category = ? WHERE user_id = ? AND category = ?`, []any{name, uid, cur.Name}})
	}
	for _, st := range steps {
		if _, err := tx.Exec(st.q, st.args...); err != nil {
			a.fail(w, err)
			return
		}
	}
	if err := tx.Commit(); err != nil {
		a.fail(w, err)
		return
	}
	a.hub.publish(uid, "categories.changed", map[string]any{})
	cur.Name = name
	writeJSON(w, http.StatusOK, cur)
}

// deleteCategory removes a user-created category. Its transactions and recurring items move to "Lainnya" of
// the same type; rules and budgets for it are dropped, since they only existed to point at this category.
// One database transaction, so a failure changes nothing.
func (a *app) deleteCategory(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	uid := userID(r)
	cur, ok := a.customCategory(w, uid, id, "deleted")
	if !ok {
		return
	}
	tx, err := a.db.Begin()
	if err != nil {
		a.fail(w, err)
		return
	}
	defer tx.Rollback()
	var moved, rulesDeleted int64
	steps := []struct {
		q   string
		n   *int64
		arg []any
	}{
		{`UPDATE transactions SET category = 'Lainnya' WHERE user_id = ? AND category = ? AND type = ?`, &moved, []any{uid, cur.Name, cur.Type}},
		{`UPDATE recurring SET category = 'Lainnya' WHERE user_id = ? AND category = ? AND type = ?`, nil, []any{uid, cur.Name, cur.Type}},
		{`DELETE FROM rules WHERE user_id = ? AND category = ? AND type = ?`, &rulesDeleted, []any{uid, cur.Name, cur.Type}},
		{`DELETE FROM categories WHERE id = ?`, nil, []any{id}},
	}
	if cur.Type == "out" { // budgets only exist for expense categories
		steps = append(steps, struct {
			q   string
			n   *int64
			arg []any
		}{`DELETE FROM budgets WHERE user_id = ? AND category = ?`, nil, []any{uid, cur.Name}})
	}
	for _, st := range steps {
		res, err := tx.Exec(st.q, st.arg...)
		if err != nil {
			a.fail(w, err)
			return
		}
		if st.n != nil {
			*st.n, _ = res.RowsAffected()
		}
	}
	if err := tx.Commit(); err != nil {
		a.fail(w, err)
		return
	}
	a.hub.publish(uid, "categories.changed", map[string]any{})
	writeJSON(w, http.StatusOK, map[string]any{"moved_to": "Lainnya", "transactions": moved, "rules_deleted": rulesDeleted})
}

// ---------- rules ----------

func (a *app) listRules(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(`SELECT id, pattern, match, type, category FROM rules WHERE user_id = ? ORDER BY id`, userID(r))
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
	uid := userID(r)
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
	case !validType(ru.Type) || !a.categoryExists(uid, ru.Category, ru.Type):
		writeErr(w, http.StatusBadRequest, "unknown type/category, see /api/categories")
		return
	}
	// one rule per pattern: a newer correction replaces the old one
	a.db.Exec(`DELETE FROM rules WHERE user_id = ? AND lower(pattern) = lower(?)`, uid, ru.Pattern)
	res, err := a.db.Exec(`INSERT INTO rules (user_id, pattern, match, type, category) VALUES (?, ?, ?, ?, ?)`,
		uid, ru.Pattern, ru.Match, ru.Type, ru.Category)
	if err != nil {
		a.fail(w, err)
		return
	}
	ru.ID, _ = res.LastInsertId()
	writeJSON(w, http.StatusCreated, ru)
}

// patchRule edits a rule in place, so changing its category no longer needs delete + create.
func (a *app) patchRule(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var p struct {
		Pattern  *string `json:"pattern"`
		Type     *string `json:"type"`
		Category *string `json:"category"`
	}
	if !decode(w, r, &p) {
		return
	}
	var ru Rule
	uid := userID(r)
	err := a.db.QueryRow(`SELECT id, pattern, match, type, category FROM rules WHERE id = ? AND user_id = ?`, id, uid).
		Scan(&ru.ID, &ru.Pattern, &ru.Match, &ru.Type, &ru.Category)
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, http.StatusNotFound, "not found")
		return
	} else if err != nil {
		a.fail(w, err)
		return
	}
	if p.Pattern != nil {
		ru.Pattern = strings.TrimSpace(*p.Pattern)
	}
	if p.Type != nil {
		ru.Type = *p.Type
	}
	if p.Category != nil {
		ru.Category = *p.Category
	}
	switch {
	case ru.Pattern == "":
		writeErr(w, http.StatusBadRequest, "pattern is required")
		return
	case !validType(ru.Type) || !a.categoryExists(uid, ru.Category, ru.Type):
		writeErr(w, http.StatusBadRequest, "unknown type/category, see /api/categories")
		return
	}
	// one rule per pattern: renaming onto another rule's pattern is a conflict, not a silent overwrite
	var other int64
	if err := a.db.QueryRow(`SELECT id FROM rules WHERE user_id = ? AND lower(pattern) = lower(?) AND id != ?`, uid, ru.Pattern, id).Scan(&other); err == nil {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "another rule already uses this pattern", "id": other})
		return
	}
	if _, err := a.db.Exec(`UPDATE rules SET pattern = ?, type = ?, category = ? WHERE id = ? AND user_id = ?`, ru.Pattern, ru.Type, ru.Category, id, uid); err != nil {
		a.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ru)
}

func (a *app) deleteRule(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	res, err := a.db.Exec(`DELETE FROM rules WHERE id = ? AND user_id = ?`, id, userID(r))
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
