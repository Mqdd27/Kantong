package main

import (
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

const token = "test-token"

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	_, srv := newTestApp(t)
	return srv
}

func newTestApp(t *testing.T) (*app, *httptest.Server) {
	t.Helper()
	fx := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/price/XAU" {
			w.Write([]byte(`{"price":3110.34768}`)) // 100 USD per gram
			return
		}
		w.Write([]byte(`{"rates":{"USD":0.00005,"SGD":0.0001}}`)) // 1 USD = 20000, 1 SGD = 10000
	}))
	t.Cleanup(fx.Close)
	a, err := newApp(config{
		fxURL:         fx.URL,
		goldURL:       fx.URL,
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
	return a, srv
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
	// a deleted email transaction stays known, so the cron doesn't re-import it
	var list []Tx
	call(t, srv, "GET", "/api/transactions?month=2026-11", "", &list)
	call(t, srv, "DELETE", "/api/transactions/"+strconv.FormatInt(list[0].ID, 10), "", nil)
	call(t, srv, "POST", "/api/source-refs/check", `{"refs":["<m1@bca>"]}`, &res)
	if len(res.Unknown) != 0 {
		t.Fatalf("deleted ref came back: %+v", res.Unknown)
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
	if res, _ := http.Post(srv.URL+"/api/login", "application/json", strings.NewReader(`{"username":"owner","password":"x"}`)); res.StatusCode != 401 {
		t.Fatalf("bad login: %d", res.StatusCode)
	}
	res, _ = http.Post(srv.URL+"/api/login", "application/json", strings.NewReader(`{"username":"owner","password":"pw"}`))
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

func TestCurrencyAndAssets(t *testing.T) {
	srv := newTestServer(t)
	var tx Tx
	if c := call(t, srv, "POST", "/api/transactions", `{"type":"out","category":"Tagihan","merchant":"Claude","source":"manual","currency":"usd","orig_amount":20}`, &tx); c != 201 || tx.Amount != 400000 || tx.Currency != "USD" {
		t.Fatalf("usd tx: %d %+v", c, tx)
	}
	// charged IDR amount given explicitly wins over the computed one
	if c := call(t, srv, "POST", "/api/transactions", `{"amount":330000,"type":"out","category":"Tagihan","source":"manual","currency":"USD","orig_amount":20}`, &tx); c != 201 || tx.Amount != 330000 {
		t.Fatalf("explicit idr: %d %+v", c, tx)
	}
	if c := call(t, srv, "POST", "/api/transactions", `{"type":"out","category":"Tagihan","source":"manual","currency":"USD"}`, nil); c != 400 {
		t.Fatalf("missing orig_amount: %d", c)
	}

	call(t, srv, "POST", "/api/assets", `{"name":"Jago SGD","kind":"forex","currency":"sgd","balance":100}`, nil)
	call(t, srv, "POST", "/api/assets", `{"name":"Saham","kind":"stock","balance":5000000}`, nil)
	// order: pending -> success updates balance; repeated success is idempotent
	if c := call(t, srv, "POST", "/api/orders", `{"asset":"ORI027","kind":"sbn","side":"buy","amount":1000000,"order_ref":"B-1"}`, nil); c != 201 {
		t.Fatalf("order: %d", c)
	}
	var list struct {
		Total   int64   `json:"total_idr"`
		Pending []Order `json:"pending_orders"`
		Assets  []Asset `json:"assets"`
	}
	call(t, srv, "GET", "/api/assets", "", &list)
	if list.Total != 6000000 || len(list.Pending) != 1 {
		t.Fatalf("before settle: %+v", list)
	}
	for range 2 {
		if c := call(t, srv, "PATCH", "/api/orders/B-1", `{"status":"success"}`, nil); c != 200 {
			t.Fatalf("settle: %d", c)
		}
	}
	if c := call(t, srv, "PATCH", "/api/orders/B-1", `{"status":"failed"}`, nil); c != 409 {
		t.Fatalf("flip settled order: %d", c)
	}
	call(t, srv, "POST", "/api/orders", `{"asset":"ORI027","side":"buy","amount":500000,"order_ref":"B-2","status":"failed"}`, nil)
	call(t, srv, "GET", "/api/assets", "", &list)
	if list.Total != 7000000 || len(list.Pending) != 0 {
		t.Fatalf("after settle: %+v", list)
	}
	var hist []struct{ Total, Invested int64 }
	call(t, srv, "GET", "/api/assets/history", "", &hist)
	if len(hist) != 1 || hist[0].Total != 7000000 || hist[0].Invested != 7000000 {
		t.Fatalf("history: %+v", hist)
	}
}

func TestRecurringMask(t *testing.T) {
	for _, c := range []struct {
		x        Recurring
		in, want string
	}{
		{Recurring{Freq: "weekly", Mask: 62}, "2026-11-06", "2026-11-09"},                    // Mon–Fri: Fri -> Mon
		{Recurring{Freq: "weekly", Mask: 62}, "2026-11-09", "2026-11-10"},                    // Mon -> Tue
		{Recurring{Freq: "weekly", Mask: 1<<2 | 1<<4}, "2026-11-10", "2026-11-12"},           // Tue/Thu
		{Recurring{Freq: "monthly", Mask: 1<<0 | 1<<6, day: 31}, "2026-01-31", "2026-07-31"}, // Jan/Jul
		{Recurring{Freq: "monthly", Mask: 1<<0 | 1<<6, day: 31}, "2026-07-31", "2027-01-31"},
	} {
		if got := c.x.next(c.in); got != c.want {
			t.Errorf("%+v next(%s) = %s, want %s", c.x, c.in, got, c.want)
		}
	}
	// a Saturday start for a Mon–Fri item moves to Monday
	srv := newTestServer(t)
	var x Recurring
	if call(t, srv, "POST", "/api/recurring", `{"type":"out","category":"Transport","merchant":"Parkir","amount":5000,"freq":"weekly","mask":62,"next_date":"2026-11-07"}`, &x); x.NextDate != "2026-11-09" {
		t.Fatalf("next_date = %s", x.NextDate)
	}
	if c := call(t, srv, "POST", "/api/recurring", `{"type":"out","category":"Transport","amount":1,"freq":"weekly","mask":128,"next_date":"2026-11-07"}`, nil); c != 400 {
		t.Fatalf("bad mask accepted: %d", c)
	}
}

func TestNextDate(t *testing.T) {
	for _, c := range []struct {
		in, freq string
		day      int
		want     string
	}{
		{"2026-01-31", "monthly", 31, "2026-02-28"},
		{"2026-02-28", "monthly", 31, "2026-03-31"},
		{"2026-11-08", "weekly", 8, "2026-11-15"},
		{"2028-02-29", "yearly", 29, "2029-02-28"},
	} {
		if got := nextDate(c.in, c.freq, c.day); got != c.want {
			t.Errorf("nextDate(%s,%s,%d) = %s, want %s", c.in, c.freq, c.day, got, c.want)
		}
	}
}

func TestBudgetRecurringSearch(t *testing.T) {
	srv := newTestServer(t)
	month := time.Now().In(wib).Format("2006-01")
	prev := time.Now().In(wib).AddDate(0, -1, 0).Format("2006-01")
	// limit set last month carries into this month
	call(t, srv, "PUT", "/api/budget", `{"month":"`+prev+`","category":"","amount":5000000}`, nil)
	call(t, srv, "PUT", "/api/budget", `{"month":"`+prev+`","category":"Makan","amount":1000000}`, nil)
	call(t, srv, "POST", "/api/transactions", `{"amount":200000,"type":"out","category":"Makan","merchant":"Warteg 100%","source":"manual"}`, nil)
	call(t, srv, "POST", "/api/transactions", `{"amount":3000000,"type":"out","category":"Investasi","source":"manual"}`, nil)

	// auto recurring dated today posts immediately; a manual one stays upcoming
	td := time.Now().In(wib).Format(time.DateOnly)
	if c := call(t, srv, "POST", "/api/recurring", `{"type":"out","category":"Tagihan","merchant":"Claude","currency":"USD","orig_amount":20,"freq":"monthly","next_date":"`+td+`","auto":true}`, nil); c != 201 {
		t.Fatalf("recurring: %d", c)
	}
	var recs []Recurring
	call(t, srv, "GET", "/api/recurring", "", &recs)
	if len(recs) != 1 || recs[0].NextDate <= td {
		t.Fatalf("auto not advanced: %+v", recs)
	}
	end := time.Now().In(wib).AddDate(0, 1, -time.Now().In(wib).Day()).Format(time.DateOnly) // last day of month
	call(t, srv, "POST", "/api/recurring", `{"amount":150000,"type":"out","category":"Tagihan","merchant":"Internet","freq":"monthly","next_date":"`+end+`"}`, nil)

	var b struct {
		Overall    *int64       `json:"overall"`
		Spent      int64        `json:"spent"`
		Upcoming   []Upcoming   `json:"upcoming"`
		Categories []BudgetLine `json:"categories"`
	}
	call(t, srv, "GET", "/api/budget?month="+month, "", &b)
	// spent = 200000 makan + 400000 claude (20 USD); Investasi is outside the budget
	if b.Overall == nil || *b.Overall != 5000000 || b.Spent != 600000 || len(b.Upcoming) != 1 || b.Upcoming[0].Amount != 150000 {
		t.Fatalf("budget: %+v", b)
	}
	// record the manual bill: it leaves upcoming and becomes a transaction
	call(t, srv, "POST", "/api/recurring/"+strconv.FormatInt(recs[0].ID+1, 10)+"/advance", "", nil)
	call(t, srv, "GET", "/api/budget?month="+month, "", &b)
	if len(b.Upcoming) != 0 || b.Spent != 750000 {
		t.Fatalf("after advance: %+v", b)
	}

	var found []Tx
	call(t, srv, "GET", "/api/transactions?q=100%25", "", &found)
	if len(found) != 1 || found[0].Merchant != "Warteg 100%" {
		t.Fatalf("search: %+v", found)
	}
}

func TestRulesNotifyBackup(t *testing.T) {
	got := make(chan string, 4)
	tg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		got <- r.URL.Path + "|" + r.Form.Get("chat_id") + "|" + r.Form.Get("text")
	}))
	defer tg.Close()
	dir := t.TempDir()
	a, err := newApp(config{dbPath: filepath.Join(dir, "t.db"), apiToken: token, appPassword: "pw",
		sessionSecret: strings.Repeat("s", 32), tgAPI: tg.URL, tgToken: "TOK", tgChat: "42"})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(a.routes())
	defer func() { srv.Close(); a.db.Close() }()

	// a newer rule for the same pattern replaces the old one
	call(t, srv, "POST", "/api/rules", `{"pattern":"indomaret","type":"out","category":"Makan"}`, nil)
	call(t, srv, "POST", "/api/rules", `{"pattern":"INDOMARET","type":"out","category":"Belanja"}`, nil)
	var rules []Rule
	if call(t, srv, "GET", "/api/rules", "", &rules); len(rules) != 1 {
		t.Fatalf("rules = %+v", rules)
	}
	var tx Tx
	body := strings.Replace(emailTx, `"confidence":0.92`, `"confidence":0.3`, 1)
	if code := call(t, srv, "POST", "/api/transactions", body, &tx); code != 201 || tx.Category != "Belanja" || tx.NeedsReview {
		t.Fatalf("rule not applied: %d %+v", code, tx)
	}
	if m := <-got; m != "/botTOK/sendMessage|42|Keluar Rp25.000 · Belanja · Indomaret" {
		t.Fatalf("telegram = %q", m)
	}
	// manual transactions don't notify and keep their category
	call(t, srv, "POST", "/api/transactions", `{"amount":1000,"type":"out","category":"Jajan","merchant":"Indomaret","source":"manual"}`, &tx)
	if tx.Category != "Jajan" || len(got) != 0 {
		t.Fatalf("manual tx changed/notified: %+v", tx)
	}

	day := time.Date(2026, 1, 1, 3, 0, 0, 0, wib)
	for i := range 16 {
		if err := a.backup(day.AddDate(0, 0, i), 14); err != nil {
			t.Fatal(err)
		}
	}
	files, _ := filepath.Glob(filepath.Join(dir, "backups", "money-*.db"))
	if len(files) != 14 || !strings.HasSuffix(files[0], "money-2026-01-03.db") {
		t.Fatalf("backups = %v", files)
	}
}

func TestAssetReturn(t *testing.T) {
	srv := newTestServer(t)
	var x Asset
	if c := call(t, srv, "POST", "/api/assets", `{"name":"PBS030","kind":"bond","balance":5000000,"invested":5000000,"return_pct":5.9,"maturity":"2028-07-15","payout":6,"face":5000000}`, &x); c != 201 || x.ReturnPct == nil || *x.ReturnPct != 5.9 || *x.Maturity != "2028-07-15" || x.Payout != 6 || *x.Face != 5e6 {
		t.Fatalf("%d %+v", c, x)
	}
	if c := call(t, srv, "PATCH", "/api/assets/"+strconv.FormatInt(x.ID, 10), `{"maturity":"15-07-2028"}`, nil); c != 400 {
		t.Fatalf("bad maturity accepted: %d", c)
	}
	if c := call(t, srv, "PATCH", "/api/assets/"+strconv.FormatInt(x.ID, 10), `{"payout":2}`, nil); c != 400 {
		t.Fatalf("bad payout accepted: %d", c)
	}
}

func TestGoldGrams(t *testing.T) {
	srv := newTestServer(t)
	call(t, srv, "POST", "/api/assets", `{"name":"Emas","kind":"gold","currency":"XAU","balance":1.5,"invested":2500000}`, nil)
	var p struct {
		Assets      []Asset `json:"assets"`
		TotalIDR    int64   `json:"total_idr"`
		InvestedIDR int64   `json:"invested_idr"`
	}
	call(t, srv, "GET", "/api/assets", "", &p)
	if p.Assets[0].Rate != 2e6 || p.TotalIDR != 3e6 || p.InvestedIDR != 2.5e6 { // 1.5 g × Rp2.000.000
		t.Fatalf("%+v", p)
	}
}

func TestExportXLSX(t *testing.T) {
	srv := newTestServer(t)
	call(t, srv, "POST", "/api/transactions", `{"occurred_at":"2026-11-07T12:30:00+07:00","amount":25000,"type":"out","category":"Jajan","merchant":"=Indo<maret>&","source":"manual"}`, nil)
	req, _ := http.NewRequest("GET", srv.URL+"/api/export.xlsx", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	z, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range z.File {
		if f.Name == "xl/worksheets/sheet1.xml" {
			rc, _ := f.Open()
			sheet, _ := io.ReadAll(rc)
			if !bytes.Contains(sheet, []byte("=Indo&lt;maret&gt;&amp;")) || !bytes.Contains(sheet, []byte("<v>25000</v>")) || !bytes.Contains(sheet, []byte("2026-11-07 12:30")) {
				t.Fatalf("sheet: %s", sheet)
			}
			return
		}
	}
	t.Fatal("no sheet1")
}

func TestAssetFollowsTransactions(t *testing.T) {
	srv := newTestServer(t)
	var x Asset
	call(t, srv, "POST", "/api/assets", `{"name":"Jago","kind":"bank","balance":1000000}`, &x)
	time.Sleep(2 * time.Second) // anchor (second precision) < transactions
	at := time.Now().Add(-time.Second).In(wib).Format(time.RFC3339)
	for i, b := range []string{
		`"type":"out","category":"Jajan","account":"jago","amount":50000`,
		`"type":"in","category":"Gaji","account":"Jago","amount":20000`,
		`"type":"transfer","category":"Transfer","account":"Jago","amount":99000`, // to someone else
		`"type":"transfer","category":"Transfer","account":"BCA","to_account":" JAGO ","amount":30000`,
		`"type":"out","category":"Jajan","account":"BCA","amount":7000`,
	} {
		call(t, srv, "POST", "/api/transactions", `{"occurred_at":"`+at+`","source":"manual","source_ref":"r`+strconv.Itoa(i)+`",`+b+`}`, nil)
	}
	var rev Tx
	call(t, srv, "POST", "/api/transactions", `{"occurred_at":"`+at+`","source":"manual","needs_review":true,"type":"out","category":"Jajan","account":"Jago","amount":5000}`, &rev)
	call(t, srv, "POST", "/api/transactions", `{"occurred_at":"2026-01-01T10:00:00+07:00","source":"manual","type":"out","category":"Jajan","account":"Jago","amount":1}`, nil) // before anchor
	bal := func() float64 {
		var p struct{ Assets []Asset }
		call(t, srv, "GET", "/api/assets", "", &p)
		return p.Assets[0].Balance
	}
	if b := bal(); b != 901000 { // 1jt - 50rb + 20rb - 99rb + 30rb, Review item not yet
		t.Fatalf("live balance %v", b)
	}
	call(t, srv, "PATCH", "/api/transactions/"+strconv.FormatInt(rev.ID, 10), `{"needs_review":false}`, nil)
	if b := bal(); b != 896000 {
		t.Fatalf("approved review %v", b)
	}
	time.Sleep(time.Second)
	call(t, srv, "PATCH", "/api/assets/"+strconv.FormatInt(x.ID, 10), `{"note":"x"}`, nil) // moves the anchor, keeps the flow
	if b := bal(); b != 896000 {
		t.Fatalf("after patch %v", b)
	}
	// real balance is 4.200 higher (interest): balance becomes 900.200 and the gap lands in Review
	call(t, srv, "PATCH", "/api/assets/"+strconv.FormatInt(x.ID, 10), `{"balance":900200}`, nil)
	var txs []Tx
	call(t, srv, "GET", "/api/transactions?month="+time.Now().In(wib).Format("2006-01"), "", &txs)
	var adj *Tx
	for i := range txs {
		if txs[i].Category == "Penyesuaian" {
			adj = &txs[i]
		}
	}
	if adj == nil || adj.Type != "in" || adj.Amount != 4200 || !adj.NeedsReview || adj.Account != "Jago" {
		t.Fatalf("adjustment %+v", adj)
	}
	call(t, srv, "PATCH", "/api/transactions/"+strconv.FormatInt(adj.ID, 10), `{"needs_review":false}`, nil)
	if b := bal(); b != 900200 { // approving it must not count it twice
		t.Fatalf("after adjustment %v", b)
	}
	var act struct {
		Transactions []Tx
		Orders       []Order
	}
	call(t, srv, "GET", "/api/assets/"+strconv.FormatInt(x.ID, 10)+"/activity", "", &act)
	// out, in, transfer out, transfer in from BCA, review item, old one, adjustment; BCA's own spending excluded
	if len(act.Transactions) != 7 || act.Transactions[0].Category != "Penyesuaian" || act.Orders == nil {
		t.Fatalf("activity %d %+v", len(act.Transactions), act.Transactions)
	}
}

// A credit card is a negative-balance asset: spending grows the debt, a transfer from the bank pays it down.
func TestCreditCardLiability(t *testing.T) {
	srv := newTestServer(t)
	call(t, srv, "POST", "/api/assets", `{"name":"Jago","kind":"bank","balance":3000000}`, nil)
	var cc Asset
	if c := call(t, srv, "POST", "/api/assets", `{"name":"BCA CC","kind":"credit_card","balance":-1000000}`, &cc); c != 201 {
		t.Fatalf("create card %d", c)
	}
	time.Sleep(2 * time.Second)
	at := time.Now().Add(-time.Second).In(wib).Format(time.RFC3339)
	call(t, srv, "POST", "/api/transactions", `{"occurred_at":"`+at+`","source":"manual","type":"out","category":"Makan","account":"BCA CC","amount":200000}`, nil)
	call(t, srv, "POST", "/api/transactions", `{"occurred_at":"`+at+`","source":"manual","type":"transfer","category":"Transfer","account":"Jago","to_account":"BCA CC","amount":500000}`, nil)
	var p portfolio
	call(t, srv, "GET", "/api/assets", "", &p)
	got := map[string]float64{}
	for _, x := range p.Assets {
		got[x.Name] = x.Balance
	}
	if got["BCA CC"] != -700000 || got["Jago"] != 2500000 || p.TotalIDR != 1800000 { // -1jt -200rb +500rb; 2.5jt - 700rb
		t.Fatalf("balances %v total %d", got, p.TotalIDR)
	}
	// statement shows 750rb owed: the 50rb gap (interest/fee) lands in Review as spending
	call(t, srv, "PATCH", "/api/assets/"+strconv.FormatInt(cc.ID, 10), `{"balance":-750000}`, nil)
	var txs []Tx
	call(t, srv, "GET", "/api/transactions?month="+time.Now().In(wib).Format("2006-01"), "", &txs)
	var adj *Tx
	for i := range txs {
		if txs[i].Category == "Penyesuaian" {
			adj = &txs[i]
		}
	}
	if adj == nil || adj.Type != "out" || adj.Amount != 50000 || adj.Account != "BCA CC" {
		t.Fatalf("adjustment %+v", adj)
	}
}

// Buying a bond: the bank debit lowers Jago right away, the bond grows only when the order succeeds.
func TestBondPurchaseFlow(t *testing.T) {
	srv := newTestServer(t)
	call(t, srv, "POST", "/api/assets", `{"name":"Jago","kind":"bank","balance":10000000}`, nil)
	time.Sleep(2 * time.Second)
	at := time.Now().Add(-time.Second).In(wib).Format(time.RFC3339)
	call(t, srv, "POST", "/api/transactions", `{"occurred_at":"`+at+`","source":"email","source_ref":"<d@jago>","type":"out","category":"Investasi","account":"Jago","amount":5000000}`, nil)
	call(t, srv, "POST", "/api/orders", `{"asset":"FR0100","kind":"bond","side":"buy","amount":5000000,"status":"pending","order_ref":"o1","source_ref":"<o1@bibit>","occurred_at":"`+at+`"}`, nil)
	get := func() map[string]float64 {
		var p struct{ Assets []Asset }
		call(t, srv, "GET", "/api/assets", "", &p)
		m := map[string]float64{}
		for _, x := range p.Assets {
			m[x.Name] = x.Balance
		}
		return m
	}
	if m := get(); m["Jago"] != 5e6 || m["FR0100"] != 0 {
		t.Fatalf("pending %v", m)
	}
	call(t, srv, "PATCH", "/api/orders/o1", `{"status":"success"}`, nil)
	if m := get(); m["Jago"] != 5e6 || m["FR0100"] != 5e6 {
		t.Fatalf("success %v", m)
	}
	var p struct{ Assets []Asset }
	call(t, srv, "GET", "/api/assets", "", &p)
	for _, x := range p.Assets {
		var act struct{ Orders []Order }
		call(t, srv, "GET", "/api/assets/"+strconv.FormatInt(x.ID, 10)+"/activity", "", &act)
		if x.Name == "FR0100" && (len(act.Orders) != 1 || act.Orders[0].Status != "success") {
			t.Fatalf("orders %+v", act.Orders)
		}
	}
}

func TestPatchRule(t *testing.T) {
	srv := newTestServer(t)
	var a, b Rule
	call(t, srv, "POST", "/api/rules", `{"pattern":"MyTelkomsel Apps","type":"out","category":"Lainnya"}`, &a)
	call(t, srv, "POST", "/api/rules", `{"pattern":"Indomaret","type":"out","category":"Belanja"}`, &b)
	id := strconv.FormatInt(a.ID, 10)

	// partial update keeps the other fields and the id
	var got Rule
	if code := call(t, srv, "PATCH", "/api/rules/"+id, `{"category":"Tagihan"}`, &got); code != 200 ||
		got.ID != a.ID || got.Pattern != "MyTelkomsel Apps" || got.Type != "out" || got.Category != "Tagihan" {
		t.Fatalf("patch = %d %+v", code, got)
	}
	var rules []Rule
	call(t, srv, "GET", "/api/rules", "", &rules)
	if len(rules) != 2 || rules[0].Category != "Tagihan" {
		t.Fatalf("rules = %+v", rules)
	}

	// the edited rule is applied to the next email transaction
	var tx Tx
	body := `{"amount":45000,"type":"out","category":"Lainnya","merchant":"MyTelkomsel Apps","account":"Jago","source":"email","source_ref":"<tsel@jago>","confidence":0.3}`
	if code := call(t, srv, "POST", "/api/transactions", body, &tx); code != 201 || tx.Category != "Tagihan" || tx.NeedsReview {
		t.Fatalf("edited rule not applied: %d %+v", code, tx)
	}

	for name, c := range map[string]struct {
		path, body string
		want       int
	}{
		"unknown id":             {"/api/rules/999", `{"category":"Tagihan"}`, 404},
		"bad id":                 {"/api/rules/abc", `{"category":"Tagihan"}`, 400},
		"unknown category":       {"/api/rules/" + id, `{"category":"Nope"}`, 400},
		"category of other type": {"/api/rules/" + id, `{"category":"Gaji"}`, 400},
		"empty pattern":          {"/api/rules/" + id, `{"pattern":"  "}`, 400},
		"pattern conflict":       {"/api/rules/" + id, `{"pattern":"indomaret"}`, 409},
	} {
		if code := call(t, srv, "PATCH", c.path, c.body, nil); code != c.want {
			t.Errorf("%s: status %d, want %d", name, code, c.want)
		}
	}
	// a rejected edit leaves the rule untouched
	call(t, srv, "GET", "/api/rules", "", &rules)
	if rules[0].Pattern != "MyTelkomsel Apps" || rules[0].Category != "Tagihan" {
		t.Fatalf("rule changed by a rejected edit: %+v", rules[0])
	}
}

func TestEmailTransferWithoutDestinationNeedsReview(t *testing.T) {
	srv := newTestServer(t)
	post := func(ref, extra string) Tx {
		var tx Tx
		body := `{"occurred_at":"2026-11-10T14:09:00+07:00","amount":17000,"type":"transfer","category":"Transfer","account":"SeaBank",` +
			extra + `"source":"email","source_ref":"` + ref + `","confidence":1,"needs_review":false}`
		if code := call(t, srv, "POST", "/api/transactions", body, &tx); code != 201 {
			t.Fatalf("create %s: %d", ref, code)
		}
		return tx
	}
	if tx := post("<a@bank>", ""); !tx.NeedsReview {
		t.Errorf("email transfer without to_account must wait in Review: %+v", tx)
	}
	if tx := post("<b@bank>", `"to_account":"  ","merchant":"SeaBank to BCA",`); !tx.NeedsReview {
		t.Errorf("blank to_account must wait in Review: %+v", tx)
	}
	if tx := post("<c@bank>", `"to_account":"BCA",`); tx.NeedsReview {
		t.Errorf("transfer with a destination is not flagged: %+v", tx)
	}

	// manual entries and non-transfer emails keep their behaviour
	var tx Tx
	call(t, srv, "POST", "/api/transactions", `{"amount":5000,"type":"transfer","category":"Transfer","account":"SeaBank","source":"manual"}`, &tx)
	if tx.NeedsReview {
		t.Errorf("manual transfer must not be flagged: %+v", tx)
	}
	call(t, srv, "POST", "/api/transactions", `{"amount":5000,"type":"out","category":"Jajan","source":"email","source_ref":"<d@bank>","confidence":1}`, &tx)
	if tx.NeedsReview {
		t.Errorf("email expense must not be flagged: %+v", tx)
	}

	// approving it in Review clears the flag and it stays cleared
	flagged := post("<e@bank>", "")
	if code := call(t, srv, "PATCH", "/api/transactions/"+strconv.FormatInt(flagged.ID, 10), `{"needs_review":false}`, &tx); code != 200 || tx.NeedsReview {
		t.Fatalf("approve: %d %+v", code, tx)
	}
}

// Due day 5: the next due date rolls over after the 5th; a missed one needs a payment after the 5th of the month before.
func TestDebtDueStatus(t *testing.T) {
	a, err := newApp(config{appPassword: "pw", dbPath: filepath.Join(t.TempDir(), "t.db"), sessionSecret: strings.Repeat("s", 32)})
	if err != nil {
		t.Fatal(err)
	}
	defer a.db.Close()
	day := func(s string) time.Time {
		d, _ := time.ParseInLocation(time.DateOnly, s, wib)
		return d.Add(10 * time.Hour)
	}
	five, last := 5, 31
	check := func(name string, x Asset, now, wantDate string, wantOver bool) {
		t.Helper()
		if err := a.dueStatus(context.Background(), a.ownerID, &x, day(now)); err != nil {
			t.Fatal(err)
		}
		if x.DueDate != wantDate || x.Overdue != wantOver {
			t.Fatalf("%s: got %s overdue=%v, want %s %v", name, x.DueDate, x.Overdue, wantDate, wantOver)
		}
	}
	pay := func(date string) {
		t.Helper()
		if err := a.insertTx(a.ownerID, &Tx{OccurredAt: date + "T09:00:00+07:00", Amount: 100000, Type: "transfer", Category: "Transfer",
			Account: "Jago", ToAccount: "SPayLater", Source: "manual", Currency: "IDR"}); err != nil {
			t.Fatal(err)
		}
	}
	pl := Asset{Name: "SPayLater", Kind: "paylater", Balance: -500000, DueDay: &five}
	check("before due", pl, "2026-11-03", "2026-11-05", false)
	check("due today", pl, "2026-11-05", "2026-11-05", false)
	check("missed", pl, "2026-11-08", "2026-12-05", true)
	pl.Balance = 0
	check("missed but nothing owed", pl, "2026-11-08", "2026-12-05", false)
	pl.Balance = -500000
	pay("2026-10-01") // belongs to the October bill
	check("old payment only", pl, "2026-11-08", "2026-12-05", true)
	pay("2026-10-20")
	check("paid for this cycle", pl, "2026-11-08", "2026-12-05", false)
	check("day 31 in February", Asset{Name: "X", Kind: "loan", Balance: -1, DueDay: &last}, "2026-02-10", "2026-02-28", false)
}

// A recurring installment is a transfer into the paylater account, so posting it pays the debt down.
func TestRecurringTransferPaysDebt(t *testing.T) {
	srv := newTestServer(t)
	call(t, srv, "POST", "/api/assets", `{"name":"Jago","kind":"bank","balance":5000000}`, nil)
	call(t, srv, "POST", "/api/assets", `{"name":"SPayLater","kind":"paylater","balance":-1500000,"due_day":5}`, nil)
	// tomorrow 09:00 is after the balance anchor (now); "Catat" posts it ahead of time
	tomorrow := time.Now().In(wib).AddDate(0, 0, 1).Format(time.DateOnly)
	var rec Recurring
	if c := call(t, srv, "POST", "/api/recurring", `{"type":"transfer","category":"Transfer","merchant":"Cicilan SPayLater","account":"Jago",
		"to_account":"SPayLater","amount":500000,"freq":"monthly","next_date":"`+tomorrow+`"}`, &rec); c != 201 || rec.ToAccount != "SPayLater" {
		t.Fatalf("create %d %+v", c, rec)
	}
	call(t, srv, "POST", "/api/recurring/"+strconv.FormatInt(rec.ID, 10)+"/advance", "", nil)
	var p portfolio
	call(t, srv, "GET", "/api/assets", "", &p)
	for _, x := range p.Assets {
		if (x.Name == "SPayLater" && (x.Balance != -1000000 || x.DueDate == "")) || (x.Name == "Jago" && x.Balance != 4500000) {
			t.Fatalf("%+v", x)
		}
	}
}

// An email transaction split over three categories: total and balance unchanged, the email stays known,
// re-splitting replaces the parts, one part merges back, and deleting the original promotes the next part.
func TestSplitTransaction(t *testing.T) {
	srv := newTestServer(t)
	var tx Tx
	call(t, srv, "POST", "/api/transactions", `{"occurred_at":"2026-11-07T12:30:00+07:00","amount":150000,"type":"out","category":"Jajan",
		"merchant":"Indomaret","account":"BCA","source":"email","source_ref":"<s1@bca>","confidence":0.9}`, &tx)
	split := func(id int64, body string) (int, []Tx) {
		var out []Tx
		return call(t, srv, "POST", "/api/transactions/"+strconv.FormatInt(id, 10)+"/split", body, &out), out
	}
	if c, _ := split(tx.ID, `{"parts":[{"category":"Jajan","amount":35000},{"category":"Belanja","amount":100000}]}`); c != 400 {
		t.Fatalf("wrong sum accepted: %d", c)
	}
	if c, _ := split(tx.ID, `{"parts":[{"category":"Gaji","amount":150000}]}`); c != 400 {
		t.Fatalf("in-category on an out tx accepted: %d", c)
	}
	c, parts := split(tx.ID, `{"parts":[{"category":"Jajan","amount":35000,"note":"snack"},{"category":"Belanja","amount":85000},{"category":"Tagihan","amount":30000,"note":"token"}]}`)
	if c != 200 || len(parts) != 3 || parts[0].ID != tx.ID || parts[2].SplitOf == nil || *parts[2].SplitOf != tx.ID || parts[1].Note != "" {
		t.Fatalf("split %d %+v", c, parts)
	}
	month := func() (n int, sum int64, cats map[string]int64) {
		var txs []Tx
		call(t, srv, "GET", "/api/transactions?month=2026-11", "", &txs)
		cats = map[string]int64{}
		for _, x := range txs {
			n, sum, cats[x.Category] = n+1, sum+x.Amount, cats[x.Category]+x.Amount
		}
		return
	}
	if n, sum, cats := month(); n != 3 || sum != 150000 || cats["Belanja"] != 85000 {
		t.Fatalf("after split %d %d %v", n, sum, cats)
	}
	var refs struct{ Unknown []string }
	call(t, srv, "POST", "/api/source-refs/check", `{"refs":["<s1@bca>"]}`, &refs)
	if len(refs.Unknown) != 0 {
		t.Fatal("email ref lost after split")
	}
	// re-split from any part: replaces the group
	if c, parts = split(parts[2].ID, `{"parts":[{"category":"Jajan","amount":50000},{"category":"Belanja","amount":100000}]}`); c != 200 || len(parts) != 2 {
		t.Fatalf("resplit %d %+v", c, parts)
	}
	if n, sum, _ := month(); n != 2 || sum != 150000 {
		t.Fatalf("after resplit %d %d", n, sum)
	}
	// delete the original: the other part takes over the email ref and is a plain row again
	call(t, srv, "DELETE", "/api/transactions/"+strconv.FormatInt(tx.ID, 10), "", nil)
	var left []Tx
	call(t, srv, "GET", "/api/transactions?month=2026-11", "", &left)
	if len(left) != 1 || left[0].SplitOf != nil || left[0].SourceRef == nil || *left[0].SourceRef != "<s1@bca>" {
		t.Fatalf("after delete %+v", left)
	}
	call(t, srv, "POST", "/api/source-refs/check", `{"refs":["<s1@bca>"]}`, &refs)
	if len(refs.Unknown) != 0 {
		t.Fatal("email ref lost after deleting a part")
	}
	// merge back = split into one part
	c, parts = split(left[0].ID, `{"parts":[{"category":"Belanja","amount":100000},{"category":"Jajan","amount":0}]}`)
	if c != 400 {
		t.Fatalf("zero part accepted %d", c)
	}
	split(left[0].ID, `{"parts":[{"category":"Belanja","amount":60000},{"category":"Jajan","amount":40000}]}`)
	if c, parts = split(left[0].ID, `{"parts":[{"category":"Belanja","amount":100000}]}`); c != 200 || parts[0].SplitOf != nil {
		t.Fatalf("merge %d %+v", c, parts)
	}
	if n, sum, _ := month(); n != 1 || sum != 100000 {
		t.Fatalf("after merge %d %d", n, sum)
	}
}

// A split or delete that fails halfway rolls back: the parts, totals and email ref stay as they were.
func TestSplitRollback(t *testing.T) {
	a, srv := newTestApp(t)
	var tx Tx
	call(t, srv, "POST", "/api/transactions", `{"occurred_at":"2026-11-07T12:30:00+07:00","amount":150000,"type":"out","category":"Jajan",
		"merchant":"Indomaret","account":"BCA","source":"email","source_ref":"<r1@bca>","confidence":0.9}`, &tx)
	path := "/api/transactions/" + strconv.FormatInt(tx.ID, 10)
	if c := call(t, srv, "POST", path+"/split", `{"parts":[{"category":"Jajan","amount":50000},{"category":"Belanja","amount":100000}]}`, nil); c != 200 {
		t.Fatalf("split %d", c)
	}
	snapshot := func() string {
		var txs []Tx
		call(t, srv, "GET", "/api/transactions?month=2026-11", "", &txs)
		b, _ := json.Marshal(txs)
		return string(b)
	}
	before := snapshot()
	exec := func(q string) {
		t.Helper()
		if _, err := a.db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	// the third part's INSERT fails after the old parts were already deleted and the head updated
	exec(`CREATE TRIGGER boom BEFORE INSERT ON transactions WHEN NEW.category = 'Tagihan' BEGIN SELECT RAISE(ABORT, 'boom'); END`)
	if c := call(t, srv, "POST", path+"/split", `{"parts":[{"category":"Jajan","amount":30000},{"category":"Belanja","amount":70000},{"category":"Tagihan","amount":50000}]}`, nil); c != 500 {
		t.Fatalf("failing split: %d", c)
	}
	if after := snapshot(); after != before {
		t.Fatalf("split not rolled back:\n%s\n%s", before, after)
	}
	// deleting the head fails while handing its email ref to the next part
	exec(`CREATE TRIGGER boom2 BEFORE UPDATE OF source_ref ON transactions BEGIN SELECT RAISE(ABORT, 'boom'); END`)
	if c := call(t, srv, "DELETE", path, "", nil); c != 500 {
		t.Fatalf("failing delete: %d", c)
	}
	if after := snapshot(); after != before {
		t.Fatalf("delete not rolled back:\n%s\n%s", before, after)
	}
	// rules that are checked before anything is written
	call(t, srv, "POST", "/api/transactions", `{"occurred_at":"2026-11-08T10:00:00+07:00","source":"manual","type":"transfer","category":"Transfer",
		"account":"BCA","to_account":"Jago","amount":100000}`, &tx)
	if c := call(t, srv, "POST", "/api/transactions/"+strconv.FormatInt(tx.ID, 10)+"/split", `{"parts":[{"category":"Transfer","amount":100000}]}`, nil); c != 400 {
		t.Fatalf("transfer split: %d", c)
	}
	if c := call(t, srv, "POST", path+"/split", `{"parts":[]}`, nil); c != 400 {
		t.Fatalf("empty parts: %d", c)
	}
}

// due_day is 1-31, and an auto recurring transfer into a debt posts once per date, however often the cron runs.
func TestDebtDueDayAndAutoInstallment(t *testing.T) {
	a, srv := newTestApp(t)
	for _, d := range []string{"0", "32"} {
		if c := call(t, srv, "POST", "/api/assets", `{"name":"Kredivo","kind":"paylater","balance":-100000,"due_day":`+d+`}`, nil); c != 400 {
			t.Fatalf("due_day %s: %d", d, c)
		}
	}
	call(t, srv, "POST", "/api/assets", `{"name":"Jago","kind":"bank","balance":5000000}`, nil)
	call(t, srv, "POST", "/api/assets", `{"name":"Kredivo","kind":"paylater","balance":-1500000,"due_day":5}`, nil)
	now := time.Now().In(wib)
	var rec Recurring
	if c := call(t, srv, "POST", "/api/recurring", `{"type":"transfer","category":"Transfer","merchant":"Cicilan Kredivo","account":"Jago",
		"to_account":"Kredivo","amount":500000,"freq":"monthly","auto":true,"next_date":"`+now.Format(time.DateOnly)+`"}`, &rec); c != 201 {
		t.Fatalf("create %d", c)
	}
	a.postDue(context.Background()) // the hourly cron, after create already posted today's
	a.postDue(context.Background())
	var txs []Tx
	call(t, srv, "GET", "/api/transactions?month="+now.Format("2006-01"), "", &txs)
	if len(txs) != 1 || txs[0].Type != "transfer" || txs[0].ToAccount != "Kredivo" || txs[0].Amount != 500000 {
		t.Fatalf("posted %+v", txs)
	}
	var recs []Recurring
	call(t, srv, "GET", "/api/recurring", "", &recs)
	if len(recs) != 1 || recs[0].NextDate <= now.Format(time.DateOnly) {
		t.Fatalf("not advanced %+v", recs)
	}
}

func TestCreateCategory(t *testing.T) {
	srv := newTestServer(t)
	names := func() map[string]bool {
		var cats []Category
		call(t, srv, "GET", "/api/categories", "", &cats)
		m := map[string]bool{}
		for _, c := range cats {
			m[c.Type+":"+c.Name] = true
		}
		return m
	}
	before := len(names())

	var got Category
	if code := call(t, srv, "POST", "/api/categories", `{"name":"  Sedekah  ","type":"out"}`, &got); code != 201 ||
		got.Name != "Sedekah" || got.Type != "out" {
		t.Fatalf("create = %d %+v", code, got)
	}
	if n := names(); !n["out:Sedekah"] || len(n) != before+1 {
		t.Fatalf("categories after create: %v", n)
	}

	// the new category is usable right away by transactions, rules, budgets and splits
	var tx Tx
	if code := call(t, srv, "POST", "/api/transactions", `{"amount":2000,"type":"out","category":"Sedekah","merchant":"Tip driver","source":"manual"}`, &tx); code != 201 || tx.Category != "Sedekah" {
		t.Fatalf("transaction in new category = %d %+v", code, tx)
	}
	if code := call(t, srv, "POST", "/api/rules", `{"pattern":"Tip driver","type":"out","category":"Sedekah"}`, nil); code != 201 {
		t.Fatalf("rule for new category = %d", code)
	}

	// same name on the other type is a different category; a second 'in' category works too
	if code := call(t, srv, "POST", "/api/categories", `{"name":"Sedekah","type":"in"}`, nil); code != 201 {
		t.Fatalf("same name, other type = %d, want 201", code)
	}

	for name, c := range map[string]struct {
		body string
		want int
	}{
		"duplicate":               {`{"name":"Sedekah","type":"out"}`, 409},
		"duplicate, other case":   {`{"name":"sEdEkAh","type":"out"}`, 409},
		"duplicate, extra spaces": {`{"name":" Sedekah ","type":"out"}`, 409},
		"built-in":                {`{"name":"Makan","type":"out"}`, 409},
		"empty":                   {`{"name":"","type":"out"}`, 400},
		"blank":                   {`{"name":"   ","type":"out"}`, 400},
		"too long":                {`{"name":"` + strings.Repeat("a", 31) + `","type":"out"}`, 400},
		"control character":       {`{"name":"a\u0000b","type":"out"}`, 400},
		"newline":                 {`{"name":"a\nb","type":"out"}`, 400},
		"transfer type":           {`{"name":"Lain","type":"transfer"}`, 400},
		"unknown type":            {`{"name":"Lain","type":"x"}`, 400},
		"missing type":            {`{"name":"Lain"}`, 400},
		"unknown field":           {`{"name":"Lain","type":"out","id":1}`, 400},
		"not json":                {`nope`, 400},
	} {
		if code := call(t, srv, "POST", "/api/categories", c.body, nil); code != c.want {
			t.Errorf("%s: status %d, want %d", name, code, c.want)
		}
	}
	// exactly 30 characters (counted as characters, not bytes) is accepted
	if code := call(t, srv, "POST", "/api/categories", `{"name":"`+strings.Repeat("é", 30)+`","type":"out"}`, nil); code != 201 {
		t.Errorf("30 multibyte characters = %d, want 201", code)
	}
	// rejected requests added nothing: before + Sedekah(out) + Sedekah(in) + the 30-char one
	if n := len(names()); n != before+3 {
		t.Errorf("%d categories, want %d", n, before+3)
	}
}

func TestCreateCategoryAuth(t *testing.T) {
	srv := newTestServer(t)
	res, err := http.Post(srv.URL+"/api/categories", "application/json", strings.NewReader(`{"name":"X","type":"out"}`))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 401 {
		t.Fatalf("no token = %d, want 401", res.StatusCode)
	}
}

func TestRenameCategory(t *testing.T) {
	srv := newTestServer(t)
	var cats []Category
	idOf := func(name, typ string) int64 {
		call(t, srv, "GET", "/api/categories", "", &cats)
		for _, c := range cats {
			if c.Name == name && c.Type == typ {
				return c.ID
			}
		}
		return 0
	}
	var made Category
	call(t, srv, "POST", "/api/categories", `{"name":"Sedekah","type":"out"}`, &made)
	if made.ID == 0 || !made.Custom {
		t.Fatalf("created = %+v, want an id and custom=true", made)
	}
	// built-ins are listed as not custom
	call(t, srv, "GET", "/api/categories", "", &cats)
	for _, c := range cats {
		if c.Name == "Makan" && (c.Custom || c.ID == 0) {
			t.Fatalf("built-in listed as %+v", c)
		}
	}

	// data in every place that refers to the category
	var tx Tx
	call(t, srv, "POST", "/api/transactions", `{"amount":2000,"type":"out","category":"Sedekah","merchant":"Tip","source":"manual"}`, &tx)
	call(t, srv, "POST", "/api/rules", `{"pattern":"Tip driver","type":"out","category":"Sedekah"}`, nil)
	month := time.Now().In(wib).Format("2006-01")
	if code := call(t, srv, "PUT", "/api/budget", `{"month":"`+month+`","category":"Sedekah","amount":50000}`, nil); code != 204 {
		t.Fatalf("budget = %d", code)
	}
	var rec Recurring
	if code := call(t, srv, "POST", "/api/recurring", `{"type":"out","category":"Sedekah","merchant":"Masjid","amount":10000,"freq":"monthly","next_date":"2099-01-05"}`, &rec); code != 201 {
		t.Fatalf("recurring = %d", code)
	}

	var got Category
	if code := call(t, srv, "PATCH", "/api/categories/"+strconv.FormatInt(made.ID, 10), `{"name":"  Infaq   Masjid "}`, &got); code != 200 ||
		got.Name != "Infaq Masjid" || got.ID != made.ID || !got.Custom {
		t.Fatalf("rename = %d %+v", code, got)
	}

	// the old name is gone and the new one carries everything over
	if idOf("Sedekah", "out") != 0 || idOf("Infaq Masjid", "out") != made.ID {
		t.Fatalf("categories after rename: %+v", cats)
	}
	var txs []Tx
	call(t, srv, "GET", "/api/transactions?month="+time.Now().In(wib).Format("2006-01"), "", &txs)
	if len(txs) != 1 || txs[0].Category != "Infaq Masjid" {
		t.Errorf("transaction category = %+v", txs)
	}
	var rules []Rule
	call(t, srv, "GET", "/api/rules", "", &rules)
	if len(rules) != 1 || rules[0].Category != "Infaq Masjid" {
		t.Errorf("rule category = %+v", rules)
	}
	var recs []Recurring
	call(t, srv, "GET", "/api/recurring", "", &recs)
	if len(recs) != 1 || recs[0].Category != "Infaq Masjid" {
		t.Errorf("recurring category = %+v", recs)
	}
	var bud struct{ Categories []BudgetLine }
	call(t, srv, "GET", "/api/budget?month="+month, "", &bud)
	found := false
	for _, l := range bud.Categories {
		if l.Category == "Sedekah" {
			t.Errorf("budget still under the old name: %+v", l)
		}
		if l.Category == "Infaq Masjid" && l.Limit != nil && *l.Limit == 50000 && l.Spent == 2000 {
			found = true
		}
	}
	if !found {
		t.Errorf("budget limit/spent did not move to the new name: %+v", bud.Categories)
	}
	// the rule still applies to new email transactions, under the new name
	call(t, srv, "POST", "/api/transactions", `{"amount":3000,"type":"out","category":"Lainnya","merchant":"Tip driver","source":"email","source_ref":"<rn@x>","confidence":0.4}`, &tx)
	if tx.Category != "Infaq Masjid" || tx.NeedsReview {
		t.Errorf("rule after rename: %+v", tx)
	}
	// the old name can no longer be used, the new one can
	if code := call(t, srv, "POST", "/api/transactions", `{"amount":1,"type":"out","category":"Sedekah","source":"manual"}`, nil); code != 400 {
		t.Errorf("old name accepted: %d", code)
	}

	// same name on the other type is unaffected by a rename on 'out'
	var inCat Category
	call(t, srv, "POST", "/api/categories", `{"name":"Hadiah","type":"in"}`, &inCat)
	call(t, srv, "POST", "/api/categories", `{"name":"Hadiah","type":"out"}`, nil)
	if code := call(t, srv, "PATCH", "/api/categories/"+strconv.FormatInt(inCat.ID, 10), `{"name":"Bonus"}`, nil); code != 200 {
		t.Errorf("rename 'in' category = %d", code)
	}
	if idOf("Hadiah", "out") == 0 {
		t.Errorf("renaming the 'in' category touched the 'out' one: %+v", cats)
	}

	id := strconv.FormatInt(made.ID, 10)
	makan := strconv.FormatInt(idOf("Makan", "out"), 10)
	for name, c := range map[string]struct {
		path, body string
		want       int
	}{
		"built-in":           {"/api/categories/" + makan, `{"name":"Makanan"}`, 403},
		"unknown id":         {"/api/categories/9999", `{"name":"X"}`, 404},
		"bad id":             {"/api/categories/abc", `{"name":"X"}`, 400},
		"missing name":       {"/api/categories/" + id, `{}`, 400},
		"empty":              {"/api/categories/" + id, `{"name":""}`, 400},
		"blank":              {"/api/categories/" + id, `{"name":"   "}`, 400},
		"too long":           {"/api/categories/" + id, `{"name":"` + strings.Repeat("a", 31) + `"}`, 400},
		"control character":  {"/api/categories/" + id, `{"name":"a\u0000b"}`, 400},
		"newline":            {"/api/categories/" + id, `{"name":"a\nb"}`, 400},
		"clashes with other": {"/api/categories/" + id, `{"name":"makan"}`, 409},
		"clashes, same type": {"/api/categories/" + id, `{"name":"HADIAH"}`, 409},
		"unknown field":      {"/api/categories/" + id, `{"name":"X","type":"in"}`, 400},
	} {
		if code := call(t, srv, "PATCH", c.path, c.body, nil); code != c.want {
			t.Errorf("%s: status %d, want %d", name, code, c.want)
		}
	}
	// renaming to the same name, or just changing its case, is allowed and a no-op / case fix
	if code := call(t, srv, "PATCH", "/api/categories/"+id, `{"name":"Infaq Masjid"}`, nil); code != 200 {
		t.Errorf("same name = %d, want 200", code)
	}
	if code := call(t, srv, "PATCH", "/api/categories/"+id, `{"name":"infaq masjid"}`, &got); code != 200 || got.Name != "infaq masjid" {
		t.Errorf("case-only change = %d %+v", code, got)
	}
	// rejected edits changed nothing
	if idOf("infaq masjid", "out") != made.ID || idOf("Makan", "out") == 0 {
		t.Errorf("categories after rejected edits: %+v", cats)
	}
}

func TestRenameCategoryAuth(t *testing.T) {
	srv := newTestServer(t)
	req, _ := http.NewRequest("PATCH", srv.URL+"/api/categories/1", strings.NewReader(`{"name":"X"}`))
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 401 {
		t.Fatalf("no token = %d, want 401", res.StatusCode)
	}
}

func TestDeleteCategory(t *testing.T) {
	srv := newTestServer(t)
	var made, twin Category
	call(t, srv, "POST", "/api/categories", `{"name":"Sedekah","type":"out"}`, &made)
	call(t, srv, "POST", "/api/categories", `{"name":"Sedekah","type":"in"}`, &twin)
	month := time.Now().In(wib).Format("2006-01")
	call(t, srv, "POST", "/api/transactions", `{"amount":2000,"type":"out","category":"Sedekah","merchant":"Tip","source":"manual"}`, nil)
	call(t, srv, "POST", "/api/transactions", `{"amount":5000,"type":"in","category":"Sedekah","merchant":"Teman","source":"manual"}`, nil)
	call(t, srv, "POST", "/api/rules", `{"pattern":"Tip driver","type":"out","category":"Sedekah"}`, nil)
	call(t, srv, "PUT", "/api/budget", `{"month":"`+month+`","category":"Sedekah","amount":50000}`, nil)
	call(t, srv, "POST", "/api/recurring", `{"type":"out","category":"Sedekah","merchant":"Masjid","amount":10000,"freq":"monthly","next_date":"2099-01-05"}`, nil)

	// built-in, unknown, bad id
	var cats []Category
	call(t, srv, "GET", "/api/categories", "", &cats)
	for _, c := range cats {
		if c.Name == "Makan" {
			if code := call(t, srv, "DELETE", "/api/categories/"+strconv.FormatInt(c.ID, 10), "", nil); code != 403 {
				t.Errorf("delete built-in = %d, want 403", code)
			}
		}
	}
	if code := call(t, srv, "DELETE", "/api/categories/99999", "", nil); code != 404 {
		t.Errorf("delete unknown = %d, want 404", code)
	}
	if code := call(t, srv, "DELETE", "/api/categories/abc", "", nil); code != 400 {
		t.Errorf("delete bad id = %d, want 400", code)
	}

	var res struct {
		MovedTo      string `json:"moved_to"`
		Transactions int    `json:"transactions"`
		RulesDeleted int    `json:"rules_deleted"`
	}
	if code := call(t, srv, "DELETE", "/api/categories/"+strconv.FormatInt(made.ID, 10), "", &res); code != 200 ||
		res.MovedTo != "Lainnya" || res.Transactions != 1 || res.RulesDeleted != 1 {
		t.Fatalf("delete = %d %+v", code, res)
	}
	if code := call(t, srv, "DELETE", "/api/categories/"+strconv.FormatInt(made.ID, 10), "", nil); code != 404 {
		t.Errorf("second delete = %d, want 404", code)
	}

	call(t, srv, "GET", "/api/categories", "", &cats)
	for _, c := range cats {
		if c.ID == made.ID {
			t.Errorf("category still listed: %+v", c)
		}
	}
	var txs []Tx
	call(t, srv, "GET", "/api/transactions?month="+month, "", &txs)
	for _, x := range txs {
		if x.Type == "out" && x.Category != "Lainnya" || x.Type == "in" && x.Category != "Sedekah" {
			t.Errorf("transaction after delete = %+v (the in twin must stay)", x)
		}
	}
	var rules []Rule
	call(t, srv, "GET", "/api/rules", "", &rules)
	if len(rules) != 0 {
		t.Errorf("rules after delete = %+v", rules)
	}
	var recs []Recurring
	call(t, srv, "GET", "/api/recurring", "", &recs)
	if len(recs) != 1 || recs[0].Category != "Lainnya" {
		t.Errorf("recurring after delete = %+v", recs)
	}
	var bud struct{ Categories []BudgetLine }
	call(t, srv, "GET", "/api/budget?month="+month, "", &bud)
	for _, l := range bud.Categories {
		if l.Category == "Sedekah" {
			t.Errorf("budget left behind: %+v", l)
		}
	}
	// the old name is rejected for new transactions
	if code := call(t, srv, "POST", "/api/transactions", `{"amount":1,"type":"out","category":"Sedekah","merchant":"x","source":"manual"}`, nil); code != 400 {
		t.Errorf("post with deleted category = %d, want 400", code)
	}
}

func TestSPAGzip(t *testing.T) {
	h := spa()
	for _, p := range []string{"/", "/analytics"} {
		r := httptest.NewRequest("GET", p, nil)
		r.Header.Set("Accept-Encoding", "gzip, br")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 200 || w.Header().Get("Content-Encoding") != "gzip" || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/html") {
			t.Fatalf("%s: code %d, headers %v", p, w.Code, w.Header())
		}
		zr, err := gzip.NewReader(w.Body)
		if err != nil {
			t.Fatal(err)
		}
		if b, _ := io.ReadAll(zr); !bytes.Contains(b, []byte("<html")) {
			t.Fatalf("%s: body is not index.html", p)
		}
	}
	w := httptest.NewRecorder() // no Accept-Encoding: plain body
	h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Header().Get("Content-Encoding") != "" || !strings.Contains(w.Body.String(), "<html") {
		t.Fatalf("plain: %v", w.Header())
	}
}

func TestSPAIndexRedirect(t *testing.T) {
	r := httptest.NewRequest("GET", "/index.html", nil)
	r.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()
	spa().ServeHTTP(w, r)
	if w.Code != http.StatusMovedPermanently {
		t.Fatalf("/index.html = %d, want 301", w.Code)
	}
}
