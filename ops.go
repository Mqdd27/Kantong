package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// applyRule overrides type/category from the most specific matching rule.
func (a *app) applyRule(t *Tx) {
	if t.Merchant == "" {
		return
	}
	err := a.db.QueryRow(`SELECT type, category FROM rules
		WHERE match = 'contains' AND instr(lower(?), lower(pattern)) > 0
		ORDER BY length(pattern) DESC, id DESC LIMIT 1`, strings.TrimSpace(t.Merchant)).Scan(&t.Type, &t.Category)
	if err == nil {
		one := 1.0
		t.Confidence, t.NeedsReview = &one, false
	}
}

// notify sends a Telegram message for a transaction that came from email.
func (a *app) notify(t Tx) {
	if a.cfg.tgToken == "" || a.cfg.tgChat == "" {
		return
	}
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
	go func() {
		c := http.Client{Timeout: 10 * time.Second}
		res, err := c.PostForm(a.cfg.tgAPI+"/bot"+a.cfg.tgToken+"/sendMessage", url.Values{"chat_id": {a.cfg.tgChat}, "text": {text}})
		if err != nil {
			slog.Warn("telegram", "err", strings.ReplaceAll(err.Error(), a.cfg.tgToken, "***")) // URL holds the token
			return
		}
		res.Body.Close()
		if res.StatusCode != http.StatusOK {
			slog.Warn("telegram", "status", res.StatusCode)
		}
	}()
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
