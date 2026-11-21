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

func addUser(db *sql.DB, name, pw string) (int64, error) {
	if err := validUsername(name); err != nil {
		return 0, err
	}
	h, err := hashPassword(pw)
	if err != nil {
		return 0, err
	}
	res, err := db.Exec(`INSERT INTO users (username, pass_hash) VALUES (?, ?)`, name, h)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
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

// bootstrapUsers creates the first account from APP_PASSWORD on an install without users, hands it
// the sessions from before accounts existed, and returns its id: API_TOKEN calls act as this user.
func bootstrapUsers(db *sql.DB, username, password string) (int64, error) {
	var owner sql.NullInt64
	db.QueryRow(`SELECT MIN(id) FROM users`).Scan(&owner)
	switch {
	case !owner.Valid && password == "":
		return 0, errors.New("no users yet: set APP_PASSWORD (and APP_USER) for the first account, or run `adduser`")
	case !owner.Valid:
		if username == "" {
			username = "owner"
		}
		id, err := addUser(db, username, password)
		if err != nil {
			return 0, fmt.Errorf("first user: %w", err)
		}
		owner.Int64 = id
		slog.Info("created first user from APP_PASSWORD; remove APP_PASSWORD from .env", "username", username)
	case password != "":
		slog.Warn("APP_PASSWORD is ignored now that users exist; remove it from .env")
	}
	_, err := db.Exec(`UPDATE sessions SET user_id = ? WHERE user_id IS NULL`, owner.Int64)
	return owner.Int64, err
}

// cli runs `adduser <name>` / `passwd <name>` against DB_PATH and prints a generated password.
func cli(args []string) int {
	if len(args) != 2 || (args[0] != "adduser" && args[0] != "passwd") {
		fmt.Fprintln(os.Stderr, "usage: money-manager adduser|passwd <username>")
		return 2
	}
	db, err := openDB(env("DB_PATH", "/data/money.db"))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer db.Close()
	pw := rand.Text()
	if args[0] == "adduser" {
		_, err = addUser(db, args[1], pw)
	} else {
		err = setPassword(db, args[1], pw)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Printf("username: %s\npassword: %s\n", args[1], pw)
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
