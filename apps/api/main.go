package main

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path"
	"strings"
	"syscall"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed all:web/dist
var webFS embed.FS

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Jakarta has no DST, so a fixed zone avoids shipping tzdata in a scratch image.
var wib = time.FixedZone("WIB", 7*3600)

type config struct {
	port, dbPath, apiToken, appPassword, sessionSecret, fxURL, goldURL string
	tgAPI, tgToken, tgChat                                             string // Telegram notifications; off when token/chat empty
	insecureCookie                                                     bool   // COOKIE_SECURE=false: allow login over plain HTTP (no PWA install/push then)
	hermesURL, hermesKey, chatModels                                   string // AI chat via Hermes; off when the URL is empty
}

type app struct {
	cfg config
	db  *sql.DB
	hub *hub
}

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, nil)))
	cfg := config{
		port:           env("PORT", "8080"),
		dbPath:         env("DB_PATH", "/data/money.db"),
		apiToken:       os.Getenv("API_TOKEN"),
		appPassword:    os.Getenv("APP_PASSWORD"),
		sessionSecret:  os.Getenv("SESSION_SECRET"),
		insecureCookie: os.Getenv("COOKIE_SECURE") == "false",
		fxURL:          env("FX_URL", "https://api.frankfurter.dev"),
		goldURL:        env("GOLD_URL", "https://api.gold-api.com"),
		tgAPI:          env("TELEGRAM_API", "https://api.telegram.org"),
		tgToken:        os.Getenv("TELEGRAM_BOT_TOKEN"),
		tgChat:         os.Getenv("TELEGRAM_CHAT_ID"),
		hermesURL:      os.Getenv("HERMES_API_URL"),
		hermesKey:      os.Getenv("HERMES_API_KEY"),
		chatModels:     os.Getenv("CHAT_MODELS"),
	}
	if cfg.apiToken == "" || cfg.appPassword == "" || len(cfg.sessionSecret) < 32 {
		slog.Error("API_TOKEN, APP_PASSWORD and SESSION_SECRET (>=32 chars) are required")
		os.Exit(1)
	}
	a, err := newApp(cfg)
	if err != nil {
		slog.Error("startup", "err", err)
		os.Exit(1)
	}
	defer a.db.Close()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	srv := &http.Server{
		Addr:              ":" + cfg.port,
		Handler:           a.routes(),
		ReadHeaderTimeout: 10 * time.Second,
		BaseContext:       func(_ net.Listener) context.Context { return ctx }, // cancels SSE streams on shutdown
	}
	go a.snapshotLoop(ctx)
	go a.backupLoop(ctx)
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(sctx)
	}()
	slog.Info("listening", "addr", srv.Addr, "db", cfg.dbPath)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		slog.Error("server", "err", err)
		os.Exit(1)
	}
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func newApp(cfg config) (*app, error) {
	dsn := "file:" + cfg.dbPath + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)&_pragma=synchronous(NORMAL)&_txlock=immediate"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &app{cfg: cfg, db: db, hub: newHub()}, nil
}

func migrate(db *sql.DB) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (version TEXT PRIMARY KEY)`); err != nil {
		return err
	}
	entries, err := fs.ReadDir(migrationsFS, "migrations") // sorted by name
	if err != nil {
		return err
	}
	for _, e := range entries {
		var n int
		db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, e.Name()).Scan(&n)
		if n > 0 {
			continue
		}
		body, err := migrationsFS.ReadFile("migrations/" + e.Name())
		if err != nil {
			return err
		}
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(string(body)); err != nil {
			tx.Rollback()
			return fmt.Errorf("%s: %w", e.Name(), err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations (version) VALUES (?)`, e.Name()); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		slog.Info("migrated", "file", e.Name())
	}
	return nil
}

func (a *app) routes() http.Handler {
	api := http.NewServeMux()
	api.HandleFunc("POST /api/transactions", a.createTx)
	api.HandleFunc("GET /api/transactions", a.listTx)
	api.HandleFunc("PATCH /api/transactions/{id}", a.patchTx)
	api.HandleFunc("DELETE /api/transactions/{id}", a.deleteTx)
	api.HandleFunc("POST /api/source-refs/check", a.checkRefs)
	api.HandleFunc("GET /api/summary", a.summary)
	api.HandleFunc("GET /api/categories", a.listCategories)
	api.HandleFunc("GET /api/rules", a.listRules)
	api.HandleFunc("POST /api/rules", a.createRule)
	api.HandleFunc("PATCH /api/rules/{id}", a.patchRule)
	api.HandleFunc("DELETE /api/rules/{id}", a.deleteRule)
	api.HandleFunc("GET /api/events", a.events)
	api.HandleFunc("GET /api/budget", a.budget)
	api.HandleFunc("PUT /api/budget", a.setBudget)
	api.HandleFunc("GET /api/recurring", a.listRecurring)
	api.HandleFunc("POST /api/recurring", a.createRecurring)
	api.HandleFunc("PATCH /api/recurring/{id}", a.patchRecurring)
	api.HandleFunc("DELETE /api/recurring/{id}", a.deleteRecurring)
	api.HandleFunc("POST /api/recurring/{id}/advance", a.advanceRecurring)
	api.HandleFunc("GET /api/export.xlsx", a.exportXLSX)
	api.HandleFunc("GET /api/assets", a.listAssets)
	api.HandleFunc("GET /api/assets/history", a.assetHistory)
	api.HandleFunc("GET /api/assets/{id}/activity", a.assetActivity)
	api.HandleFunc("POST /api/assets", a.createAsset)
	api.HandleFunc("PATCH /api/assets/{id}", a.patchAsset)
	api.HandleFunc("DELETE /api/assets/{id}", a.deleteAsset)
	api.HandleFunc("POST /api/orders", a.createOrder)
	api.HandleFunc("PATCH /api/orders/{ref}", a.patchOrder)
	api.HandleFunc("GET /api/chat", a.getChat)
	api.HandleFunc("POST /api/chat", a.postChat)
	api.HandleFunc("DELETE /api/chat", a.clearChat)
	api.HandleFunc("GET /api/fx", func(w http.ResponseWriter, r *http.Request) {
		c, ok := normCurrency(r.URL.Query().Get("currency"))
		if !ok {
			writeErr(w, http.StatusBadRequest, "currency must be a 3-letter ISO code")
			return
		}
		rate, err := a.idrRate(r.Context(), c, r.URL.Query().Get("date"))
		if err != nil {
			writeErr(w, http.StatusBadGateway, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"currency": c, "idr": rate})
	})

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })
	mux.HandleFunc("POST /api/login", a.login)
	mux.Handle("/api/", a.auth(api))
	mux.Handle("/", spa())
	return mux
}

// spa serves the embedded PWA, falling back to index.html for client-side routes.
func spa() http.Handler {
	mime.AddExtensionType(".webmanifest", "application/manifest+json")
	dist, _ := fs.Sub(webFS, "web/dist")
	files := http.FileServerFS(dist)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if strings.HasPrefix(p, "assets/") { // content-hashed by Vite
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache") // index.html, sw.js, manifest must revalidate
		}
		if p != "" {
			if _, err := fs.Stat(dist, p); err != nil {
				http.ServeFileFS(w, r, dist, "index.html")
				return
			}
		}
		files.ServeHTTP(w, r)
	})
}
