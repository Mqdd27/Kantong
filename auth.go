package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	sessionCookie = "mm_session"
	sessionTTL    = 90 * 24 * time.Hour
)

// secretEq compares hashes so neither content nor length leaks through timing.
func secretEq(a, b string) bool {
	ha, hb := sha256.Sum256([]byte(a)), sha256.Sum256([]byte(b))
	return subtle.ConstantTimeCompare(ha[:], hb[:]) == 1
}

func (a *app) sign(exp string) string {
	m := hmac.New(sha256.New, []byte(a.cfg.sessionSecret))
	m.Write([]byte(exp))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

func (a *app) validSession(v string) bool {
	exp, sig, ok := strings.Cut(v, ".")
	if !ok || !hmac.Equal([]byte(sig), []byte(a.sign(exp))) {
		return false
	}
	n, err := strconv.ParseInt(exp, 10, 64)
	return err == nil && time.Now().Unix() < n
}

func (a *app) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if tok, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok && secretEq(tok, a.cfg.apiToken) {
			next.ServeHTTP(w, r)
			return
		}
		if c, err := r.Cookie(sessionCookie); err == nil && a.validSession(c.Value) {
			next.ServeHTTP(w, r)
			return
		}
		writeErr(w, http.StatusUnauthorized, "unauthorized")
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
	exp := strconv.FormatInt(time.Now().Add(sessionTTL).Unix(), 10)
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    exp + "." + a.sign(exp),
		Path:     "/",
		MaxAge:   int(sessionTTL.Seconds()),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
	w.WriteHeader(http.StatusNoContent)
}
