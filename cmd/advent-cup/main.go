package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
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

func main() {
	addr := flag.String("addr", envOr("ADDR", ":8080"), "listen address")
	dbPath := flag.String("db", envOr("DB_PATH", "advent.db"), "SQLite database path")
	coffees := flag.String("coffees", envOr("COFFEES_FILE", ""), "JSON file of calendar coffees (defaults to the embedded list)")
	secure := flag.Bool("secure-cookies", envOr("SECURE_COOKIES", "false") == "true", "mark session cookies Secure (set when served over HTTPS)")
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

	handler, err := app.NewServer(db, rdb, advent.WebFS(), *secure)
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
	}
	srv.RegisterOnShutdown(handler.CloseStreams)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
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
