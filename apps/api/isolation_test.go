package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// callTok is call with a given API token; out may be nil. Returns status and raw body.
func callTok(t *testing.T, srv *httptest.Server, tok, method, path, body string, out any) (int, string) {
	t.Helper()
	req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+tok)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if out != nil {
		json.Unmarshal(raw, out)
	}
	return res.StatusCode, string(raw)
}

// TestIsolation: everything the first account stores is invisible and untouchable for a second one,
// on every endpoint, and the second account can reuse the same names and refs.
func TestIsolation(t *testing.T) {
	a, srv := newTestApp(t)
	if _, err := addUser(a.db, "budi", "pw2"); err != nil {
		t.Fatal(err)
	}
	budi, err := newToken(a.db, "budi")
	if err != nil {
		t.Fatal(err)
	}
	own := func(method, path, body string) (int, string) { return callTok(t, srv, token, method, path, body, nil) }
	other := func(method, path, body string) (int, string) { return callTok(t, srv, budi, method, path, body, nil) }
	mustOwn := func(want int, method, path, body string) string {
		t.Helper()
		code, raw := own(method, path, body)
		if code != want {
			t.Fatalf("owner %s %s: %d %s", method, path, code, raw)
		}
		return raw
	}
	id := func(raw string) string {
		var v struct {
			ID int64 `json:"id"`
		}
		json.Unmarshal([]byte(raw), &v)
		return fmt.Sprint(v.ID)
	}

	// budi listens before the owner writes anything
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL+"/api/events", nil)
	req.Header.Set("Authorization", "Bearer "+budi)
	stream, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	events := make(chan string, 64)
	go func() {
		sc := bufio.NewScanner(stream.Body)
		for sc.Scan() {
			if ev, ok := strings.CutPrefix(sc.Text(), "event: "); ok {
				events <- ev
			}
		}
		close(events)
	}()

	// the owner fills every kind of data
	month := time.Now().In(wib).Format("2006-01")
	cat := id(mustOwn(201, "POST", "/api/categories", `{"name":"Kopi","type":"out"}`))
	tx := id(mustOwn(201, "POST", "/api/transactions", `{"amount":25000,"type":"out","category":"Kopi","merchant":"Kopken","source":"email","source_ref":"m1"}`))
	gone := id(mustOwn(201, "POST", "/api/transactions", `{"amount":5000,"type":"out","category":"Jajan","source":"email","source_ref":"m2"}`))
	mustOwn(204, "DELETE", "/api/transactions/"+gone, "")
	rule := id(mustOwn(201, "POST", "/api/rules", `{"pattern":"kopken","type":"out","category":"Kopi"}`))
	asset := id(mustOwn(201, "POST", "/api/assets", `{"name":"BCA","kind":"bank","balance":1000000}`))
	mustOwn(201, "POST", "/api/orders", `{"asset":"Bibit","kind":"mutual_fund","side":"buy","amount":500000,"order_ref":"o1"}`)
	mustOwn(204, "PUT", "/api/budget", `{"month":"`+month+`","category":"Kopi","amount":200000}`)
	mustOwn(204, "PUT", "/api/budget", `{"month":"`+month+`","category":"","amount":3000000}`)
	rec := id(mustOwn(201, "POST", "/api/recurring", `{"type":"out","category":"Tagihan","merchant":"Netflix","amount":54000,"freq":"monthly","next_date":"2099-01-01"}`))
	mustOwn(200, "GET", "/api/assets", "") // writes today's snapshot

	// budi sees none of it
	for _, path := range []string{"/api/transactions", "/api/transactions?q=Kop", "/api/rules", "/api/recurring", "/api/assets/history"} {
		if code, raw := other("GET", path, ""); code != 200 || strings.TrimSpace(raw) != "[]" {
			t.Errorf("budi GET %s: %d %s", path, code, raw)
		}
	}
	var p portfolio
	if callTok(t, srv, budi, "GET", "/api/assets", "", &p); len(p.Assets) != 0 || len(p.Pending) != 0 || p.TotalIDR != 0 {
		t.Errorf("budi assets: %+v", p)
	}
	var sum struct {
		Out int64 `json:"out"`
	}
	if callTok(t, srv, budi, "GET", "/api/summary", "", &sum); sum.Out != 0 {
		t.Errorf("budi summary out: %d", sum.Out)
	}
	var bud struct {
		Overall    *int64        `json:"overall"`
		Spent      int64         `json:"spent"`
		Categories []BudgetLine  `json:"categories"`
		Upcoming   []interface{} `json:"upcoming"`
	}
	if callTok(t, srv, budi, "GET", "/api/budget?month="+month, "", &bud); bud.Overall != nil || bud.Spent != 0 || len(bud.Categories) != 0 {
		t.Errorf("budi budget: %+v", bud)
	}
	var cats []Category
	callTok(t, srv, budi, "GET", "/api/categories", "", &cats)
	for _, c := range cats {
		if c.Name == "Kopi" || c.Custom {
			t.Errorf("budi sees owner's category: %+v", c)
		}
	}
	if len(cats) < 10 {
		t.Errorf("budi has no built-in categories: %d", len(cats))
	}
	if _, raw := other("POST", "/api/source-refs/check", `{"refs":["m1","m2"]}`); !strings.Contains(raw, `"m1","m2"`) {
		t.Errorf("owner's refs leak into budi's dedup: %s", raw)
	}
	if code, raw := other("GET", "/api/export.xlsx", ""); code != 200 || strings.Contains(raw, "Kopken") {
		t.Errorf("budi export: %d", code)
	}
	if code, raw := other("GET", "/api/assets/"+asset+"/activity", ""); code != 404 {
		t.Errorf("budi asset activity: %d %s", code, raw)
	}

	// and can't change any of it
	for _, c := range []struct{ method, path, body string }{
		{"PATCH", "/api/transactions/" + tx, `{"note":"x"}`},
		{"DELETE", "/api/transactions/" + tx, ""},
		{"POST", "/api/transactions/" + tx + "/split", `{"parts":[{"category":"Makan","amount":25000}]}`},
		{"PATCH", "/api/categories/" + cat, `{"name":"Teh"}`},
		{"DELETE", "/api/categories/" + cat, ""},
		{"PATCH", "/api/rules/" + rule, `{"pattern":"x"}`},
		{"DELETE", "/api/rules/" + rule, ""},
		{"PATCH", "/api/assets/" + asset, `{"balance":1}`},
		{"DELETE", "/api/assets/" + asset, ""},
		{"PATCH", "/api/orders/o1", `{"status":"success"}`},
		{"PATCH", "/api/recurring/" + rec, `{"amount":1}`},
		{"DELETE", "/api/recurring/" + rec, ""},
		{"POST", "/api/recurring/" + rec + "/advance", ""},
	} {
		if code, raw := other(c.method, c.path, c.body); code != 404 {
			t.Errorf("budi %s %s: %d %s", c.method, c.path, code, raw)
		}
	}
	// owner-only names are unknown to budi, so these fail validation instead of touching owner data
	if code, _ := other("POST", "/api/transactions", `{"amount":1,"type":"out","category":"Kopi","source":"manual"}`); code != 400 {
		t.Errorf("budi used owner's category: %d", code)
	}

	// the same source_ref, asset name, order_ref and rule pattern are free for budi
	for _, c := range []struct{ path, body string }{
		{"/api/transactions", `{"amount":7000,"type":"out","category":"Jajan","merchant":"Kopken","source":"email","source_ref":"m1"}`},
		{"/api/assets", `{"name":"BCA","kind":"bank","balance":5}`},
		{"/api/orders", `{"asset":"Bibit","kind":"mutual_fund","side":"buy","amount":1,"order_ref":"o1"}`},
		{"/api/rules", `{"pattern":"kopken","type":"out","category":"Makan"}`},
		{"/api/categories", `{"name":"Kopi","type":"out"}`},
	} {
		if code, raw := other("POST", c.path, c.body); code != 201 {
			t.Errorf("budi POST %s: %d %s", c.path, code, raw)
		}
	}
	// budi's rule doesn't recategorize the owner's mail, and the owner's dedup still holds
	var x Tx
	callTok(t, srv, token, "POST", "/api/transactions", `{"amount":9000,"type":"out","category":"Jajan","merchant":"Kopken 2","source":"email","source_ref":"m3"}`, &x)
	if x.Category != "Kopi" {
		t.Errorf("owner's rule: %+v", x)
	}
	if code, _ := own("POST", "/api/transactions", `{"amount":1,"type":"out","category":"Jajan","source":"email","source_ref":"m1"}`); code != 409 {
		t.Errorf("owner duplicate ref: %d", code)
	}

	// the owner's rows are untouched
	var mine []Tx
	callTok(t, srv, token, "GET", "/api/transactions", "", &mine)
	if len(mine) != 2 || mine[1].Note != "" || mine[1].Category != "Kopi" {
		t.Errorf("owner transactions: %+v", mine)
	}

	// chat is the owner's only (Hermes reads with the owner's token); insight uses the caller's numbers
	a.cfg.hermesURL = "http://127.0.0.1:1"
	if _, raw := other("GET", "/api/chat", ""); !strings.Contains(raw, `"enabled":false`) {
		t.Errorf("budi chat flag: %s", raw)
	}
	if code, _ := other("POST", "/api/chat", `{"text":"hi"}`); code != 403 {
		t.Errorf("budi chat: %d", code)
	}
	if ms, _ := a.insightData(a.ownerID+1, month, 1); ms[0].Spend != 7000 {
		t.Errorf("budi insight: %+v", ms)
	}

	// budi's stream only carried the events of budi's own writes
	cancel()
	var got []string
	for ev := range events {
		got = append(got, ev)
	}
	want := "transaction.created assets.changed assets.changed categories.changed"
	if strings.Join(got, " ") != want {
		t.Errorf("budi events: %v, want %s", got, want)
	}
}
