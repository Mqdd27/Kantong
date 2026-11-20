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

// sessionID is the id of the cookie session making the request; "" for Bearer API_TOKEN calls.
func sessionID(r *http.Request) string { s, _ := r.Context().Value(sidKey{}).(string); return s }

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

// session returns the id of the live session behind the cookie, or "". Cookies from before
// sessions were stored (exp.sig) match no row, so every device signs in once after the upgrade.
func (a *app) session(r *http.Request) string {
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" {
		return ""
	}
	id := a.sign(c.Value)
	var lastSeen string
	if a.db.QueryRow(`SELECT last_seen FROM sessions WHERE id = ? AND expires_at > ?`, id, time.Now().Unix()).Scan(&lastSeen) != nil {
		return ""
	}
	// last_seen is for the device list: minute precision is plenty and spares a write per request
	if now := time.Now().UTC(); lastSeen < now.Add(-time.Minute).Format(time.RFC3339) {
		a.db.Exec(`UPDATE sessions SET last_seen = ?, ip = ? WHERE id = ?`, now.Format(time.RFC3339), clientIP(r), id)
	}
	return id
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
		if tok, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok && secretEq(tok, a.cfg.apiToken) {
			next.ServeHTTP(w, r)
			return
		}
		if id := a.session(r); id != "" {
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), sidKey{}, id)))
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

// ponytail: no login rate limit; API is only reachable inside the tailnet. Add one if exposed publicly.
func (a *app) login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Password string `json:"password"`
	}
	if !decode(w, r, &body) {
		return
	}
	if !secretEq(body.Password, a.cfg.appPassword) {
		writeErr(w, http.StatusUnauthorized, "wrong password")
		return
	}
	tok := rand.Text()
	a.db.Exec(`DELETE FROM sessions WHERE expires_at <= ?`, time.Now().Unix())
	if _, err := a.db.Exec(`INSERT INTO sessions (id, user_agent, ip, expires_at) VALUES (?, ?, ?, ?)`,
		a.sign(tok), r.UserAgent(), clientIP(r), time.Now().Add(sessionTTL).Unix()); err != nil {
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
	rows, err := a.db.Query(`SELECT id, user_agent, ip, created_at, last_seen FROM sessions WHERE expires_at > ? ORDER BY last_seen DESC`, time.Now().Unix())
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
	n, err := a.endSessions(`id = ?`, id)
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

// revokeSessions signs out every device except the caller's (all of them when called with API_TOKEN).
// API_TOKEN itself is untouched: rotate it in .env to cut off scripts.
func (a *app) revokeSessions(w http.ResponseWriter, r *http.Request) {
	if _, err := a.endSessions(`id != ?`, sessionID(r)); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

