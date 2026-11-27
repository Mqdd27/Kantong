package main

import (
	"errors"
	"net/http"
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

// handOffShared runs before an account is deleted: its kantong bersama pass to their oldest member.
func (a *app) handOffShared(uid int64) error { return errors.New("not implemented") }
