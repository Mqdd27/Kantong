package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
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

	var first struct{ Session chatSession }
	if code := call(t, srv, "POST", "/api/chat", `{"text":"bulan ini gimana?"}`, &first); code != 200 || first.Session.Title != "bulan ini gimana?" {
		t.Fatalf("first POST = %d %+v", code, first)
	}
	sid := strconv.FormatInt(first.Session.ID, 10)
	if code := call(t, srv, "POST", "/api/chat", `{"text":"lalu?","model":"9router:cx/gpt-5.5","session_id":`+sid+`}`, nil); code != 200 {
		t.Fatalf("second POST = %d", code)
	}
	// a new session starts with no history from the first one
	if code := call(t, srv, "POST", "/api/chat", `{"text":"topik lain"}`, nil); code != 200 {
		t.Fatalf("new session POST = %d", code)
	}
	if n := len(seen[2]["messages"].([]any)); n != 2 { // system + new only
		t.Fatalf("new session carried %d messages, want 2", n)
	}
	if code := call(t, srv, "POST", "/api/chat", `{"text":"x","session_id":999}`, nil); code != 404 {
		t.Fatalf("missing session = %d, want 404", code)
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
	var list struct {
		Enabled  bool
		Sessions []chatSession
		Models   []string
	}
	call(t, srv, "GET", "/api/chat", "", &list)
	if !list.Enabled || len(list.Sessions) != 2 || len(list.Models) != 2 {
		t.Fatalf("session list: %+v", list)
	}
	var got struct{ Messages []chatMsg }
	call(t, srv, "GET", "/api/chat/"+sid, "", &got)
	if len(got.Messages) != 4 || got.Messages[3].Model != "9router:cx/gpt-5.5" {
		t.Fatalf("session messages: %+v", got)
	}
	if code := call(t, srv, "DELETE", "/api/chat/"+sid, "", nil); code != 204 {
		t.Fatalf("DELETE = %d", code)
	}
	call(t, srv, "GET", "/api/chat/"+sid, "", &got)
	if len(got.Messages) != 0 {
		t.Fatalf("messages survived delete: %d", len(got.Messages))
	}
}
