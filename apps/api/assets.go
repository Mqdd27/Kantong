package main

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"time"
)

type Asset struct {
	ID         int64    `json:"id"`
	Name       string   `json:"name"`
	Kind       string   `json:"kind"`
	Currency   string   `json:"currency"`
	Balance    float64  `json:"balance"`
	Invested   *float64 `json:"invested"`
	Note       string   `json:"note"`
	Target     *float64 `json:"target"`      // savings goal in Currency
	TargetDate *string  `json:"target_date"` // YYYY-MM-DD
	ReturnPct  *float64 `json:"return_pct"`  // expected % per year (bond coupon/yield, fund return)
	Maturity   *string  `json:"maturity"`    // YYYY-MM-DD, bonds/SBN
	Payout     int      `json:"payout"`      // months between coupons (1, 3, 6, 12); 0 = none
	Face       *float64 `json:"face"`        // bond nominal, repaid at maturity
	DueDay     *int     `json:"due_day"`     // debts: day of month the bill is due
	UpdatedAt  string   `json:"updated_at"`
	// computed for responses
	Rate     float64 `json:"rate"`                // IDR per unit of Currency
	ValueIDR int64   `json:"value_idr"`           // Balance converted to IDR
	DueDate  string  `json:"due_date,omitempty"`  // next due date (YYYY-MM-DD)
	Overdue  bool    `json:"overdue,omitempty"`   // last due date passed, still owing, no payment since the one before
	SharedID int64   `json:"shared_id,omitempty"` // kantong bersama this asset is, 0 = personal
	uid      int64   // holder: the owner, or for a kantong bersama its creator (or who took over)
}

type Order struct {
	ID         int64   `json:"id"`
	AssetID    int64   `json:"asset_id"`
	Asset      string  `json:"asset"`
	Side       string  `json:"side"`
	Amount     float64 `json:"amount"`
	Status     string  `json:"status"`
	OrderRef   string  `json:"order_ref"`
	SourceRef  *string `json:"source_ref"`
	Note       string  `json:"note"`
	OccurredAt string  `json:"occurred_at"`
}

var assetKinds = map[string]bool{"bank": true, "ewallet": true, "forex": true, "cash": true, "stock": true,
	"bond": true, "sbn": true, "mutual_fund": true, "gold": true, "crypto": true, "other": true,
	"credit_card": true, "paylater": true, "loan": true}

// Liabilities hold a negative balance (what is owed): net worth is the plain sum, card spending
// booked to the account makes it more negative and a transfer into it (a payment) pays it down.
var liabilityKinds = map[string]bool{"credit_card": true, "paylater": true, "loan": true}

const assetCols = `id, name, kind, currency, balance, invested, note, target, target_date, return_pct, maturity, payout, face, due_day, updated_at,
	user_id, COALESCE((SELECT s.id FROM shared_assets s WHERE s.asset_id = assets.id), 0)`

// assetScope limits assets to the caller's own plus the kantong bersama they belong to. Arg: @uid.
const assetScope = `(user_id = @uid OR id IN (SELECT s.asset_id FROM shared_assets s JOIN shared_members m ON m.shared_id = s.id WHERE m.user_id = @uid))`

func scanAsset(s scanner) (Asset, error) {
	var x Asset
	err := s.Scan(&x.ID, &x.Name, &x.Kind, &x.Currency, &x.Balance, &x.Invested, &x.Note, &x.Target, &x.TargetDate, &x.ReturnPct, &x.Maturity, &x.Payout, &x.Face, &x.DueDay, &x.UpdatedAt,
		&x.uid, &x.SharedID)
	return x, err
}

// canAccessAsset loads an asset the caller owns or shares; sql.ErrNoRows (answer 404) for anyone else,
// so a kantong's existence doesn't leak. Every member may read and write it.
func (a *app) canAccessAsset(uid, id int64) (Asset, error) {
	return scanAsset(a.db.QueryRow(`SELECT `+assetCols+` FROM assets WHERE id = @id AND `+assetScope, sql.Named("id", id), sql.Named("uid", uid)))
}

// nameTaken reports whether any of uids already sees an asset called name (own or kantong bersama,
// ignoring case: balances match account names that way), other than asset except.
func (a *app) nameTaken(name string, except int64, uids ...int64) (bool, error) {
	for _, uid := range uids {
		var taken bool
		if err := a.db.QueryRow(`SELECT EXISTS (SELECT 1 FROM assets WHERE lower(name) = lower(@name) AND id != @except AND `+assetScope+`)`,
			sql.Named("name", strings.TrimSpace(name)), sql.Named("except", except), sql.Named("uid", uid)).Scan(&taken); err != nil || taken {
			return taken, err
		}
	}
	return false, nil
}

// assetTxScope matches the transactions that move an asset's balance: the holder's own booked to its
// name, or for a kantong bersama every member's (kantong_tx). Args from assetTxArgs.
const assetTxScope = `(CASE WHEN @shared = 0 THEN user_id = @holder AND (lower(trim(account)) = @name OR lower(to_account) = @name)
	ELSE id IN (SELECT tx_id FROM kantong_tx WHERE shared_id = @shared) END)`

func assetTxArgs(x Asset, more ...any) []any {
	return append([]any{sql.Named("name", strings.ToLower(strings.TrimSpace(x.Name))), sql.Named("holder", x.uid), sql.Named("shared", x.SharedID)}, more...)
}

const orderCols = `o.id, o.asset_id, a.name, o.side, o.amount, o.status, o.order_ref, o.source_ref, o.note, o.occurred_at`

func scanOrder(s scanner) (Order, error) {
	var o Order
	err := s.Scan(&o.ID, &o.AssetID, &o.Asset, &o.Side, &o.Amount, &o.Status, &o.OrderRef, &o.SourceRef, &o.Note, &o.OccurredAt)
	return o, err
}

// Clients refetch /api/assets on this event, which also refreshes today's snapshot.
func (a *app) assetsChanged(uid int64) { a.hub.publish(uid, "assets.changed", map[string]any{}) }

func validAsset(w http.ResponseWriter, x *Asset) bool {
	x.Name, x.Note = strings.TrimSpace(x.Name), strings.TrimSpace(x.Note)
	if x.TargetDate != nil && *x.TargetDate == "" {
		x.TargetDate = nil
	}
	if x.Maturity != nil && *x.Maturity == "" {
		x.Maturity = nil
	}
	var ok bool
	x.Currency, ok = normCurrency(x.Currency)
	switch {
	case x.Name == "":
		writeErr(w, http.StatusBadRequest, "name is required")
	case !assetKinds[x.Kind]:
		writeErr(w, http.StatusBadRequest, "kind must be one of bank, ewallet, forex, cash, stock, bond, sbn, mutual_fund, gold, crypto, other, credit_card, paylater, loan")
	case !ok:
		writeErr(w, http.StatusBadRequest, "currency must be a 3-letter ISO code")
	case x.Target != nil && *x.Target <= 0:
		writeErr(w, http.StatusBadRequest, "target must be positive")
	case x.TargetDate != nil && !validDate(*x.TargetDate):
		writeErr(w, http.StatusBadRequest, "target_date must be YYYY-MM-DD")
	case x.ReturnPct != nil && (*x.ReturnPct <= -100 || *x.ReturnPct > 100):
		writeErr(w, http.StatusBadRequest, "return_pct must be between -100 and 100 (% per year)")
	case x.Payout != 0 && x.Payout != 1 && x.Payout != 3 && x.Payout != 6 && x.Payout != 12:
		writeErr(w, http.StatusBadRequest, "payout must be 0 (no coupon), 1, 3, 6 or 12 months")
	case x.Face != nil && *x.Face <= 0:
		writeErr(w, http.StatusBadRequest, "face must be > 0")
	case x.Maturity != nil && !validDate(*x.Maturity):
		writeErr(w, http.StatusBadRequest, "maturity must be YYYY-MM-DD")
	case x.DueDay != nil && (*x.DueDay < 1 || *x.DueDay > 31):
		writeErr(w, http.StatusBadRequest, "due_day must be 1-31")
	default:
		return true
	}
	return false
}

type portfolio struct {
	Assets       []Asset  `json:"assets"`
	TotalIDR     int64    `json:"total_idr"`
	InvestedIDR  int64    `json:"invested_idr"` // cost basis, or current value for assets without one
	Pending      []Order  `json:"pending_orders"`
	MissingRates []string `json:"missing_rates"`
}

// portfolio values every asset in IDR and records today's snapshot for the performance chart.
func (a *app) portfolio(ctx context.Context, uid int64) (p portfolio, err error) {
	p.Assets, p.Pending, p.MissingRates = []Asset{}, []Order{}, []string{}
	rows, err := a.db.QueryContext(ctx, `SELECT `+assetCols+` FROM assets WHERE `+assetScope+` ORDER BY kind, name`, sql.Named("uid", uid))
	if err != nil {
		return p, err
	}
	for rows.Next() {
		x, err := scanAsset(rows)
		if err != nil {
			rows.Close()
			return p, err
		}
		p.Assets = append(p.Assets, x)
	}
	rows.Close()

	for i := range p.Assets {
		x := &p.Assets[i]
		if x.Balance, err = a.liveBalance(ctx, *x); err != nil {
			return p, err
		}
		if x.DueDay != nil && liabilityKinds[x.Kind] {
			if err = a.dueStatus(ctx, x, time.Now().In(wib)); err != nil {
				return p, err
			}
		}
		rate, err := a.idrRate(ctx, x.Currency, "")
		if err != nil {
			p.MissingRates = append(p.MissingRates, x.Currency)
			continue
		}
		x.Rate = rate
		x.ValueIDR = int64(math.Round(x.Balance * rate))
		p.TotalIDR += x.ValueIDR
		basis := x.Balance * rate
		if x.Invested != nil {
			basis = *x.Invested * rate
			if x.Currency == "XAU" { // gold modal is entered in IDR, balance in grams
				basis = *x.Invested
			}
		}
		p.InvestedIDR += int64(math.Round(basis))
	}

	orows, err := a.db.QueryContext(ctx, `SELECT `+orderCols+` FROM asset_orders o JOIN assets a ON a.id = o.asset_id
		WHERE o.user_id = ? AND o.status = 'pending' ORDER BY o.occurred_at DESC`, uid)
	if err != nil {
		return p, err
	}
	defer orows.Close()
	for orows.Next() {
		o, err := scanOrder(orows)
		if err != nil {
			return p, err
		}
		p.Pending = append(p.Pending, o)
	}

	if len(p.Assets) > 0 && len(p.MissingRates) == 0 { // a partial total would show a fake dip
		_, err = a.db.ExecContext(ctx, `INSERT OR REPLACE INTO asset_snapshots (user_id, date, total_idr, invested_idr) VALUES (?, ?, ?, ?)`,
			uid, time.Now().In(wib).Format(time.DateOnly), p.TotalIDR, p.InvestedIDR)
	}
	return p, err
}

func (a *app) listAssets(w http.ResponseWriter, r *http.Request) {
	p, err := a.portfolio(r.Context(), userID(r))
	if err != nil {
		a.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (a *app) assetHistory(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(`SELECT date, total_idr, invested_idr FROM asset_snapshots WHERE user_id = ? ORDER BY date`, userID(r))
	if err != nil {
		a.fail(w, err)
		return
	}
	defer rows.Close()
	type point struct {
		Date     string `json:"date"`
		Total    int64  `json:"total"`
		Invested int64  `json:"invested"`
	}
	out := []point{}
	for rows.Next() {
		var p point
		if err := rows.Scan(&p.Date, &p.Total, &p.Invested); err != nil {
			a.fail(w, err)
			return
		}
		out = append(out, p)
	}
	writeJSON(w, http.StatusOK, out)
}

// snapshotLoop hourly records each account's net worth snapshot (so days without visits get a point)
// and posts due recurring items.
func (a *app) snapshotLoop(ctx context.Context) {
	for {
		a.postDue(ctx)
		for _, uid := range a.userIDs() {
			if _, err := a.portfolio(ctx, uid); err != nil && ctx.Err() == nil {
				slog.Error("snapshot", "user", uid, "err", err)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Hour):
		}
	}
}

func (a *app) createAsset(w http.ResponseWriter, r *http.Request) {
	var x Asset
	if !decode(w, r, &x) || !validAsset(w, &x) {
		return
	}
	uid := userID(r)
	if taken, err := a.nameTaken(x.Name, 0, uid); err != nil || taken {
		a.nameConflict(w, err)
		return
	}
	res, err := a.db.Exec(`INSERT INTO assets (user_id, name, kind, currency, balance, invested, note, target, target_date, return_pct, maturity, payout, face, due_day) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		uid, x.Name, x.Kind, x.Currency, x.Balance, x.Invested, x.Note, x.Target, x.TargetDate, x.ReturnPct, x.Maturity, x.Payout, x.Face, x.DueDay)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			writeErr(w, http.StatusConflict, "asset name already exists")
			return
		}
		a.fail(w, err)
		return
	}
	x.ID, _ = res.LastInsertId()
	a.assetsChanged(uid)
	writeJSON(w, http.StatusCreated, x)
}

func (a *app) patchAsset(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	uid := userID(r)
	x, err := a.canAccessAsset(uid, id)
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, http.StatusNotFound, "not found")
		return
	} else if err != nil {
		a.fail(w, err)
		return
	}
	if x.Balance, err = a.liveBalance(r.Context(), x); err != nil { // the save becomes the new anchor
		a.fail(w, err)
		return
	}
	tracked, name := x.Balance, x.Name
	// Decoding onto the current row means omitted fields keep their value.
	if !decode(w, r, &x) || !validAsset(w, &x) {
		return
	}
	if !strings.EqualFold(x.Name, name) {
		users := []int64{x.uid}
		if x.SharedID != 0 {
			users = a.members(x.SharedID)
		}
		if taken, err := a.nameTaken(x.Name, id, users...); err != nil || taken {
			a.nameConflict(w, err)
			return
		}
	}
	now := time.Now().UTC().Truncate(time.Second)
	x.UpdatedAt = now.Format(time.RFC3339)
	_, err = a.db.Exec(`UPDATE assets SET name = ?, kind = ?, currency = ?, balance = ?, invested = ?, note = ?, target = ?, target_date = ?,
		return_pct = ?, maturity = ?, payout = ?, face = ?, due_day = ?, updated_at = ? WHERE id = ?`,
		x.Name, x.Kind, x.Currency, x.Balance, x.Invested, x.Note, x.Target, x.TargetDate, x.ReturnPct, x.Maturity, x.Payout, x.Face, x.DueDay, x.UpdatedAt, id)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			writeErr(w, http.StatusConflict, "asset name already exists")
			return
		}
		a.fail(w, err)
		return
	}
	x.ID = id
	if err := a.adjust(uid, x, tracked, now); err != nil { // entered by whoever saved it
		a.fail(w, err)
		return
	}
	a.assetChanged(x)
	writeJSON(w, http.StatusOK, x)
}

// adjust records the gap between the tracked and the newly entered balance of a cash account or debt
// (interest, admin fees, spending without an email) as a "Penyesuaian" transaction in Review,
// so the user can give it a real category. It sits exactly on the new anchor, so it doesn't move the balance again.
func (a *app) adjust(uid int64, x Asset, tracked float64, at time.Time) error {
	diff := int64(math.Round(x.Balance - tracked))
	if diff == 0 || x.Currency != "IDR" || (x.Kind != "bank" && x.Kind != "ewallet" && x.Kind != "cash" && !liabilityKinds[x.Kind]) {
		return nil
	}
	t := Tx{OccurredAt: at.In(wib).Format(time.RFC3339), Amount: diff, Type: "in", Category: "Penyesuaian",
		Merchant: "Penyesuaian saldo", Account: x.Name, Note: "selisih saldo tercatat vs saldo asli",
		Source: "manual", NeedsReview: true, Currency: "IDR"}
	if diff < 0 {
		t.Amount, t.Type = -diff, "out"
	}
	return a.insertTx(uid, &t)
}

// assetActivity is an asset's history, newest first: transactions booked to (or transferred into)
// the account of the same name, including balance adjustments, plus its investment orders.
func (a *app) assetActivity(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	x, err := a.canAccessAsset(userID(r), id)
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, http.StatusNotFound, "not found")
		return
	} else if err != nil {
		a.fail(w, err)
		return
	}
	out := struct {
		Transactions []Tx    `json:"transactions"`
		Orders       []Order `json:"orders"`
	}{[]Tx{}, []Order{}}
	// ponytail: last 200 only, add paging when an account outgrows it
	rows, err := a.db.Query(`SELECT `+txCols+` FROM transactions WHERE `+assetTxScope+`
		ORDER BY occurred_at DESC, id DESC LIMIT 200`, assetTxArgs(x)...)
	if err != nil {
		a.fail(w, err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		t, err := scanTx(rows)
		if err != nil {
			a.fail(w, err)
			return
		}
		out.Transactions = append(out.Transactions, t)
	}
	orows, err := a.db.Query(`SELECT `+orderCols+` FROM asset_orders o JOIN assets a ON a.id = o.asset_id
		WHERE o.asset_id = ? ORDER BY o.occurred_at DESC LIMIT 200`, id)
	if err != nil {
		a.fail(w, err)
		return
	}
	defer orows.Close()
	for orows.Next() {
		o, err := scanOrder(orows)
		if err != nil {
			a.fail(w, err)
			return
		}
		out.Orders = append(out.Orders, o)
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *app) deleteAsset(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	uid := userID(r)
	x, err := a.canAccessAsset(uid, id)
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, http.StatusNotFound, "not found")
		return
	} else if err != nil {
		a.fail(w, err)
		return
	}
	if x.SharedID != 0 {
		writeErr(w, http.StatusConflict, "this is a kantong bersama: its creator deletes it with DELETE /api/shared/{id}")
		return
	}
	if _, err := a.db.Exec(`DELETE FROM assets WHERE id = ? AND user_id = ?`, id, uid); err != nil {
		a.fail(w, err)
		return
	}
	a.assetsChanged(uid)
	w.WriteHeader(http.StatusNoContent)
}

// dueOn is day `day` of the month `months` away from now, clamped to the month's last day.
func dueOn(now time.Time, day, months int) time.Time {
	first := time.Date(now.Year(), now.Month()+time.Month(months), 1, 0, 0, 0, 0, wib)
	last := first.AddDate(0, 1, -1).Day()
	return first.AddDate(0, 0, min(day, last)-1)
}

// dueStatus sets the next due date of a debt and whether the last one was missed:
// it has passed, the account still owes, and nothing was paid into it since the due date before.
func (a *app) dueStatus(ctx context.Context, x *Asset, now time.Time) error {
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, wib)
	due := dueOn(now, *x.DueDay, 0)
	if !today.After(due) {
		x.DueDate = due.Format(time.DateOnly)
		return nil
	}
	x.DueDate = dueOn(now, *x.DueDay, 1).Format(time.DateOnly)
	if x.Balance >= 0 {
		return nil
	}
	var paid int
	err := a.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM transactions WHERE `+assetTxScope+` AND type = 'transfer' AND lower(to_account) = @name
		AND needs_review = 0 AND substr(occurred_at, 1, 10) > @since`, assetTxArgs(*x, sql.Named("since", dueOn(now, *x.DueDay, -1).Format(time.DateOnly)))...).Scan(&paid)
	x.Overdue = paid == 0
	return err
}

// liveBalance is the stored balance plus transactions booked to an account of the same name
// after the balance was last saved, so a bank asset follows spending without manual updates.
// in adds, out subtracts, a transfer subtracts from account and adds to to_account.
// Transactions still waiting in Review don't count until approved. For a kantong bersama every
// member's transactions count once (assetTxScope); this is the one place a balance is computed.
func (a *app) liveBalance(ctx context.Context, x Asset) (float64, error) {
	var d float64
	err := a.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(
		(CASE WHEN type = 'in' THEN 1 WHEN lower(trim(account)) = @name THEN -1 ELSE 0 END
		 + CASE WHEN type = 'transfer' AND lower(to_account) = @name THEN 1 ELSE 0 END)
		* CASE WHEN currency = 'IDR' THEN amount ELSE orig_amount END), 0)
		FROM transactions WHERE `+assetTxScope+` AND currency = @currency AND needs_review = 0
		AND julianday(occurred_at) > julianday(@anchor)`, assetTxArgs(x, sql.Named("currency", x.Currency), sql.Named("anchor", x.UpdatedAt))...).Scan(&d)
	return x.Balance + d, err
}

// ---------- orders ----------

// applyOrder moves a successful order into the asset's balance and cost basis.
func applyOrder(tx *sql.Tx, assetID int64, side string, amount float64) error {
	if side == "buy" {
		_, err := tx.Exec(`UPDATE assets SET balance = balance + ?, invested = COALESCE(invested, 0) + ? WHERE id = ?`,
			amount, amount, assetID)
		return err
	}
	// sell: reduce cost basis proportionally to the share of value sold
	_, err := tx.Exec(`UPDATE assets SET
		invested = CASE WHEN invested IS NULL OR balance <= 0 THEN invested ELSE MAX(invested * (1 - MIN(? / balance, 1)), 0) END,
		balance = balance - ? WHERE id = ?`, amount, amount, assetID)
	return err
}

func (a *app) createOrder(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Asset      string  `json:"asset"`    // asset name; created when missing and kind is given
		Kind       string  `json:"kind"`     // only used to create the asset
		Currency   string  `json:"currency"` // only used to create the asset
		Side       string  `json:"side"`
		Amount     float64 `json:"amount"`
		Status     string  `json:"status"`
		OrderRef   string  `json:"order_ref"`
		SourceRef  *string `json:"source_ref"`
		Note       string  `json:"note"`
		OccurredAt string  `json:"occurred_at"`
	}
	if !decode(w, r, &b) {
		return
	}
	b.Asset, b.OrderRef = strings.TrimSpace(b.Asset), strings.TrimSpace(b.OrderRef)
	if b.Status == "" {
		b.Status = "pending"
	}
	if b.OccurredAt == "" {
		b.OccurredAt = time.Now().In(wib).Format(time.RFC3339)
	}
	ts, err := time.Parse(time.RFC3339, b.OccurredAt)
	switch {
	case b.Asset == "":
		writeErr(w, http.StatusBadRequest, "asset is required")
		return
	case b.Side != "buy" && b.Side != "sell":
		writeErr(w, http.StatusBadRequest, "side must be buy or sell")
		return
	case b.Amount <= 0:
		writeErr(w, http.StatusBadRequest, "amount must be positive")
		return
	case b.Status != "pending" && b.Status != "success" && b.Status != "failed":
		writeErr(w, http.StatusBadRequest, "status must be pending, success or failed")
		return
	case b.OrderRef == "":
		writeErr(w, http.StatusBadRequest, "order_ref is required (the order number from the email)")
		return
	case err != nil:
		writeErr(w, http.StatusBadRequest, "occurred_at must be RFC3339")
		return
	}

	var existing int64
	uid := userID(r)
	if a.db.QueryRow(`SELECT id FROM asset_orders WHERE user_id = ? AND order_ref = ?`, uid, b.OrderRef).Scan(&existing) == nil {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "duplicate order_ref", "id": existing})
		return
	}

	tx, err := a.db.Begin()
	if err != nil {
		a.fail(w, err)
		return
	}
	defer tx.Rollback()
	var assetID int64
	err = tx.QueryRow(`SELECT id FROM assets WHERE `+assetScope+` AND name = @name COLLATE NOCASE`, sql.Named("uid", uid), sql.Named("name", b.Asset)).Scan(&assetID)
	if errors.Is(err, sql.ErrNoRows) {
		x := Asset{Name: b.Asset, Kind: b.Kind, Currency: b.Currency}
		if !validAsset(w, &x) {
			return
		}
		res, err := tx.Exec(`INSERT INTO assets (user_id, name, kind, currency) VALUES (?, ?, ?, ?)`, uid, x.Name, x.Kind, x.Currency)
		if err != nil {
			a.fail(w, err)
			return
		}
		assetID, _ = res.LastInsertId()
	} else if err != nil {
		a.fail(w, err)
		return
	}
	res, err := tx.Exec(`INSERT INTO asset_orders (user_id, asset_id, side, amount, status, order_ref, source_ref, note, occurred_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, uid, assetID, b.Side, b.Amount, b.Status, b.OrderRef, b.SourceRef, b.Note, ts.In(wib).Format(time.RFC3339))
	if err != nil {
		a.fail(w, err)
		return
	}
	if b.Status == "success" {
		if err := applyOrder(tx, assetID, b.Side, b.Amount); err != nil {
			a.fail(w, err)
			return
		}
	}
	if err := tx.Commit(); err != nil {
		a.fail(w, err)
		return
	}
	id, _ := res.LastInsertId()
	a.assetsChanged(uid)
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "asset_id": assetID})
}

// patchOrder settles a pending order by its order_ref: {"status": "success" | "failed"}.
func (a *app) patchOrder(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("ref")
	var b struct {
		Status string `json:"status"`
	}
	if !decode(w, r, &b) {
		return
	}
	if b.Status != "success" && b.Status != "failed" {
		writeErr(w, http.StatusBadRequest, "status must be success or failed")
		return
	}
	tx, err := a.db.Begin()
	if err != nil {
		a.fail(w, err)
		return
	}
	defer tx.Rollback()
	uid := userID(r)
	o, err := scanOrder(tx.QueryRow(`SELECT `+orderCols+` FROM asset_orders o JOIN assets a ON a.id = o.asset_id WHERE o.user_id = ? AND o.order_ref = ?`, uid, ref))
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, http.StatusNotFound, "unknown order_ref")
		return
	} else if err != nil {
		a.fail(w, err)
		return
	}
	if o.Status == b.Status { // repeated email: idempotent
		writeJSON(w, http.StatusOK, o)
		return
	}
	if o.Status != "pending" {
		writeErr(w, http.StatusConflict, "order already "+o.Status)
		return
	}
	if _, err := tx.Exec(`UPDATE asset_orders SET status = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now') WHERE id = ?`, b.Status, o.ID); err != nil {
		a.fail(w, err)
		return
	}
	if b.Status == "success" {
		if err := applyOrder(tx, o.AssetID, o.Side, o.Amount); err != nil {
			a.fail(w, err)
			return
		}
	}
	if err := tx.Commit(); err != nil {
		a.fail(w, err)
		return
	}
	o.Status = b.Status
	a.assetsChanged(uid)
	writeJSON(w, http.StatusOK, o)
}

func validDate(s string) bool { _, err := time.Parse(time.DateOnly, s); return err == nil }
