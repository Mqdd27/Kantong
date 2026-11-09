package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
)

var currencyRe = regexp.MustCompile(`^[A-Z]{3}$`)

func normCurrency(c string) (string, bool) {
	c = strings.ToUpper(strings.TrimSpace(c))
	if c == "" {
		c = "IDR"
	}
	return c, currencyRe.MatchString(c)
}

// idrRate returns how many IDR one unit of cur was worth on date (YYYY-MM-DD).
// Rates come from frankfurter.dev (ECB data) and are cached per requested date.
// ponytail: ECB has no weekend rates; frankfurter returns the previous business day, which is fine here.
func (a *app) idrRate(ctx context.Context, cur, date string) (float64, error) {
	if cur == "IDR" {
		return 1, nil
	}
	if cur == "XAU" {
		return a.goldRate(ctx)
	}
	today := time.Now().In(wib).Format(time.DateOnly)
	if date == "" || date > today {
		date = today
	}
	var r float64
	if err := a.db.QueryRowContext(ctx, `SELECT idr FROM fx_rates WHERE date = ? AND currency = ?`, date, cur).Scan(&r); err == nil {
		return r, nil
	}

	path := date
	if date == today {
		path = "latest"
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", a.cfg.fxURL+"/v1/"+path+"?base=IDR", nil)
	req.Header.Set("User-Agent", "kantong")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("fx fetch: %w", err)
	}
	defer res.Body.Close()
	var body struct {
		Rates map[string]float64 `json:"rates"`
	}
	if res.StatusCode != 200 || json.NewDecoder(res.Body).Decode(&body) != nil {
		return 0, fmt.Errorf("fx fetch: HTTP %d", res.StatusCode)
	}
	for c, perIDR := range body.Rates {
		if perIDR > 0 {
			a.db.ExecContext(ctx, `INSERT OR REPLACE INTO fx_rates (date, currency, idr) VALUES (?, ?, ?)`, date, c, 1/perIDR)
		}
	}
	if per := body.Rates[cur]; per > 0 {
		return 1 / per, nil
	}
	return 0, fmt.Errorf("no exchange rate for %s", cur)
}

// goldRate returns IDR per gram of gold (Kantong's "XAU" balance is grams), from the
// gold-api.com spot price in USD per troy ounce, cached per day.
// ponytail: spot price, so it sits a few % above dealer buyback (Pegadaian/Antam); no history, past dates use today.
func (a *app) goldRate(ctx context.Context) (float64, error) {
	today := time.Now().In(wib).Format(time.DateOnly)
	var r float64
	if err := a.db.QueryRowContext(ctx, `SELECT idr FROM fx_rates WHERE date = ? AND currency = 'XAU'`, today).Scan(&r); err == nil {
		return r, nil
	}
	usd, err := a.idrRate(ctx, "USD", today)
	if err != nil {
		return 0, err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", a.cfg.goldURL+"/price/XAU", nil)
	req.Header.Set("User-Agent", "kantong")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("gold fetch: %w", err)
	}
	defer res.Body.Close()
	var body struct {
		Price float64 `json:"price"` // USD per troy ounce
	}
	if res.StatusCode != 200 || json.NewDecoder(res.Body).Decode(&body) != nil || body.Price <= 0 {
		return 0, fmt.Errorf("gold fetch: HTTP %d", res.StatusCode)
	}
	r = body.Price / 31.1034768 * usd
	a.db.ExecContext(ctx, `INSERT OR REPLACE INTO fx_rates (date, currency, idr) VALUES (?, 'XAU', ?)`, today, r)
	return r, nil
}
