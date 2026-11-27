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

// TestSharedKantong covers the access rules of a kantong bersama (docs/shared-kantong.md §5):
// one asset row shared by its members, invisible (404) to everyone else.
func TestSharedKantong(t *testing.T) {
	a, srv := newTestApp(t)
	tok := map[string]string{"owner": token}
	uid := map[string]int64{"owner": a.ownerID}
	for _, n := range []string{"budi", "sari", "dedi"} {
		id, err := addUser(a.db, n, "pw")
		if err != nil {
			t.Fatal(err)
		}
		if tok[n], err = newToken(a.db, n); err != nil {
			t.Fatal(err)
		}
		uid[n] = id
	}
	// as calls the API as who and reports a status other than want; must stops the test instead.
	as := func(who string, want int, method, path, body string) string {
		t.Helper()
		code, raw := callTok(t, srv, tok[who], method, path, body, nil)
		if code != want {
			t.Errorf("%s %s %s: %d %s, want %d", who, method, path, code, strings.TrimSpace(raw), want)
		}
		return raw
	}
	must := func(who string, want int, method, path, body string) string {
		t.Helper()
		code, raw := callTok(t, srv, tok[who], method, path, body, nil)
		if code != want {
			t.Fatalf("%s %s %s: %d %s, want %d", who, method, path, code, strings.TrimSpace(raw), want)
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
	type sharedAsset struct {
		ID       int64   `json:"id"`
		Name     string  `json:"name"`
		Balance  float64 `json:"balance"`
		SharedID int64   `json:"shared_id"`
	}
	assets := func(who string) (out struct {
		Assets   []sharedAsset `json:"assets"`
		TotalIDR int64         `json:"total_idr"`
	}) {
		t.Helper()
		callTok(t, srv, tok[who], "GET", "/api/assets", "", &out)
		return out
	}
	balance := func(who, assetID string) (float64, bool) {
		t.Helper()
		for _, x := range assets(who).Assets {
			if fmt.Sprint(x.ID) == assetID {
				return x.Balance, true
			}
		}
		return 0, false
	}
	type kantong struct {
		ID      int64  `json:"id"`
		AssetID int64  `json:"asset_id"`
		Name    string `json:"name"`
		Role    string `json:"role"`
		Members []struct {
			UserID int64  `json:"user_id"`
			Role   string `json:"role"`
		} `json:"members"`
	}
	kantongs := func(who string) (out []kantong) {
		t.Helper()
		callTok(t, srv, tok[who], "GET", "/api/shared", "", &out)
		return out
	}
	createdBy := func(txID string) (owner, by int64) {
		t.Helper()
		if err := a.db.QueryRow(`SELECT user_id, created_by FROM transactions WHERE id = ?`, txID).Scan(&owner, &by); err != nil {
			t.Fatalf("tx %s: %v", txID, err)
		}
		return
	}
	// a minute ahead, so a transaction is after the balance anchor saved in the same second
	soon := time.Now().In(wib).Add(time.Minute).Format(time.RFC3339)
	txBody := func(amount int, account, extra string) string {
		return fmt.Sprintf(`{"amount":%d,"type":"out","category":"Makan","account":%q,"occurred_at":%q,"source":"manual"%s}`, amount, account, soon, extra)
	}

	// owner has a personal bank account; budi a personal "Dapur"
	must("owner", 201, "POST", "/api/assets", `{"name":"BCA","kind":"bank","balance":1000000}`)
	budiDapur := id(must("budi", 201, "POST", "/api/assets", `{"name":"Dapur","kind":"cash","balance":10}`))

	raw := must("owner", 201, "POST", "/api/shared", `{"name":"Dapur","kind":"cash","balance":500000}`)
	var k kantong
	json.Unmarshal([]byte(raw), &k)
	sid, asset := fmt.Sprint(k.ID), fmt.Sprint(k.AssetID)
	if k.Role != "owner" || k.Name != "Dapur" || k.AssetID == 0 {
		t.Fatalf("created kantong: %s", raw)
	}

	t.Run("8 name clash with a member's personal asset", func(t *testing.T) {
		as("owner", 409, "POST", "/api/shared/"+sid+"/members", `{"username":"budi"}`) // budi still has a personal Dapur
		must("budi", 204, "DELETE", "/api/assets/"+budiDapur, "")
		must("owner", 201, "POST", "/api/shared/"+sid+"/members", `{"username":"budi"}`)
		as("owner", 409, "POST", "/api/shared/"+sid+"/members", `{"username":"budi"}`) // already a member
		as("budi", 409, "POST", "/api/assets", `{"name":"dapur","kind":"cash"}`)
		as("owner", 409, "POST", "/api/assets", `{"name":"Dapur","kind":"cash"}`)
		as("owner", 409, "POST", "/api/shared", `{"name":"DAPUR","kind":"cash"}`)
		as("budi", 409, "PATCH", "/api/assets/"+id(must("budi", 201, "POST", "/api/assets", `{"name":"Gopay","kind":"ewallet"}`)), `{"name":"Dapur"}`)
	})

	ownerTx := id(must("owner", 201, "POST", "/api/transactions", txBody(20000, "Dapur", "")))
	budiTx := id(must("budi", 201, "POST", "/api/transactions", txBody(30000, "dapur", "")))
	must("budi", 201, "POST", "/api/transactions", txBody(5000, "Lain", `,"merchant":"Rahasia Budi"`))

	t.Run("1 outsider gets 404 everywhere", func(t *testing.T) {
		if _, ok := balance("sari", asset); ok {
			t.Error("sari sees the kantong in /api/assets")
		}
		if ks := kantongs("sari"); len(ks) != 0 {
			t.Errorf("sari GET /api/shared: %+v", ks)
		}
		if raw := as("sari", 200, "GET", "/api/transactions", ""); strings.TrimSpace(raw) != "[]" {
			t.Errorf("sari sees transactions: %s", raw)
		}
		for _, c := range []struct{ method, path, body string }{
			{"GET", "/api/assets/" + asset + "/activity", ""},
			{"PATCH", "/api/assets/" + asset, `{"balance":1}`},
			{"DELETE", "/api/assets/" + asset, ""},
			{"PATCH", "/api/transactions/" + ownerTx, `{"note":"x"}`},
			{"DELETE", "/api/transactions/" + budiTx, ""},
			{"POST", "/api/transactions/" + ownerTx + "/split", `{"parts":[{"category":"Makan","amount":20000}]}`},
			{"POST", "/api/shared/" + sid + "/members", `{"username":"sari"}`},
			{"DELETE", "/api/shared/" + sid + "/members/" + fmt.Sprint(uid["budi"]), ""},
			{"DELETE", "/api/shared/" + sid, `{"confirm":"Dapur"}`},
			{"POST", "/api/shared/" + sid + "/leave", ""},
		} {
			as("sari", 404, c.method, c.path, c.body)
		}
	})

	t.Run("5 live balance sums both members once", func(t *testing.T) {
		for _, who := range []string{"owner", "budi"} {
			if b, ok := balance(who, asset); !ok || b != 450000 {
				t.Errorf("%s balance: %v %v, want 450000", who, b, ok)
			}
		}
	})

	t.Run("6 net worth and spending count the kantong once", func(t *testing.T) {
		if got := assets("owner").TotalIDR; got != 1450000 {
			t.Errorf("owner net worth: %d, want 1450000", got)
		}
		if got := assets("budi").TotalIDR; got != 450000 {
			t.Errorf("budi net worth: %d, want 450000", got)
		}
		var sum struct {
			Out int64 `json:"out"`
		}
		for who, want := range map[string]int64{"owner": 50000, "budi": 55000} {
			sum.Out = 0
			callTok(t, srv, tok[who], "GET", "/api/summary", "", &sum)
			if sum.Out != want {
				t.Errorf("%s summary out: %d, want %d", who, sum.Out, want)
			}
		}
		if raw := as("owner", 200, "GET", "/api/transactions", ""); strings.Contains(raw, "Rahasia Budi") {
			t.Error("budi's personal transaction leaks to the owner")
		}
	})

	t.Run("2 members read and write transactions and balance", func(t *testing.T) {
		var txs []struct {
			ID        int64  `json:"id"`
			CreatedBy *int64 `json:"created_by"`
		}
		callTok(t, srv, tok["owner"], "GET", "/api/transactions", "", &txs)
		seen := false
		for _, x := range txs {
			if fmt.Sprint(x.ID) == budiTx {
				seen = x.CreatedBy != nil && *x.CreatedBy == uid["budi"]
			}
		}
		if !seen {
			t.Errorf("owner doesn't see budi's transaction with created_by: %+v", txs)
		}
		as("budi", 200, "PATCH", "/api/transactions/"+ownerTx, `{"note":"dari budi"}`)
		as("budi", 400, "PATCH", "/api/transactions/"+ownerTx, `{"account":"BCA"}`) // can't move it into owner's personal account
		as("owner", 200, "PATCH", "/api/transactions/"+budiTx, `{"amount":35000}`)
		as("budi", 200, "GET", "/api/assets/"+asset+"/activity", "")
		as("budi", 200, "PATCH", "/api/assets/"+asset, `{"balance":400000}`)
		if b, _ := balance("owner", asset); b != 400000 {
			t.Errorf("owner balance after budi's update: %v", b)
		}
		as("budi", 204, "DELETE", "/api/transactions/"+ownerTx, "")
		if o, by := createdBy(budiTx); o != uid["budi"] || by != uid["budi"] {
			t.Errorf("budi's tx: user %d created_by %d", o, by)
		}
	})

	t.Run("3 invited member can't invite, remove or delete", func(t *testing.T) {
		as("budi", 403, "POST", "/api/shared/"+sid+"/members", `{"username":"sari"}`)
		as("budi", 403, "DELETE", "/api/shared/"+sid+"/members/"+fmt.Sprint(uid["owner"]), "")
		as("budi", 403, "DELETE", "/api/shared/"+sid, `{"confirm":"Dapur"}`)
		as("budi", 409, "DELETE", "/api/assets/"+asset, "") // the kantong is deleted through /api/shared only
		as("owner", 400, "POST", "/api/shared/"+sid+"/leave", "")
	})

	t.Run("9 email transaction keeps its mailbox as created_by", func(t *testing.T) {
		x := id(must("budi", 201, "POST", "/api/transactions", `{"amount":7000,"type":"out","category":"Makan","account":"Dapur","occurred_at":"`+soon+`","source":"email","source_ref":"<b1@bank>"}`))
		if o, by := createdBy(x); o != uid["budi"] || by != uid["budi"] {
			t.Errorf("email tx: user %d created_by %d", o, by)
		}
		if raw := as("owner", 200, "GET", "/api/transactions", ""); !strings.Contains(raw, `"id":`+x+`,`) {
			t.Error("owner doesn't see budi's email transaction")
		}
	})

	t.Run("leaving keeps the member's transactions in the kantong", func(t *testing.T) {
		must("owner", 201, "POST", "/api/shared/"+sid+"/members", `{"username":"sari"}`)
		x := id(must("sari", 201, "POST", "/api/transactions", txBody(1000, "Dapur", "")))
		before, _ := balance("owner", asset)
		must("sari", 204, "POST", "/api/shared/"+sid+"/leave", "")
		as("sari", 404, "PATCH", "/api/assets/"+asset, `{"note":"x"}`)
		if after, _ := balance("owner", asset); after != before {
			t.Errorf("balance moved when sari left: %v -> %v", before, after)
		}
		if _, by := createdBy(x); by != uid["sari"] {
			t.Errorf("sari's tx created_by: %d", by)
		}
		if raw := as("budi", 200, "GET", "/api/transactions", ""); !strings.Contains(raw, `"id":`+x+`,`) {
			t.Error("budi lost sari's transaction")
		}
	})

	t.Run("4 creator invites, removes and deletes with the right name", func(t *testing.T) {
		must("owner", 204, "DELETE", "/api/shared/"+sid+"/members/"+fmt.Sprint(uid["budi"]), "")
		as("budi", 404, "GET", "/api/assets/"+asset+"/activity", "")
		if b, ok := balance("owner", asset); !ok || b != 393000 {
			t.Errorf("owner balance after removing budi: %v %v, want 393000", b, ok)
		}
		as("owner", 400, "DELETE", "/api/shared/"+sid, `{"confirm":"dapur"}`)
		as("owner", 400, "DELETE", "/api/shared/"+sid, ``)
		must("owner", 204, "DELETE", "/api/shared/"+sid, `{"confirm":"Dapur"}`)
		if _, ok := balance("owner", asset); ok {
			t.Error("kantong still listed after delete")
		}
		var n int
		a.db.QueryRow(`SELECT COUNT(*) FROM transactions WHERE lower(account) = 'dapur'`).Scan(&n)
		if n != 0 {
			t.Errorf("%d kantong transactions left after delete", n)
		}
		if raw := as("budi", 200, "GET", "/api/transactions", ""); !strings.Contains(raw, "Rahasia Budi") {
			t.Error("deleting the kantong took budi's personal transaction")
		}
	})

	t.Run("7 creator's account deleted: oldest member takes over, else the kantong goes", func(t *testing.T) {
		k2 := id(must("owner", 201, "POST", "/api/shared", `{"name":"Liburan","kind":"cash","balance":0}`))
		must("owner", 201, "POST", "/api/shared/"+k2+"/members", `{"username":"budi"}`)
		must("owner", 201, "POST", "/api/shared/"+k2+"/members", `{"username":"dedi"}`)
		x := id(must("owner", 201, "POST", "/api/transactions", txBody(1000, "Liburan", "")))
		solo := id(must("dedi", 201, "POST", "/api/shared", `{"name":"Solo","kind":"cash","balance":0}`))
		if err := a.handOffShared(uid["owner"]); err != nil {
			t.Fatal(err)
		}
		if ks := kantongs("budi"); len(ks) != 1 || ks[0].Role != "owner" || len(ks[0].Members) != 2 {
			t.Errorf("budi after hand-off: %+v", ks)
		}
		if ks := kantongs("owner"); len(ks) != 0 {
			t.Errorf("old creator still a member: %+v", ks)
		}
		if o, by := createdBy(x); o != uid["budi"] || by != uid["owner"] {
			t.Errorf("creator's tx after hand-off: user %d created_by %d", o, by)
		}
		as("budi", 201, "POST", "/api/shared/"+k2+"/members", `{"username":"sari"}`)
		if err := a.handOffShared(uid["dedi"]); err != nil {
			t.Fatal(err)
		}
		var n int
		a.db.QueryRow(`SELECT COUNT(*) FROM shared_assets WHERE id = ?`, solo).Scan(&n)
		if n != 0 || len(kantongs("dedi")) != 0 {
			t.Error("kantong without other members survived its creator")
		}
	})
}
