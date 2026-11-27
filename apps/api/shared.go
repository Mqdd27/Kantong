package main

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Kantong bersama: one asset row used by several accounts (docs/shared-kantong.md).
// Which transactions belong to one is defined once, by the kantong_tx view (migration 023).

// members lists a kantong's accounts, oldest first.
func (a *app) members(sharedID int64) []int64 {
	return a.ids(`SELECT user_id FROM shared_members WHERE shared_id = ? ORDER BY joined_at, user_id`, sharedID)
}

// audience is uid plus the other members of every kantong bersama of uid that t is booked to or transferred into.
func (a *app) audience(uid int64, t Tx) []int64 {
	return append([]int64{uid}, a.ids(`SELECT DISTINCT o.user_id FROM shared_members me
		JOIN shared_assets s ON s.id = me.shared_id JOIN assets x ON x.id = s.asset_id
		JOIN shared_members o ON o.shared_id = s.id AND o.user_id != me.user_id
		WHERE me.user_id = ? AND lower(x.name) IN (lower(trim(?)), lower(?))`, uid, t.Account, t.ToAccount)...)
}

func (a *app) ids(query string, args ...any) []int64 {
	rows, err := a.db.Query(query, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		rows.Scan(&id)
		out = append(out, id)
	}
	return out
}

func (a *app) publishAll(uids []int64, event string, v any) {
	for _, uid := range uids {
		a.hub.publish(uid, event, v)
	}
}

// publishTx tells uid and the members of any kantong bersama t is booked to.
func (a *app) publishTx(uid int64, event string, t Tx) { a.publishAll(a.audience(uid, t), event, t) }

// assetChanged refreshes the assets of everyone who sees x.
func (a *app) assetChanged(x Asset) {
	if x.SharedID == 0 {
		a.assetsChanged(x.uid)
		return
	}
	for _, uid := range a.members(x.SharedID) {
		a.assetsChanged(uid)
	}
}

// nameConflict answers a failed or positive nameTaken check.
func (a *app) nameConflict(w http.ResponseWriter, err error) {
	if err != nil {
		a.fail(w, err)
		return
	}
	writeErr(w, http.StatusConflict, "asset name already exists for you or a member of the kantong bersama")
}

type Kantong struct {
	ID      int64    `json:"id"`
	AssetID int64    `json:"asset_id"`
	Name    string   `json:"name"`
	Role    string   `json:"role"` // the caller's: owner (creator, or who took over) or member
	Members []Member `json:"members"`
}

type Member struct {
	UserID   int64  `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	JoinedAt string `json:"joined_at"`
}

// role is uid's role in kantong sid, "" when not a member (answer 404, so its existence doesn't leak).
func (a *app) role(uid, sid int64) string {
	var role string
	a.db.QueryRow(`SELECT role FROM shared_members WHERE shared_id = ? AND user_id = ?`, sid, uid).Scan(&role)
	return role
}

// kantongs lists the caller's kantong bersama with their members; sid != 0 picks one.
func (a *app) kantongs(uid, sid int64) ([]Kantong, error) {
	rows, err := a.db.Query(`SELECT s.id, s.asset_id, x.name, me.role, u.id, u.username, m.role, m.joined_at
		FROM shared_members me JOIN shared_assets s ON s.id = me.shared_id JOIN assets x ON x.id = s.asset_id
		JOIN shared_members m ON m.shared_id = s.id JOIN users u ON u.id = m.user_id
		WHERE me.user_id = ? AND (? = 0 OR s.id = ?) ORDER BY x.name, s.id, m.joined_at, u.id`, uid, sid, sid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Kantong{}
	for rows.Next() {
		var k Kantong
		var m Member
		if err := rows.Scan(&k.ID, &k.AssetID, &k.Name, &k.Role, &m.UserID, &m.Username, &m.Role, &m.JoinedAt); err != nil {
			return nil, err
		}
		if n := len(out); n == 0 || out[n-1].ID != k.ID {
			out = append(out, k)
		}
		out[len(out)-1].Members = append(out[len(out)-1].Members, m)
	}
	return out, rows.Err()
}

func (a *app) listShared(w http.ResponseWriter, r *http.Request) {
	ks, err := a.kantongs(userID(r), 0)
	if err != nil {
		a.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ks)
}

// createShared makes a kantong bersama from one of the caller's assets ({"asset_id": 3}) or a new one
// (the asset fields of POST /api/assets). The caller becomes its owner.
func (a *app) createShared(w http.ResponseWriter, r *http.Request) {
	var b struct {
		AssetID int64 `json:"asset_id"`
		Asset
	}
	if !decode(w, r, &b) {
		return
	}
	uid := userID(r)
	x := b.Asset
	if b.AssetID != 0 {
		var err error
		if x, err = a.canAccessAsset(uid, b.AssetID); errors.Is(err, sql.ErrNoRows) || err == nil && x.uid != uid {
			writeErr(w, http.StatusNotFound, "not found")
			return
		} else if err != nil {
			a.fail(w, err)
			return
		}
		if x.SharedID != 0 {
			writeErr(w, http.StatusConflict, "already a kantong bersama")
			return
		}
		// re-anchor: from now on only kantong transactions move the balance
		if x.Balance, err = a.liveBalance(r.Context(), x); err != nil {
			a.fail(w, err)
			return
		}
	} else if !validAsset(w, &x) {
		return
	}
	if taken, err := a.nameTaken(x.Name, b.AssetID, uid); err != nil || taken {
		a.nameConflict(w, err)
		return
	}
	tx, err := a.db.Begin()
	if err != nil {
		a.fail(w, err)
		return
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339)
	if b.AssetID != 0 {
		_, err = tx.Exec(`UPDATE assets SET balance = ?, updated_at = ? WHERE id = ?`, x.Balance, now, x.ID)
	} else {
		err = tx.QueryRow(`INSERT INTO assets (user_id, name, kind, currency, balance, invested, note, target, target_date, return_pct, maturity, payout, face, due_day)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id`,
			uid, x.Name, x.Kind, x.Currency, x.Balance, x.Invested, x.Note, x.Target, x.TargetDate, x.ReturnPct, x.Maturity, x.Payout, x.Face, x.DueDay).Scan(&x.ID)
	}
	var sid int64
	if err == nil {
		err = tx.QueryRow(`INSERT INTO shared_assets (asset_id, created_by, created_at) VALUES (?, ?, ?) RETURNING id`, x.ID, uid, now).Scan(&sid)
	}
	if err == nil {
		_, err = tx.Exec(`INSERT INTO shared_members (shared_id, user_id, role, joined_at) VALUES (?, ?, 'owner', ?)`, sid, uid, now)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		a.fail(w, err)
		return
	}
	a.assetsChanged(uid)
	a.writeKantong(w, http.StatusCreated, uid, sid)
}

func (a *app) writeKantong(w http.ResponseWriter, code int, uid, sid int64) {
	ks, err := a.kantongs(uid, sid)
	if err != nil || len(ks) == 0 {
		a.fail(w, errors.Join(err, errors.New("kantong vanished")))
		return
	}
	writeJSON(w, code, ks[0])
}

// sharedAs answers for kantong {id} and returns it when the caller has the role (owner, or "" for any
// member). Non-members get 404, members without the role 403.
func (a *app) sharedAs(w http.ResponseWriter, r *http.Request, need string) (sid, uid int64, ok bool) {
	sid, ok = pathID(w, r)
	if !ok {
		return
	}
	uid = userID(r)
	switch role := a.role(uid, sid); {
	case role == "":
		writeErr(w, http.StatusNotFound, "not found")
	case need != "" && role != need:
		writeErr(w, http.StatusForbidden, "only the kantong's creator can do this")
	default:
		return sid, uid, true
	}
	return 0, 0, false
}

// kantongAsset is the asset row behind kantong sid.
func (a *app) kantongAsset(sid int64) (x Asset, err error) {
	return scanAsset(a.db.QueryRow(`SELECT `+assetCols+` FROM assets WHERE id = (SELECT asset_id FROM shared_assets WHERE id = ?)`, sid))
}

// addMember invites an account by username: {"username": "budi"}. Creator only.
func (a *app) addMember(w http.ResponseWriter, r *http.Request) {
	sid, uid, ok := a.sharedAs(w, r, "owner")
	if !ok {
		return
	}
	var b struct {
		Username string `json:"username"`
	}
	if !decode(w, r, &b) {
		return
	}
	var who int64
	if err := a.db.QueryRow(`SELECT id FROM users WHERE username = ?`, strings.TrimSpace(b.Username)).Scan(&who); errors.Is(err, sql.ErrNoRows) {
		writeErr(w, http.StatusNotFound, "no such user")
		return
	} else if err != nil {
		a.fail(w, err)
		return
	}
	if a.role(who, sid) != "" {
		writeErr(w, http.StatusConflict, "already a member")
		return
	}
	x, err := a.kantongAsset(sid)
	if err != nil {
		a.fail(w, err)
		return
	}
	if taken, err := a.nameTaken(x.Name, x.ID, who); err != nil || taken {
		if err == nil {
			writeErr(w, http.StatusConflict, "that person already has an asset called "+x.Name+"; rename it first")
			return
		}
		a.fail(w, err)
		return
	}
	if _, err := a.db.Exec(`INSERT INTO shared_members (shared_id, user_id, role) VALUES (?, ?, 'member')`, sid, who); err != nil {
		a.fail(w, err)
		return
	}
	a.assetChanged(x)
	a.writeKantong(w, http.StatusCreated, uid, sid)
}

// removeMember takes a member out of the kantong. Creator only; their entries stay in the kantong.
func (a *app) removeMember(w http.ResponseWriter, r *http.Request) {
	sid, uid, ok := a.sharedAs(w, r, "owner")
	if !ok {
		return
	}
	who, err := strconv.ParseInt(r.PathValue("uid"), 10, 64)
	if err != nil || a.role(who, sid) != "member" {
		writeErr(w, http.StatusNotFound, "not a member")
		return
	}
	a.leave(w, sid, who, uid)
}

// leaveShared takes the caller out of a kantong. The creator can't leave; they delete it instead.
func (a *app) leaveShared(w http.ResponseWriter, r *http.Request) {
	sid, uid, ok := a.sharedAs(w, r, "")
	if !ok {
		return
	}
	if a.role(uid, sid) == "owner" {
		writeErr(w, http.StatusBadRequest, "the creator can't leave; delete the kantong instead")
		return
	}
	a.leave(w, sid, uid, uid)
}

func (a *app) leave(w http.ResponseWriter, sid, who, by int64) {
	x, err := a.kantongAsset(sid)
	if err != nil {
		a.fail(w, err)
		return
	}
	notify := a.members(sid)
	err = a.inTx(func(tx *sql.Tx) error { return dropMember(tx, sid, who, x.uid) })
	if err != nil {
		a.fail(w, err)
		return
	}
	for _, u := range notify {
		a.assetsChanged(u)
	}
	w.WriteHeader(http.StatusNoContent)
}

// deleteShared deletes a kantong and every transaction in it, for all members:
// {"confirm": "<exact name>"}. Creator only.
func (a *app) deleteShared(w http.ResponseWriter, r *http.Request) {
	sid, _, ok := a.sharedAs(w, r, "owner")
	if !ok {
		return
	}
	var b struct {
		Confirm string `json:"confirm"`
	}
	if !decode(w, r, &b) {
		return
	}
	x, err := a.kantongAsset(sid)
	if err != nil {
		a.fail(w, err)
		return
	}
	if strings.TrimSpace(b.Confirm) != x.Name {
		writeErr(w, http.StatusBadRequest, "confirm must be the kantong's exact name")
		return
	}
	notify := a.members(sid)
	if err := a.inTx(func(tx *sql.Tx) error { return dropKantong(tx, sid) }); err != nil {
		a.fail(w, err)
		return
	}
	for _, u := range notify {
		a.assetsChanged(u)
		a.hub.publish(u, "transactions.changed", map[string]any{})
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *app) inTx(f func(*sql.Tx) error) error {
	tx, err := a.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := f(tx); err != nil {
		return err
	}
	return tx.Commit()
}

// moveEntries hands from's transactions and orders in kantong sid to account to, keeping created_by.
// to gets any category the entries use that it lacks; an email ref to already has is dropped.
func moveEntries(tx *sql.Tx, sid, from, to int64) error {
	args := []any{sql.Named("sid", sid), sql.Named("from", from), sql.Named("to", to)}
	for _, q := range []string{
		`INSERT OR IGNORE INTO categories (user_id, name, type, in_budget, custom)
			SELECT DISTINCT @to, c.name, c.type, c.in_budget, 1 FROM transactions t
			JOIN categories c ON c.user_id = t.user_id AND c.name = t.category AND c.type = t.type
			WHERE t.user_id = @from AND t.id IN (SELECT tx_id FROM kantong_tx WHERE shared_id = @sid)`,
		`UPDATE transactions SET user_id = @to,
			source_ref = CASE WHEN EXISTS (SELECT 1 FROM transactions o WHERE o.user_id = @to AND o.source_ref = transactions.source_ref) THEN NULL ELSE source_ref END
			WHERE user_id = @from AND id IN (SELECT tx_id FROM kantong_tx WHERE shared_id = @sid)`,
		`UPDATE OR IGNORE asset_orders SET user_id = @to WHERE user_id = @from AND asset_id = (SELECT asset_id FROM shared_assets WHERE id = @sid)`,
	} {
		if _, err := tx.Exec(q, args...); err != nil {
			return err
		}
	}
	return nil
}

// dropMember removes who from kantong sid; their entries stay, moved to holder.
func dropMember(tx *sql.Tx, sid, who, holder int64) error {
	if err := moveEntries(tx, sid, who, holder); err != nil {
		return err
	}
	_, err := tx.Exec(`DELETE FROM shared_members WHERE shared_id = ? AND user_id = ?`, sid, who)
	return err
}

// dropKantong deletes kantong sid, its asset and its transactions. Their email refs are remembered
// so the email cron doesn't import them again.
func dropKantong(tx *sql.Tx, sid int64) error {
	for _, q := range []string{
		`INSERT OR IGNORE INTO deleted_refs (user_id, ref) SELECT user_id, source_ref FROM transactions
			WHERE source_ref IS NOT NULL AND id IN (SELECT tx_id FROM kantong_tx WHERE shared_id = ?1)`,
		`DELETE FROM transactions WHERE id IN (SELECT tx_id FROM kantong_tx WHERE shared_id = ?1)`,
		`DELETE FROM assets WHERE id = (SELECT asset_id FROM shared_assets WHERE id = ?1)`, // cascades to the kantong
	} {
		if _, err := tx.Exec(q, sid); err != nil {
			return err
		}
	}
	return nil
}

// handOffShared runs before an account is deleted. Kantong it created pass to their oldest other member
// (by joined_at) with its entries, or are deleted when nobody else is in them; in the others it leaves.
func (a *app) handOffShared(uid int64) error {
	return a.inTx(func(tx *sql.Tx) error {
		type row struct {
			sid, holder int64
			role        string
		}
		var mine []row
		rows, err := tx.Query(`SELECT m.shared_id, x.user_id, m.role FROM shared_members m
			JOIN shared_assets s ON s.id = m.shared_id JOIN assets x ON x.id = s.asset_id WHERE m.user_id = ?`, uid)
		if err != nil {
			return err
		}
		for rows.Next() {
			var k row
			rows.Scan(&k.sid, &k.holder, &k.role)
			mine = append(mine, k)
		}
		rows.Close()
		for _, k := range mine {
			if k.role != "owner" {
				if err := dropMember(tx, k.sid, uid, k.holder); err != nil {
					return err
				}
				continue
			}
			var next int64
			err := tx.QueryRow(`SELECT user_id FROM shared_members WHERE shared_id = ? AND user_id != ?
				ORDER BY joined_at, user_id LIMIT 1`, k.sid, uid).Scan(&next)
			if errors.Is(err, sql.ErrNoRows) {
				if err := dropKantong(tx, k.sid); err != nil {
					return err
				}
				continue
			} else if err != nil {
				return err
			}
			for _, q := range []string{
				`UPDATE assets SET user_id = ?2 WHERE id = (SELECT asset_id FROM shared_assets WHERE id = ?1)`,
				`UPDATE shared_assets SET created_by = ?2 WHERE id = ?1`,
				`UPDATE shared_members SET role = 'owner' WHERE shared_id = ?1 AND user_id = ?2`,
			} {
				if _, err := tx.Exec(q, k.sid, next); err != nil {
					return err
				}
			}
			if err := dropMember(tx, k.sid, uid, next); err != nil {
				return err
			}
		}
		return nil
	})
}
