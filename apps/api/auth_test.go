package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func loginAs(t *testing.T, srv *httptest.Server, ua string) *http.Cookie {
	t.Helper()
	req, _ := http.NewRequest("POST", srv.URL+"/api/login", strings.NewReader(`{"password":"pw"}`))
	req.Header.Set("User-Agent", ua)
	res, err := http.DefaultClient.Do(req)
	if err != nil || res.StatusCode != 204 || len(res.Cookies()) != 1 {
		t.Fatalf("login %s: %v %v", ua, err, res)
	}
	return res.Cookies()[0]
}

// as sends a request with a session cookie (nil = no auth at all) and returns the status and response.
func as(t *testing.T, srv *httptest.Server, c *http.Cookie, method, path string) (int, *http.Response) {
	t.Helper()
	req, _ := http.NewRequest(method, srv.URL+path, nil)
	if c != nil {
		req.AddCookie(c)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return res.StatusCode, res
}

func TestSessions(t *testing.T) {
	srv := newTestServer(t)
	mac, phone, pad := loginAs(t, srv, "Mac"), loginAs(t, srv, "iPhone"), loginAs(t, srv, "iPad")

	// the device list marks the caller's own session
	_, res := as(t, srv, mac, "GET", "/api/sessions")
	var list []Session
	json.NewDecoder(res.Body).Decode(&list)
	if len(list) != 3 {
		t.Fatalf("sessions: %+v", list)
	}
	var phoneID string
	for _, s := range list {
		if s.Current != (s.UserAgent == "Mac") {
			t.Fatalf("current flag: %+v", s)
		}
		if s.UserAgent == "iPhone" {
			phoneID = s.ID
		}
	}

	// tampered and pre-upgrade (exp.sig) cookies are rejected
	for _, v := range []string{mac.Value + "x", "4102444800.abc"} {
		if code, _ := as(t, srv, &http.Cookie{Name: sessionCookie, Value: v}, "GET", "/api/categories"); code != 401 {
			t.Fatalf("cookie %q: %d", v, code)
		}
	}

	// signing out the phone from the Mac also drops the phone's open SSE stream
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL+"/api/events", nil)
	req.AddCookie(phone)
	stream, err := http.DefaultClient.Do(req)
	if err != nil || stream.StatusCode != 200 {
		t.Fatalf("events: %v", err)
	}
	if code, _ := as(t, srv, mac, "DELETE", "/api/sessions/"+phoneID); code != 204 {
		t.Fatalf("delete phone: %d", code)
	}
	if _, err := io.ReadAll(stream.Body); err != nil { // returns once the server closes the stream
		t.Fatalf("stream not closed: %v", err)
	}
	if code, _ := as(t, srv, phone, "GET", "/api/categories"); code != 401 {
		t.Fatalf("phone still signed in: %d", code)
	}
	if code, _ := as(t, srv, mac, "DELETE", "/api/sessions/"+phoneID); code != 404 {
		t.Fatalf("delete twice: %d", code)
	}

	// revoke signs out every other device but keeps the caller and API_TOKEN
	if code, _ := as(t, srv, mac, "POST", "/api/sessions/revoke"); code != 204 {
		t.Fatalf("revoke: %d", code)
	}
	if code, _ := as(t, srv, pad, "GET", "/api/categories"); code != 401 {
		t.Fatalf("pad survived revoke: %d", code)
	}
	if code, _ := as(t, srv, mac, "GET", "/api/categories"); code != 200 {
		t.Fatalf("caller lost its session: %d", code)
	}

	// logout clears the cookie, ends the session, and is safe to repeat without one
	code, res := as(t, srv, mac, "POST", "/api/logout")
	if code != 204 || len(res.Cookies()) != 1 || res.Cookies()[0].MaxAge >= 0 {
		t.Fatalf("logout: %d %v", code, res.Cookies())
	}
	if code, _ := as(t, srv, mac, "GET", "/api/categories"); code != 401 {
		t.Fatalf("mac still signed in: %d", code)
	}
	if code, _ := as(t, srv, nil, "POST", "/api/logout"); code != 204 {
		t.Fatalf("logout without session: %d", code)
	}
	if code := call(t, srv, "GET", "/api/categories", "", nil); code != 200 {
		t.Fatalf("API_TOKEN after revoke: %d", code)
	}
}

func TestExpiredSessions(t *testing.T) {
	a, srv := newTestApp(t)
	live, old := loginAs(t, srv, "Live"), loginAs(t, srv, "Old")
	a.db.Exec(`UPDATE sessions SET expires_at = ? WHERE user_agent = 'Old'`, time.Now().Add(-time.Second).Unix())
	var exp int64
	a.db.QueryRow(`SELECT expires_at FROM sessions WHERE user_agent = 'Live'`).Scan(&exp)

	// an expired session is refused even before it is pruned
	if code, _ := as(t, srv, old, "GET", "/api/categories"); code != 401 {
		t.Fatalf("expired session: %d", code)
	}
	if code, _ := as(t, srv, live, "GET", "/api/categories"); code != 200 {
		t.Fatalf("live session: %d", code)
	}

	// pruning removes only the expired row; the live one keeps its expiry and stays signed in
	if n, err := a.pruneSessions(time.Now()); n != 1 || err != nil {
		t.Fatalf("prune: %d %v", n, err)
	}
	var ua string
	var exp2 int64
	var n int
	a.db.QueryRow(`SELECT COUNT(*), MAX(user_agent), MAX(expires_at) FROM sessions`).Scan(&n, &ua, &exp2)
	if n != 1 || ua != "Live" || exp2 != exp {
		t.Fatalf("after prune: %d %q %d (want 1 Live %d)", n, ua, exp2, exp)
	}
	if code, _ := as(t, srv, live, "GET", "/api/categories"); code != 200 {
		t.Fatalf("live after prune: %d", code)
	}
	if n, _ := a.pruneSessions(time.Now()); n != 0 {
		t.Fatalf("second prune deleted %d", n)
	}
}

func TestLastSeenOncePerMinute(t *testing.T) {
	a, srv := newTestApp(t)
	c := loginAs(t, srv, "Mac")
	seen := func() (s string) { a.db.QueryRow(`SELECT last_seen FROM sessions`).Scan(&s); return }
	set := func(ago time.Duration) string {
		v := time.Now().UTC().Add(-ago).Format(time.RFC3339)
		a.db.Exec(`UPDATE sessions SET last_seen = ?`, v)
		return v
	}

	// seen 30s ago: a request doesn't write
	if v := set(30 * time.Second); as2(t, srv, c) && seen() != v {
		t.Fatalf("updated within a minute: %s -> %s", v, seen())
	}
	// seen 2 min ago: a request moves it forward
	if v := set(2 * time.Minute); as2(t, srv, c) && seen() <= v {
		t.Fatalf("not updated after a minute: %s -> %s", v, seen())
	}
}

// as2 makes one authenticated request and reports whether it succeeded.
func as2(t *testing.T, srv *httptest.Server, c *http.Cookie) bool {
	t.Helper()
	if code, _ := as(t, srv, c, "GET", "/api/categories"); code != 200 {
		t.Fatalf("request: %d", code)
	}
	return true
}
