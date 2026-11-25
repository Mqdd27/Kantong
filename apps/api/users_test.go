package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
)

func TestPasswordHash(t *testing.T) {
	h, err := hashPassword("rahasia")
	if err != nil || !strings.HasPrefix(h, "pbkdf2-sha256$600000$") {
		t.Fatalf("hash %q %v", h, err)
	}
	if !checkPassword("rahasia", h) || checkPassword("rahasia!", h) || checkPassword("rahasia", "garbage") {
		t.Fatal("checkPassword")
	}
	if h2, _ := hashPassword("rahasia"); h2 == h {
		t.Fatal("salt not random")
	}
}

func TestBootstrapUsers(t *testing.T) {
	db, err := openDB(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := bootstrapUsers(db, "", ""); err == nil {
		t.Fatal("empty install without APP_PASSWORD must fail")
	}
	// a device signed in before accounts existed is handed to the first user
	db.Exec(`INSERT INTO sessions (id, expires_at) VALUES ('old', 4102444800)`)
	id, err := bootstrapUsers(db, "", "pw")
	if err != nil {
		t.Fatal(err)
	}
	var name, hash string
	var sessUser sql.NullInt64
	db.QueryRow(`SELECT username, pass_hash FROM users WHERE id = ?`, id).Scan(&name, &hash)
	db.QueryRow(`SELECT user_id FROM sessions WHERE id = 'old'`).Scan(&sessUser)
	if name != "owner" || hash == "pw" || !checkPassword("pw", hash) || sessUser.Int64 != id {
		t.Fatalf("user %q hash %q session user %v", name, hash, sessUser)
	}
	// later starts keep the stored hash; APP_PASSWORD no longer changes anything
	if id2, err := bootstrapUsers(db, "other", "new"); err != nil || id2 != id {
		t.Fatalf("restart: %d %v", id2, err)
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n)
	if n != 1 {
		t.Fatalf("users: %d", n)
	}
}

func TestLoginPerUser(t *testing.T) {
	a, srv := newTestApp(t)
	if _, err := addUser(a.db, "budi", "pw2"); err != nil {
		t.Fatal(err)
	}
	if _, err := addUser(a.db, "BUDI", "x"); err == nil {
		t.Fatal("usernames must be unique ignoring case")
	}
	if _, err := addUser(a.db, "a b", "x"); err == nil {
		t.Fatal("spaces in username")
	}
	own, budi := loginAs(t, srv, "Mac"), loginUser(t, srv, "Budi", "pw2", "Pixel")
	loginAs(t, srv, "iPhone")

	// each account sees and signs out only its own devices
	list := func(c *http.Cookie) []Session {
		_, res := as(t, srv, c, "GET", "/api/sessions")
		var l []Session
		json.NewDecoder(res.Body).Decode(&l)
		return l
	}
	if l := list(budi); len(l) != 1 || l[0].UserAgent != "Pixel" {
		t.Fatalf("budi sessions: %+v", l)
	}
	ownID := list(own)[0].ID
	if code, _ := as(t, srv, budi, "DELETE", "/api/sessions/"+ownID); code != 404 {
		t.Fatalf("budi deleted owner's session: %d", code)
	}
	as(t, srv, budi, "POST", "/api/sessions/revoke")
	if len(list(own)) != 2 {
		t.Fatal("budi's revoke touched owner's devices")
	}

	// passwd replaces the password and signs that user out everywhere
	if err := setPassword(a.db, "budi", "pw3"); err != nil {
		t.Fatal(err)
	}
	if code, _ := as(t, srv, budi, "GET", "/api/categories"); code != 401 {
		t.Fatalf("old session after passwd: %d", code)
	}
	loginUser(t, srv, "budi", "pw3", "Pixel")
	if err := setPassword(a.db, "nobody", "x"); err == nil {
		t.Fatal("passwd for unknown user")
	}
}

func TestLoginRateLimit(t *testing.T) {
	_, srv := newTestApp(t)
	post := func(user, pw string) int {
		body, _ := json.Marshal(map[string]string{"username": user, "password": pw})
		res, err := http.Post(srv.URL+"/api/login", "application/json", strings.NewReader(string(body)))
		if err != nil {
			t.Fatal(err)
		}
		return res.StatusCode
	}
	if post("ghost", "pw") != 401 {
		t.Fatal("unknown user")
	}
	for i := 0; i < maxLoginFails-1; i++ {
		post("owner", "wrong")
	}
	if post("owner", "wrong") != 401 || post("OWNER", "pw") != 429 {
		t.Fatal("lockout after 5 failures, ignoring case, even with the right password")
	}
	if post("ghost", "pw") != 401 {
		t.Fatal("lockout is per username")
	}
}

func TestChangePassword(t *testing.T) {
	_, srv := newTestApp(t)
	put := func(c *http.Cookie, tok, body string) int {
		t.Helper()
		req, _ := http.NewRequest("PUT", srv.URL+"/api/me/password", strings.NewReader(body))
		if c != nil {
			req.AddCookie(c)
		}
		if tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		return res.StatusCode
	}
	mac, phone := loginAs(t, srv, "Mac"), loginAs(t, srv, "iPhone")

	for _, c := range []struct {
		c    *http.Cookie
		tok  string
		body string
		want int
	}{
		{nil, token, `{"current":"pw","new":"longenough"}`, 403}, // API token, not a device
		{mac, "", `{"current":"pw","new":"short"}`, 400},
		{mac, "", `{"current":"nope","new":"longenough"}`, 403}, // 403, not 401: a typo must not sign out
		{mac, "", `{"current":"pw","new":"longenough"}`, 204},
	} {
		if got := put(c.c, c.tok, c.body); got != c.want {
			t.Fatalf("%s: %d, want %d", c.body, got, c.want)
		}
	}
	if code, _ := as(t, srv, mac, "GET", "/api/me"); code != 200 {
		t.Fatalf("this device signed out: %d", code)
	}
	if code, _ := as(t, srv, phone, "GET", "/api/me"); code != 401 {
		t.Fatalf("other device still signed in: %d", code)
	}
	loginUser(t, srv, "owner", "longenough", "Pixel")

	// wrong current passwords share the login lockout
	for range maxLoginFails {
		put(mac, "", `{"current":"nope","new":"longenough"}`)
	}
	if got := put(mac, "", `{"current":"longenough","new":"another1"}`); got != 429 {
		t.Fatalf("lockout: %d", got)
	}
}
