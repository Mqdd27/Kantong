package main

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"
)

type Asset struct {
	ID        int64    `json:"id"`
	Name      string   `json:"name"`
	Kind      string   `json:"kind"`
	Currency  string   `json:"currency"`
	Balance   float64  `json:"balance"`
	Invested  *float64 `json:"invested"`
	Note      string   `json:"note"`
	UpdatedAt string   `json:"updated_at"`
	// computed for responses
	Rate     float64 `json:"rate,omitempty"`      // IDR per unit of Currency
	ValueIDR int64   `json:"value_idr,omitempty"` // Balance converted to IDR
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
	"bond": true, "sbn": true, "mutual_fund": true, "gold": true, "crypto": true, "other": true}

const assetCols = `id, name, kind, currency, balance, invested, note, updated_at`

func scanAsset(s scanner) (Asset, error) {
	var x Asset
	err := s.Scan(&x.ID, &x.Name, &x.Kind, &x.Currency, &x.Balance, &x.Invested, &x.Note, &x.UpdatedAt)
	return x, err
}

const orderCols = `o.id, o.asset_id, a.name, o.side, o.amount, o.status, o.order_ref, o.source_ref, o.note, o.occurred_at`

func scanOrder(s scanner) (Order, error) {
	var o Order
	err := s.Scan(&o.ID, &o.AssetID, &o.Asset, &o.Side, &o.Amount, &o.Status, &o.OrderRef, &o.SourceRef, &o.Note, &o.OccurredAt)
	return o, err
}

func (a *app) assetsChanged() { a.hub.publish("assets.changed", map[string]any{}) }

func validAsset(w http.ResponseWriter, x *Asset) bool {
	x.Name, x.Note = strings.TrimSpace(x.Name), strings.TrimSpace(x.Note)
	var ok bool
	x.Currency, ok = normCurrency(x.Currency)
	switch {
	case x.Name == "":
		writeErr(w, http.StatusBadRequest, "name is required")
	case !assetKinds[x.Kind]:
		writeErr(w, http.StatusBadRequest, "kind must be one of bank, ewallet, forex, cash, stock, bond, sbn, mutual_fund, gold, crypto, other")
	case !ok:
		writeErr(w, http.StatusBadRequest, "currency must be a 3-letter ISO code")
	default:
		return true
	}
	return false
}

// listAssets returns every asset with its IDR value, the net worth total and pending orders.
func (a *app) listAssets(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(`SELECT ` + assetCols + ` FROM assets ORDER BY kind, name`)
	if err != nil {
		a.fail(w, err)
		return
	}
	assets := []Asset{}
	for rows.Next() {
		x, err := scanAsset(rows)
		if err != nil {
			rows.Close()
			a.fail(w, err)
			return
		}
		assets = append(assets, x)
	}
	rows.Close()

	var total int64
	missing := []string{}
	for i := range assets {
		rate, err := a.idrRate(r.Context(), assets[i].Currency, "")
		if err != nil {
			missing = append(missing, assets[i].Currency)
			continue
		}
		assets[i].Rate = rate
		assets[i].ValueIDR = int64(assets[i].Balance*rate + 0.5)
		total += assets[i].ValueIDR
	}

	orows, err := a.db.Query(`SELECT ` + orderCols + ` FROM asset_orders o JOIN assets a ON a.id = o.asset_id
		WHERE o.status = 'pending' ORDER BY o.occurred_at DESC`)
	if err != nil {
		a.fail(w, err)
		return
	}
	defer orows.Close()
	pending := []Order{}
	for orows.Next() {
		o, err := scanOrder(orows)
		if err != nil {
			a.fail(w, err)
			return
		}
		pending = append(pending, o)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"assets": assets, "total_idr": total, "pending_orders": pending, "missing_rates": missing,
	})
}

func (a *app) createAsset(w http.ResponseWriter, r *http.Request) {
	var x Asset
	if !decode(w, r, &x) || !validAsset(w, &x) {
		return
	}
	res, err := a.db.Exec(`INSERT INTO assets (name, kind, currency, balance, invested, note) VALUES (?, ?, ?, ?, ?, ?)`,
		x.Name, x.Kind, x.Currency, x.Balance, x.Invested, x.Note)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			writeErr(w, http.StatusConflict, "asset name already exists")
			return
		}
		a.fail(w, err)
		return
	}
	x.ID, _ = res.LastInsertId()
	a.assetsChanged()
	writeJSON(w, http.StatusCreated, x)
}

func (a *app) patchAsset(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	x, err := scanAsset(a.db.QueryRow(`SELECT `+assetCols+` FROM assets WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, http.StatusNotFound, "not found")
		return
	} else if err != nil {
		a.fail(w, err)
		return
	}
	// Decoding onto the current row means omitted fields keep their value.
	if !decode(w, r, &x) || !validAsset(w, &x) {
		return
	}
	_, err = a.db.Exec(`UPDATE assets SET name = ?, kind = ?, currency = ?, balance = ?, invested = ?, note = ?,
		updated_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now') WHERE id = ?`,
		x.Name, x.Kind, x.Currency, x.Balance, x.Invested, x.Note, id)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			writeErr(w, http.StatusConflict, "asset name already exists")
			return
		}
		a.fail(w, err)
		return
	}
	x.ID = id
	a.assetsChanged()
	writeJSON(w, http.StatusOK, x)
}

func (a *app) deleteAsset(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	res, err := a.db.Exec(`DELETE FROM assets WHERE id = ?`, id)
	if err != nil {
		a.fail(w, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	a.assetsChanged()
	w.WriteHeader(http.StatusNoContent)
}

// ---------- orders ----------

// applyOrder moves a successful order into the asset's balance and cost basis.
func applyOrder(tx *sql.Tx, assetID int64, side string, amount float64) error {
	if side == "buy" {
		_, err := tx.Exec(`UPDATE assets SET balance = balance + ?, invested = COALESCE(invested, 0) + ?,
			updated_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now') WHERE id = ?`, amount, amount, assetID)
		return err
	}
	// sell: reduce cost basis proportionally to the share of value sold
	_, err := tx.Exec(`UPDATE assets SET
		invested = CASE WHEN invested IS NULL OR balance <= 0 THEN invested ELSE MAX(invested * (1 - MIN(? / balance, 1)), 0) END,
		balance = balance - ?, updated_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now') WHERE id = ?`, amount, amount, assetID)
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
	if a.db.QueryRow(`SELECT id FROM asset_orders WHERE order_ref = ?`, b.OrderRef).Scan(&existing) == nil {
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
	err = tx.QueryRow(`SELECT id FROM assets WHERE name = ? COLLATE NOCASE`, b.Asset).Scan(&assetID)
	if errors.Is(err, sql.ErrNoRows) {
		x := Asset{Name: b.Asset, Kind: b.Kind, Currency: b.Currency}
		if !validAsset(w, &x) {
			return
		}
		res, err := tx.Exec(`INSERT INTO assets (name, kind, currency) VALUES (?, ?, ?)`, x.Name, x.Kind, x.Currency)
		if err != nil {
			a.fail(w, err)
			return
		}
		assetID, _ = res.LastInsertId()
	} else if err != nil {
		a.fail(w, err)
		return
	}
	res, err := tx.Exec(`INSERT INTO asset_orders (asset_id, side, amount, status, order_ref, source_ref, note, occurred_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, assetID, b.Side, b.Amount, b.Status, b.OrderRef, b.SourceRef, b.Note, ts.In(wib).Format(time.RFC3339))
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
	a.assetsChanged()
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
	o, err := scanOrder(tx.QueryRow(`SELECT `+orderCols+` FROM asset_orders o JOIN assets a ON a.id = o.asset_id WHERE o.order_ref = ?`, ref))
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
	a.assetsChanged()
	writeJSON(w, http.StatusOK, o)
}
