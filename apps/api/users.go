package main

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

const pbkdf2Iter = 600_000 // OWASP 2023 for PBKDF2-HMAC-SHA256

var b64 = base64.RawStdEncoding

// hashPassword returns "pbkdf2-sha256$<iter>$<salt>$<key>"; the iteration count travels with the hash.
func hashPassword(pw string) (string, error) {
	salt := make([]byte, 16)
	rand.Read(salt)
	k, err := pbkdf2.Key(sha256.New, pw, salt, pbkdf2Iter, 32)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("pbkdf2-sha256$%d$%s$%s", pbkdf2Iter, b64.EncodeToString(salt), b64.EncodeToString(k)), nil
}

func checkPassword(pw, enc string) bool {
	p := strings.Split(enc, "$")
	if len(p) != 4 || p[0] != "pbkdf2-sha256" {
		return false
	}
	iter, err1 := strconv.Atoi(p[1])
	salt, err2 := b64.DecodeString(p[2])
	want, err3 := b64.DecodeString(p[3])
	if err1 != nil || err2 != nil || err3 != nil || iter < 1 {
		return false
	}
	k, err := pbkdf2.Key(sha256.New, pw, salt, iter, len(want))
	return err == nil && subtle.ConstantTimeCompare(k, want) == 1
}

// dummyHash makes a login for an unknown username cost the same as a wrong password.
var dummyHash = sync.OnceValue(func() string { h, _ := hashPassword(rand.Text()); return h })

func validUsername(name string) error {
	if name == "" || len(name) > 32 || strings.ContainsFunc(name, unicode.IsSpace) {
		return errors.New("username must be 1-32 characters without spaces")
	}
	return nil
}

// addUser creates an account with its own copy of the built-in categories.
func addUser(db *sql.DB, name, pw string) (int64, error) {
	if err := validUsername(name); err != nil {
		return 0, err
	}
	h, err := hashPassword(pw)
	if err != nil {
		return 0, err
	}
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var id int64
	if err := tx.QueryRow(`INSERT INTO users (username, pass_hash) VALUES (?, ?) RETURNING id`, name, h).Scan(&id); err != nil {
		return 0, err
	}
	// the first account's built-ins are the template; its own in/out choices (in_budget) come along
	if _, err := tx.Exec(`INSERT INTO categories (user_id, name, type, in_budget)
		SELECT ?, name, type, in_budget FROM categories WHERE custom = 0 AND user_id = (SELECT MIN(id) FROM users)`, id); err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

// setPassword replaces a user's password and signs out all their devices.
func setPassword(db *sql.DB, name, pw string) error {
	h, err := hashPassword(pw)
	if err != nil {
		return err
	}
	var id int64
	if err := db.QueryRow(`UPDATE users SET pass_hash = ? WHERE username = ? RETURNING id`, h, name).Scan(&id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("no user %q", name)
		}
		return err
	}
	_, err = db.Exec(`DELETE FROM sessions WHERE user_id = ?`, id)
	return err
}

func tokenHash(tok string) string {
	h := sha256.Sum256([]byte(tok))
	return b64.EncodeToString(h[:])
}

// newToken issues an API token for a user and returns it; only its hash is stored.
// Tokens are random, so a plain SHA-256 (not a slow password hash) is enough.
func newToken(db *sql.DB, name string) (string, error) {
	var id int64
	if err := db.QueryRow(`SELECT id FROM users WHERE username = ?`, name).Scan(&id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", fmt.Errorf("no user %q", name)
		}
		return "", err
	}
	tok := "kt_" + rand.Text()
	_, err := db.Exec(`INSERT INTO api_tokens (user_id, hash) VALUES (?, ?)`, id, tokenHash(tok))
	return tok, err
}

// tokenUser returns the account behind an API token from api_tokens, or 0.
func (a *app) tokenUser(tok string) int64 {
	var uid int64
	a.db.QueryRow(`SELECT user_id FROM api_tokens WHERE hash = ?`, tokenHash(tok)).Scan(&uid)
	return uid
}

// bootstrapUsers claims the first account (created unclaimed by a migration, owning the data from
// before accounts existed) with APP_USER/APP_PASSWORD, hands it the sessions from before accounts
// existed, and returns its id: API_TOKEN calls act as this user.
func bootstrapUsers(db *sql.DB, username, password string) (int64, error) {
	var owner int64
	var hash string
	if err := db.QueryRow(`SELECT id, pass_hash FROM users ORDER BY id LIMIT 1`).Scan(&owner, &hash); err != nil {
		return 0, err
	}
	switch {
	case hash == "" && password == "":
		return 0, errors.New("first account not set up: set APP_USER and APP_PASSWORD in .env (or run `passwd owner`)")
	case hash == "":
		if username == "" {
			username = "owner"
		}
		if err := validUsername(username); err != nil {
			return 0, fmt.Errorf("APP_USER: %w", err)
		}
		if hash, err := hashPassword(password); err != nil {
			return 0, err
		} else if _, err := db.Exec(`UPDATE users SET username = ?, pass_hash = ? WHERE id = ?`, username, hash, owner); err != nil {
			return 0, fmt.Errorf("first user: %w", err)
		}
		slog.Info("created first user from APP_PASSWORD; remove APP_PASSWORD from .env", "username", username)
	case password != "":
		slog.Warn("APP_PASSWORD is ignored now that users exist; remove it from .env")
	}
	_, err := db.Exec(`UPDATE sessions SET user_id = ? WHERE user_id IS NULL`, owner)
	return owner, err
}

// cli runs account commands against DB_PATH; secrets are printed once and only their hashes stored.
func cli(args []string) int {
	if len(args) != 2 || !slices.Contains([]string{"adduser", "passwd", "token"}, args[0]) {
		fmt.Fprintln(os.Stderr, `usage: money-manager <command> <username>
  adduser  create an account; prints its password and an API token
  passwd   set a new random password; signs the account out everywhere
  token    issue another API token (for Hermes or scripts)`)
		return 2
	}
	db, err := openDB(env("DB_PATH", "/data/money.db"))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer db.Close()
	cmd, name := args[0], args[1]
	pw, tok := rand.Text(), ""
	switch cmd {
	case "adduser":
		if _, err = addUser(db, name, pw); err == nil {
			tok, err = newToken(db, name)
		}
	case "passwd":
		err = setPassword(db, name, pw)
	case "token":
		pw = ""
		tok, err = newToken(db, name)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Printf("username:  %s\n", name)
	if pw != "" {
		fmt.Printf("password:  %s\n", pw)
	}
	if tok != "" {
		fmt.Printf("api token: %s\n", tok)
	}
	return 0
}

// ponytail: in-memory per-username lockout, reset on restart; anyone can lock a known username for
// 15 minutes. Fine inside a tailnet; move to per-IP behind a trusted proxy if exposed publicly.
type limiter struct {
	mu sync.Mutex
	m  map[string]attempt
}

type attempt struct {
	n     int
	until time.Time
}

const (
	maxLoginFails = 5
	loginLockout  = 15 * time.Minute
)

func (l *limiter) blocked(k string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	a := l.m[k]
	return a.n >= maxLoginFails && time.Now().Before(a.until)
}

func (l *limiter) fail(k string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.m == nil {
		l.m = map[string]attempt{}
	}
	a := l.m[k]
	if time.Now().After(a.until) {
		a = attempt{}
	}
	a.n++
	a.until = time.Now().Add(loginLockout)
	l.m[k] = a
}

func (l *limiter) reset(k string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.m, k)
}

// userIDs lists every account, for background work that runs per account.
func (a *app) userIDs() []int64 {
	rows, err := a.db.Query(`SELECT id FROM users ORDER BY id`)
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
