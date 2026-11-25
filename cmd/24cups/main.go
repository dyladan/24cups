package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	advent "github.com/dyladan/24cups"
	"github.com/dyladan/24cups/internal/app"
	"github.com/dyladan/24cups/internal/telemetry"
)

// version is set at build time (-ldflags "-X main.version=...").
var version = "dev"

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func envInt(k string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(k)); err == nil {
		return v
	}
	return def
}

// healthcheck probes the running server; used by Docker, since the
// distroless image has no shell or curl. Exit 0 = healthy.
func healthcheck() {
	addr := envOr("ADDR", ":8080")
	if strings.HasPrefix(addr, ":") {
		addr = "127.0.0.1" + addr
	}
	c := &http.Client{Timeout: 3 * time.Second}
	res, err := c.Get("http://" + addr + "/api/health")
	if err != nil || res.StatusCode != http.StatusOK {
		os.Exit(1)
	}
	os.Exit(0)
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		healthcheck()
	}
	addr := flag.String("addr", envOr("ADDR", ":8080"), "listen address")
	dbPath := flag.String("db", envOr("DB_PATH", "advent.db"), "SQLite database path")
	coffees := flag.String("coffees", envOr("COFFEES_FILE", ""), "JSON file of calendar coffees (defaults to the embedded list)")
	secure := flag.Bool("secure-cookies", envOr("SECURE_COOKIES", "false") == "true", "mark session cookies Secure (set when served over HTTPS)")
	ipHeader := flag.String("client-ip-header", envOr("CLIENT_IP_HEADER", ""), "trusted header with the real client IP, e.g. CF-Connecting-IP (only if the app is reachable solely through that proxy)")
	calYear := flag.Int("calendar-year", envInt("CALENDAR_YEAR", 2026), "year of the December the calendar covers (unlocks doors for signed-out visitors)")
	tsSiteKey := flag.String("turnstile-site-key", envOr("TURNSTILE_SITE_KEY", ""), "Cloudflare Turnstile site key (public)")
	tsHosts := flag.String("turnstile-hostnames", envOr("TURNSTILE_HOSTNAMES", ""), "comma-separated hostnames Turnstile tokens must come from")
	devDay := flag.Int("dev-day", envInt("DEV_DAY", 0), "development only: pretend it's this day of December (1–31) for which doors are open")
	noLimits := flag.Bool("no-rate-limits", envOr("NO_RATE_LIMITS", "false") == "true", "disable rate limits (benchmarks only)")
	flag.Parse()

	// Telemetry first so the database, startup logs and server are covered.
	otelShutdown, err := telemetry.Setup(context.Background(), version)
	if err != nil {
		slog.Error("telemetry", "err", err)
		os.Exit(1)
	}
	slog.SetDefault(slog.New(telemetry.LogHandler(slog.LevelInfo)))
	if telemetry.Enabled() {
		slog.Info("telemetry enabled", "version", version)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := otelShutdown(ctx); err != nil {
			slog.Warn("telemetry shutdown", "err", err)
		}
	}()

	db, err := app.OpenDB(*dbPath)
	if err != nil {
		slog.Error("open database", "err", err)
		os.Exit(1)
	}
	defer db.Close()
	rdb, err := app.OpenReadDB(*dbPath)
	if err != nil {
		slog.Error("open read pool", "err", err)
		os.Exit(1)
	}
	defer rdb.Close()

	data := advent.CoffeesJSON
	if *coffees != "" {
		if data, err = os.ReadFile(*coffees); err != nil {
			slog.Error("read coffees file", "err", err)
			os.Exit(1)
		}
	}
	if err := app.SeedCoffees(db, data); err != nil {
		slog.Error("seed coffees", "err", err)
		os.Exit(1)
	}

	limits := app.DefaultLimits()
	if *noLimits {
		limits = app.Limits{}
		slog.Warn("rate limits disabled")
	}
	if os.Getenv("TURNSTILE_SECRET_KEY") == "" {
		slog.Warn("TURNSTILE_SECRET_KEY not set; sign-up has no bot check")
	} else {
		for _, h := range app.ParseHostnames(*tsHosts) {
			if h == "localhost" || h == "127.0.0.1" {
				slog.Warn("TURNSTILE_HOSTNAMES includes a local hostname; remove it in production", "hostname", h)
			}
		}
	}
	if *devDay < 0 || *devDay > 31 {
		slog.Error("-dev-day must be between 1 and 31", "dev_day", *devDay)
		os.Exit(2)
	}
	if *devDay > 0 {
		slog.Warn("pretending it's December for development", "day", *devDay, "year", *calYear)
	}
	if *ipHeader == "" {
		slog.Warn("no -client-ip-header set; behind a proxy or tunnel every client shares one rate-limit bucket")
	}
	handler, err := app.NewServer(db, rdb, app.Options{
		Web: advent.WebFS(), SecureCookies: *secure, ClientIPHeader: *ipHeader, Limits: limits, CalendarYear: *calYear, DevDay: *devDay,
		// The secret is read only from the environment so it never appears
		// in process arguments.
		Turnstile: app.TurnstileConfig{SiteKey: *tsSiteKey, Secret: os.Getenv("TURNSTILE_SECRET_KEY"), Hostnames: app.ParseHostnames(*tsHosts)},
	})
	if err != nil {
		slog.Error("start", "err", err)
		os.Exit(1)
	}
	srv := &http.Server{
		Addr:              *addr,
		Handler:           handler.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    32 << 10,
	}
	srv.RegisterOnShutdown(handler.CloseStreams)
	// Fail loudly in the logs (not by refusing to start) if sign-up's bot
	// check can't work: Cloudflare unreachable or the secret rejected.
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := handler.CheckTurnstile(ctx); err != nil {
			slog.Error("turnstile check failed", "err", err)
		} else if os.Getenv("TURNSTILE_SECRET_KEY") != "" {
			slog.Info("turnstile check passed: Siteverify reachable and secret accepted")
		}
	}()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go handler.Run(ctx)
	go func() {
		slog.Info("listening", "addr", *addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("serve", "err", err)
			os.Exit(1)
		}
	}()
	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	srv.Shutdown(shutdownCtx)
}
