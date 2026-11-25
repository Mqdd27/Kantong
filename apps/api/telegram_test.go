package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTelegramPerAccount(t *testing.T) {
	got := make(chan string, 8)
	tg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		if strings.Contains(r.URL.Path, "BAD") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		got <- r.URL.Path + "|" + r.Form.Get("chat_id")
	}))
	defer tg.Close()
	a, err := newApp(config{dbPath: filepath.Join(t.TempDir(), "t.db"), apiToken: token, appPassword: "pw",
		sessionSecret: strings.Repeat("s", 32), tgAPI: tg.URL, tgToken: "1:ENVENVENVENVENVENVENVENV", tgChat: "42"})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(a.routes())
	defer func() { srv.Close(); a.db.Close() }()
	addUser(a.db, "budi", "pw2")
	budi, _ := newToken(a.db, "budi")
	next := func() string {
		t.Helper()
		select {
		case m := <-got:
			return m
		case <-time.After(2 * time.Second):
			return "none"
		}
	}
	mail := func(tok, ref string) {
		t.Helper()
		if code, raw := callTok(t, srv, tok, "POST", "/api/transactions", `{"amount":1000,"type":"out","category":"Makan","source":"email","source_ref":"`+ref+`"}`, nil); code != 201 {
			t.Fatalf("mail: %d %s", code, raw)
		}
	}

	// the first account still uses the .env bot; budi has none, so budi's mail stays silent
	if _, raw := callTok(t, srv, token, "GET", "/api/me", "", nil); !strings.Contains(raw, `"username":"owner"`) || !strings.Contains(raw, `"from_env":true`) {
		t.Fatalf("owner me: %s", raw)
	}
	mail(token, "a1")
	if m := next(); m != "/bot1:ENVENVENVENVENVENVENVENV/sendMessage|42" {
		t.Fatalf("owner notify: %s", m)
	}
	mail(budi, "b1")
	if m := next(); m != "none" {
		t.Fatalf("budi without a bot was notified: %s", m)
	}

	// bad input and a token Telegram rejects are reported, not stored
	for body, want := range map[string]string{
		`{"bot_token":"nope","chat_id":"7"}`:                       "bot token looks wrong",
		`{"bot_token":"9:BUDIBUDIBUDIBUDIBUDIBUDI","chat_id":"x"}`: "chat id must be a number",
		`{"bot_token":"9:BADBADBADBADBADBADBADBAD","chat_id":"7"}`: "test message failed",
	} {
		if code, raw := callTok(t, srv, budi, "PUT", "/api/me/telegram", body, nil); code != 400 || !strings.Contains(raw, want) {
			t.Fatalf("%s: %d %s", body, code, raw)
		}
	}

	// budi connects a bot: a test message goes out, the token is stored encrypted and never returned
	code, raw := callTok(t, srv, budi, "PUT", "/api/me/telegram", `{"bot_token":"9:BUDIBUDIBUDIBUDIBUDIBUDI","chat_id":"7"}`, nil)
	if code != 200 || !strings.Contains(raw, `"chat_id":"7"`) || strings.Contains(raw, "BUDI") {
		t.Fatalf("connect: %d %s", code, raw)
	}
	if m := next(); m != "/bot9:BUDIBUDIBUDIBUDIBUDIBUDI/sendMessage|7" {
		t.Fatalf("test message: %s", m)
	}
	var stored string
	a.db.QueryRow(`SELECT tg_token FROM users WHERE username = 'budi'`).Scan(&stored)
	if stored == "" || strings.Contains(stored, "BUDI") {
		t.Fatalf("token at rest: %q", stored)
	}
	mail(budi, "b2")
	if m := next(); m != "/bot9:BUDIBUDIBUDIBUDIBUDIBUDI/sendMessage|7" {
		t.Fatalf("budi notify: %s", m)
	}
	mail(token, "a2") // the owner's mail still goes to the owner's bot
	if m := next(); m != "/bot1:ENVENVENVENVENVENVENVENV/sendMessage|42" {
		t.Fatalf("owner notify after budi connected: %s", m)
	}

	// a changed SESSION_SECRET can't decrypt the token: notifications stop instead of failing loudly
	a.cfg.sessionSecret = strings.Repeat("x", 32)
	if tok, _ := a.telegramFor(a.ownerID + 1); tok != "" {
		t.Fatal("decrypted with the wrong key")
	}
	a.cfg.sessionSecret = strings.Repeat("s", 32)

	// a chat id alone uses the server's bot: budi never needs its token
	code, raw = callTok(t, srv, budi, "PUT", "/api/me/telegram", `{"bot_token":"","chat_id":"8"}`, nil)
	if code != 200 || !strings.Contains(raw, `"chat_id":"8"`) || !strings.Contains(raw, `"own_bot":false`) || !strings.Contains(raw, `"from_env":false`) {
		t.Fatalf("shared bot: %d %s", code, raw)
	}
	if m := next(); m != "/bot1:ENVENVENVENVENVENVENVENV/sendMessage|8" {
		t.Fatalf("shared bot test message: %s", m)
	}
	mail(budi, "b4")
	if m := next(); m != "/bot1:ENVENVENVENVENVENVENVENV/sendMessage|8" {
		t.Fatalf("budi via shared bot: %s", m)
	}
	mail(token, "a3") // the owner's chat is unchanged
	if m := next(); m != "/bot1:ENVENVENVENVENVENVENVENV/sendMessage|42" {
		t.Fatalf("owner after budi shared: %s", m)
	}
	// without a server bot, a chat id alone is refused and a stored one stops counting
	a.cfg.tgToken = ""
	if code, raw := callTok(t, srv, budi, "PUT", "/api/me/telegram", `{"bot_token":"","chat_id":"8"}`, nil); code != 400 || !strings.Contains(raw, "no shared bot") {
		t.Fatalf("no server bot: %d %s", code, raw)
	}
	if _, raw := callTok(t, srv, budi, "GET", "/api/me", "", nil); !strings.Contains(raw, `"connected":false`) {
		t.Fatalf("server bot removed: %s", raw)
	}
	a.cfg.tgToken = "1:ENVENVENVENVENVENVENVENV"

	// disconnecting clears it
	if code, raw := callTok(t, srv, budi, "PUT", "/api/me/telegram", `{"bot_token":"","chat_id":""}`, nil); code != 200 || !strings.Contains(raw, `"connected":false`) {
		t.Fatalf("disconnect: %d %s", code, raw)
	}
	mail(budi, "b3")
	if m := next(); m != "none" {
		t.Fatalf("notified after disconnect: %s", m)
	}
}
