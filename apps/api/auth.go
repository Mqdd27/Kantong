package main

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"net"
	"net/http"
	"strings"
	"time"
)

const (
	sessionCookie = "mm_session"
	sessionTTL    = 90 * 24 * time.Hour
)

type sidKey struct{}
type uidKey struct{}

// sessionID is the id of the cookie session making the request; "" for Bearer API_TOKEN calls.
func sessionID(r *http.Request) string { s, _ := r.Context().Value(sidKey{}).(string); return s }

// userID is the account making the request; the .env API_TOKEN acts as the first user.
func userID(r *http.Request) int64 { u, _ := r.Context().Value(uidKey{}).(int64); return u }

// secretEq compares hashes so neither content nor length leaks through timing.
func secretEq(a, b string) bool {
	ha, hb := sha256.Sum256([]byte(a)), sha256.Sum256([]byte(b))
	return subtle.ConstantTimeCompare(ha[:], hb[:]) == 1
}

// sign keys the cookie token into the stored session id, so the table alone cannot forge a cookie.
func (a *app) sign(tok string) string {
	m := hmac.New(sha256.New, []byte(a.cfg.sessionSecret))
	m.Write([]byte(tok))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

// session returns the id and user of the live session behind the cookie, or "". Cookies from before
// sessions were stored (exp.sig) match no row, so every device signs in once after the upgrade.
func (a *app) session(r *http.Request) (string, int64) {
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" {
		return "", 0
	}
	id := a.sign(c.Value)
	var lastSeen string
	var uid int64
	if a.db.QueryRow(`SELECT last_seen, user_id FROM sessions WHERE id = ? AND expires_at > ? AND user_id IS NOT NULL`,
		id, time.Now().Unix()).Scan(&lastSeen, &uid) != nil {
		return "", 0
	}
	// last_seen is for the device list: minute precision is plenty and spares a write per request
	if now := time.Now().UTC(); lastSeen < now.Add(-time.Minute).Format(time.RFC3339) {
		a.db.Exec(`UPDATE sessions SET last_seen = ?, ip = ? WHERE id = ?`, now.Format(time.RFC3339), clientIP(r), id)
	}
	return id, uid
}

func clientIP(r *http.Request) string {
	if f := r.Header.Get("X-Forwarded-For"); f != "" { // display only, never trusted for auth
		ip, _, _ := strings.Cut(f, ",")
		return strings.TrimSpace(ip)
	}
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	return host
}

func (a *app) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if tok, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
			uid := a.tokenUser(tok)
			if secretEq(tok, a.cfg.apiToken) { // the .env token predates accounts: it is the first user's
				uid = a.ownerID
			}
			if uid != 0 {
				next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), uidKey{}, uid)))
				return
			}
		}
		if id, uid := a.session(r); id != "" {
			ctx := context.WithValue(context.WithValue(r.Context(), sidKey{}, id), uidKey{}, uid)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}
		writeErr(w, http.StatusUnauthorized, "unauthorized")
	})
}

func (a *app) setCookie(w http.ResponseWriter, val string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    val,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   !a.cfg.insecureCookie,
		SameSite: http.SameSiteStrictMode,
	})
}

func (a *app) login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decode(w, r, &body) {
		return
	}
	name := strings.TrimSpace(body.Username)
	key := strings.ToLower(name)
	if a.logins.blocked(key) {
		writeErr(w, http.StatusTooManyRequests, "too many failed attempts, try again in 15 minutes")
		return
	}
	var uid int64
	hash := dummyHash()
	found := a.db.QueryRow(`SELECT id, pass_hash FROM users WHERE username = ?`, name).Scan(&uid, &hash) == nil
	if !checkPassword(body.Password, hash) || !found {
		a.logins.fail(key)
		writeErr(w, http.StatusUnauthorized, "wrong username or password")
		return
	}
	a.logins.reset(key)
	tok := rand.Text()
	a.db.Exec(`DELETE FROM sessions WHERE expires_at <= ?`, time.Now().Unix())
	if _, err := a.db.Exec(`INSERT INTO sessions (id, user_id, user_agent, ip, expires_at) VALUES (?, ?, ?, ?, ?)`,
		a.sign(tok), uid, r.UserAgent(), clientIP(r), time.Now().Add(sessionTTL).Unix()); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.setCookie(w, tok, int(sessionTTL.Seconds()))
	w.WriteHeader(http.StatusNoContent)
}

// logout ends this device's session. Public and idempotent: without a session it only clears the cookie.
func (a *app) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" {
		a.endSessions(`id = ?`, a.sign(c.Value))
	}
	a.setCookie(w, "", -1)
	w.WriteHeader(http.StatusNoContent)
}

// endSessions deletes the matching sessions and drops their open SSE streams.
func (a *app) endSessions(where string, args ...any) (int, error) {
	rows, err := a.db.Query(`DELETE FROM sessions WHERE `+where+` RETURNING id`, args...)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	n := 0
	for ; rows.Next(); n++ {
		var id string
		rows.Scan(&id)
		a.hub.kick(id)
	}
	return n, rows.Err()
}

type Session struct {
	ID        string `json:"id"`
	UserAgent string `json:"user_agent"`
	IP        string `json:"ip"`
	CreatedAt string `json:"created_at"`
	LastSeen  string `json:"last_seen"`
	Current   bool   `json:"current"`
}

func (a *app) listSessions(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(`SELECT id, user_agent, ip, created_at, last_seen FROM sessions WHERE user_id = ? AND expires_at > ? ORDER BY last_seen DESC`,
		userID(r), time.Now().Unix())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	out := []Session{}
	for rows.Next() {
		var s Session
		if err := rows.Scan(&s.ID, &s.UserAgent, &s.IP, &s.CreatedAt, &s.LastSeen); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.Current = s.ID == sessionID(r)
		out = append(out, s)
	}
	writeJSON(w, http.StatusOK, out)
}

// deleteSession signs out one device. Ending your own session also clears your cookie.
func (a *app) deleteSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	n, err := a.endSessions(`id = ? AND user_id = ?`, id, userID(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if n == 0 {
		writeErr(w, http.StatusNotFound, "session not found")
		return
	}
	if id == sessionID(r) {
		a.setCookie(w, "", -1)
	}
	w.WriteHeader(http.StatusNoContent)
}

// revokeSessions signs out every other device of the caller's account (all of the first user's
// when called with API_TOKEN). API_TOKEN itself is untouched: rotate it in .env to cut off scripts.
func (a *app) revokeSessions(w http.ResponseWriter, r *http.Request) {
	if _, err := a.endSessions(`id != ? AND user_id = ?`, sessionID(r), userID(r)); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
