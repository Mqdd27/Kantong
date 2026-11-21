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
	return loginUser(t, srv, "owner", "pw", ua)
}

func loginUser(t *testing.T, srv *httptest.Server, user, pw, ua string) *http.Cookie {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"username": user, "password": pw})
	req, _ := http.NewRequest("POST", srv.URL+"/api/login", strings.NewReader(string(body)))
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
