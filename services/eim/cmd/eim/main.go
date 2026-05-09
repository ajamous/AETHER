// Command eim serves the Aether eSIM IoT Manager (SGP.32).
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/ajamous/aether/services/eim/internal/devices"
	"github.com/ajamous/aether/services/eim/internal/server"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "eim:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		listen = flag.String("listen", ":8449", "HTTP listen address")
		pgURL  = flag.String("pg-url", "", "PostgreSQL URL; if empty, the in-memory store is used (lab default)")
	)
	flag.Parse()
	if env := os.Getenv("AETHER_PG_URL"); env != "" && *pgURL == "" {
		*pgURL = env
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	var store devices.Store
	if *pgURL == "" {
		logger.Warn("eim using in-memory store; devices and commands lost on restart")
		store = devices.NewMemoryStore()
	} else {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		pg, err := devices.NewPGStore(ctx, *pgURL)
		if err != nil {
			return fmt.Errorf("postgres store: %w", err)
		}
		defer pg.Close()
		store = pg
		logger.Info("eim using postgres store")
	}

	srv := server.New(store)
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	logger.Info("eim listening", slog.String("addr", *listen))
	return srv.ListenAndServe(ctx, *listen)
}
