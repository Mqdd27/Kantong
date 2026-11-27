package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// applyRule overrides type/category from the most specific matching rule.
func (a *app) applyRule(uid int64, t *Tx) {
	if t.Merchant == "" {
		return
	}
	err := a.db.QueryRow(`SELECT type, category FROM rules
		WHERE user_id = ? AND match = 'contains' AND instr(lower(?), lower(pattern)) > 0
		ORDER BY length(pattern) DESC, id DESC LIMIT 1`, uid, strings.TrimSpace(t.Merchant)).Scan(&t.Type, &t.Category)
	if err == nil {
		one := 1.0
		t.Confidence, t.NeedsReview = &one, false
	}
}

// notify sends a Telegram message for a transaction that came from email, through each account's own bot:
// to uid, and to the other members of a kantong bersama it is booked to, saying who recorded it.
func (a *app) notify(uid int64, t Tx) {
	verb := map[string]string{"in": "Masuk", "out": "Keluar", "transfer": "Transfer"}[t.Type]
	text := fmt.Sprintf("%s %s · %s", verb, rupiah(t.Amount), t.Category)
	if t.Merchant != "" {
		text += " · " + t.Merchant
	}
	if t.OrigAmount != nil {
		text += fmt.Sprintf(" (%s %g)", t.Currency, *t.OrigAmount)
	}
	if t.NeedsReview {
		text += " (perlu review)"
	}
	var by string
	for _, to := range a.audience(uid, t) {
		token, chat := a.telegramFor(to)
		if token == "" || chat == "" {
			continue
		}
		msg := text
		if to != uid {
			if by == "" {
				a.db.QueryRow(`SELECT username FROM users WHERE id = ?`, uid).Scan(&by)
			}
			msg += " · dicatat " + by
		}
		go func() {
			if err := a.sendTelegram(token, chat, msg); err != nil {
				slog.Warn("telegram", "user", to, "err", err)
			}
		}()
	}
}

func rupiah(n int64) string {
	s := fmt.Sprint(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "." + s[i:]
	}
	return "Rp" + s
}

// backupLoop writes a daily VACUUM INTO copy at 03:00 WIB (and at start if today's is missing), keeping `keep` files.
func (a *app) backupLoop(ctx context.Context) {
	for {
		if err := a.backup(time.Now().In(wib), 14); err != nil {
			slog.Error("backup", "err", err)
		}
		if n, err := a.pruneSessions(time.Now()); err != nil {
			slog.Error("prune sessions", "err", err)
		} else if n > 0 {
			slog.Info("prune sessions", "deleted", n)
		}
		now := time.Now().In(wib)
		next := time.Date(now.Year(), now.Month(), now.Day(), 3, 0, 0, 0, wib)
		if !next.After(now) {
			next = next.AddDate(0, 0, 1)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Until(next)):
		}
	}
}

func (a *app) backup(now time.Time, keep int) error {
	dir := filepath.Join(filepath.Dir(a.cfg.dbPath), "backups")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	file := filepath.Join(dir, "money-"+now.Format(time.DateOnly)+".db")
	if _, err := os.Stat(file); err != nil {
		if _, err := a.db.Exec(`VACUUM INTO ?`, file); err != nil {
			return err
		}
		slog.Info("backup", "file", file)
	}
	old, _ := filepath.Glob(filepath.Join(dir, "money-*.db")) // dated names sort by date
	slices.Sort(old)
	for _, f := range old[:max(0, len(old)-keep)] {
		os.Remove(f)
	}
	return nil
}
