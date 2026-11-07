package main

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const token = "test-token"

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	a, err := newApp(config{
		dbPath:        filepath.Join(t.TempDir(), "t.db"),
		apiToken:      token,
		appPassword:   "pw",
		sessionSecret: strings.Repeat("s", 32),
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(a.routes())
	t.Cleanup(func() { srv.Close(); a.db.Close() })
	return srv
}

func call(t *testing.T, srv *httptest.Server, method, path, body string, out any) int {
	t.Helper()
	req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if out != nil {
		json.NewDecoder(res.Body).Decode(out)
	}
	return res.StatusCode
}

const emailTx = `{"occurred_at":"2026-11-07T12:30:00+07:00","amount":25000,"type":"out","category":"Jajan",
	"merchant":"Indomaret","account":"BCA","note":"","source":"email","source_ref":"<m1@bca>","confidence":0.92,"needs_review":false}`

func TestCreateDuplicateAndList(t *testing.T) {
	srv := newTestServer(t)
	var created struct{ ID int64 }
	if code := call(t, srv, "POST", "/api/transactions", emailTx, &created); code != 201 || created.ID == 0 {
		t.Fatalf("create: %d %+v", code, created)
	}
	var dup struct{ ID int64 }
	if code := call(t, srv, "POST", "/api/transactions", emailTx, &dup); code != 409 || dup.ID != created.ID {
		t.Fatalf("duplicate: %d %+v", code, dup)
	}
	if code := call(t, srv, "POST", "/api/transactions", strings.Replace(emailTx, "Jajan", "Nope", 1), nil); code != 400 {
		t.Fatalf("bad category: %d", code)
	}
	var list []Tx
	call(t, srv, "GET", "/api/transactions?month=2026-11", "", &list)
	if len(list) != 1 || list[0].Merchant != "Indomaret" {
		t.Fatalf("list: %+v", list)
	}
	var sum struct{ Out int64 }
	call(t, srv, "GET", "/api/summary?month=2026-11", "", &sum)
	if sum.Out != 25000 {
		t.Fatalf("summary: %+v", sum)
	}
}

func TestCheckRefs(t *testing.T) {
	srv := newTestServer(t)
	call(t, srv, "POST", "/api/transactions", emailTx, nil)
	var res struct{ Unknown []string }
	call(t, srv, "POST", "/api/source-refs/check", `{"refs":["<m1@bca>","<m2@bca>"]}`, &res)
	if len(res.Unknown) != 1 || res.Unknown[0] != "<m2@bca>" {
		t.Fatalf("unknown: %+v", res.Unknown)
	}
}

func TestAuth(t *testing.T) {
	srv := newTestServer(t)
	res, _ := http.Get(srv.URL + "/api/categories")
	if res.StatusCode != 401 {
		t.Fatalf("no token: %d", res.StatusCode)
	}
	if res, _ := http.Get(srv.URL + "/healthz"); res.StatusCode != 200 {
		t.Fatalf("healthz: %d", res.StatusCode)
	}
	if res, _ := http.Post(srv.URL+"/api/login", "application/json", strings.NewReader(`{"password":"x"}`)); res.StatusCode != 401 {
		t.Fatalf("bad login: %d", res.StatusCode)
	}
	res, _ = http.Post(srv.URL+"/api/login", "application/json", strings.NewReader(`{"password":"pw"}`))
	if res.StatusCode != 204 || len(res.Cookies()) != 1 {
		t.Fatalf("login: %d", res.StatusCode)
	}
	req, _ := http.NewRequest("GET", srv.URL+"/api/categories", nil)
	req.AddCookie(res.Cookies()[0])
	if res, _ := http.DefaultClient.Do(req); res.StatusCode != 200 {
		t.Fatalf("cookie auth: %d", res.StatusCode)
	}
}

func TestSSE(t *testing.T) {
	srv := newTestServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL+"/api/events", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	sc := bufio.NewScanner(res.Body)
	sc.Scan() // ": connected" — subscribed before we create
	call(t, srv, "POST", "/api/transactions", emailTx, nil)
	for sc.Scan() {
		if sc.Text() == "event: transaction.created" {
			sc.Scan()
			if !strings.Contains(sc.Text(), `"merchant":"Indomaret"`) {
				t.Fatalf("data: %s", sc.Text())
			}
			return
		}
	}
	t.Fatal("no event received")
}
