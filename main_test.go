package main

import (
	"archive/zip"
	"bufio"
	"bytes"
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
		`"type":"transfer","category":"Transfer","account":"Jago","amount":99000`, // no destination: ignored
		`"type":"out","category":"Jajan","account":"BCA","amount":7000`,
	} {
		call(t, srv, "POST", "/api/transactions", `{"occurred_at":"`+at+`","source":"manual","source_ref":"r`+strconv.Itoa(i)+`",`+b+`}`, nil)
	}
	call(t, srv, "POST", "/api/transactions", `{"occurred_at":"2026-01-01T10:00:00+07:00","source":"manual","type":"out","category":"Jajan","account":"Jago","amount":1}`, nil) // before anchor
	bal := func() float64 {
		var p struct{ Assets []Asset }
		call(t, srv, "GET", "/api/assets", "", &p)
		return p.Assets[0].Balance
	}
	if b := bal(); b != 970000 {
		t.Fatalf("live balance %v", b)
	}
	time.Sleep(time.Second)
	call(t, srv, "PATCH", "/api/assets/"+strconv.FormatInt(x.ID, 10), `{"note":"x"}`, nil) // moves the anchor, keeps the flow
	if b := bal(); b != 970000 {
		t.Fatalf("after patch %v", b)
	}
}
