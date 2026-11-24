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

	advent "github.com/dyladan/advent-cup"
	"github.com/dyladan/advent-cup/internal/app"
)

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
	noLimits := flag.Bool("no-rate-limits", envOr("NO_RATE_LIMITS", "false") == "true", "disable rate limits (benchmarks only)")
	flag.Parse()

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
	if *ipHeader == "" {
		slog.Warn("no -client-ip-header set; behind a proxy or tunnel every client shares one rate-limit bucket")
	}
	handler, err := app.NewServer(db, rdb, app.Options{
		Web: advent.WebFS(), SecureCookies: *secure, ClientIPHeader: *ipHeader, Limits: limits, CalendarYear: *calYear,
	})
	if err != nil {
		slog.Error("start", "err", err)
		os.Exit(1)
	}
	srv := &http.Server{
		Addr:              *addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    32 << 10,
	}
	srv.RegisterOnShutdown(handler.CloseStreams)
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
