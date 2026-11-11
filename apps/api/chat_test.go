package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestChatOffByDefault(t *testing.T) {
	srv := newTestServer(t)
	var got struct{ Enabled bool }
	if code := call(t, srv, "GET", "/api/chat", "", &got); code != 200 || got.Enabled {
		t.Fatalf("GET /api/chat = %d %+v, want 200 disabled", code, got)
	}
	if code := call(t, srv, "POST", "/api/chat", `{"text":"hai"}`, nil); code != 404 {
		t.Fatalf("POST /api/chat = %d, want 404", code)
	}
}

func TestChatViaHermes(t *testing.T) {
	var seen []map[string]any
	hermes := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer hk" {
			t.Errorf("bad request: %s auth=%q", r.URL.Path, r.Header.Get("Authorization"))
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		seen = append(seen, body)
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"Pengeluaranmu aman."}}]}`))
	}))
	defer hermes.Close()

	a, err := newApp(config{dbPath: filepath.Join(t.TempDir(), "t.db"), apiToken: token, sessionSecret: strings.Repeat("s", 32),
		hermesURL: hermes.URL, hermesKey: "hk", chatModels: "9router:cc/claude-sonnet-5-5, 9router:cx/gpt-5.5"})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(a.routes())
	defer func() { srv.Close(); a.db.Close() }()

	if code := call(t, srv, "POST", "/api/chat", `{"text":"bulan ini gimana?"}`, nil); code != 200 {
		t.Fatalf("first POST = %d", code)
	}
	if code := call(t, srv, "POST", "/api/chat", `{"text":"lalu?","model":"9router:cx/gpt-5.5"}`, nil); code != 200 {
		t.Fatalf("second POST = %d", code)
	}
	if code := call(t, srv, "POST", "/api/chat", `{"text":"x","model":"evil:model"}`, nil); code != 400 {
		t.Fatalf("unknown model = %d, want 400", code)
	}
	if seen[0]["provider"] != "9router" || seen[0]["model"] != "cc/claude-sonnet-5-5" || seen[1]["model"] != "cx/gpt-5.5" {
		t.Fatalf("provider/model not passed through: %v / %v", seen[0]["model"], seen[1]["model"])
	}
	if n := len(seen[1]["messages"].([]any)); n != 4 { // system + 2 history + new
		t.Fatalf("second request carried %d messages, want 4", n)
	}
	var got struct {
		Enabled  bool
		Messages []chatMsg
		Models   []string
	}
	call(t, srv, "GET", "/api/chat", "", &got)
	if !got.Enabled || len(got.Messages) != 4 || got.Messages[3].Model != "9router:cx/gpt-5.5" || len(got.Models) != 2 {
		t.Fatalf("after chat: %+v", got)
	}
}
