// Command smds serves the Aether SM-DS.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/ajamous/aether/services/smds/internal/events"
	"github.com/ajamous/aether/services/smds/internal/server"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "smds:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		listen = flag.String("listen", ":8448", "HTTP listen address")
		pgURL  = flag.String("pg-url", "", "PostgreSQL URL; if empty, the in-memory store is used (lab default)")
	)
	flag.Parse()

	if env := os.Getenv("AETHER_PG_URL"); env != "" && *pgURL == "" {
		*pgURL = env
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	var store events.Store
	if *pgURL == "" {
		logger.Warn("smds using in-memory store; events lost on restart")
		store = events.NewMemoryStore()
	} else {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		pg, err := events.NewPGStore(ctx, *pgURL)
		if err != nil {
			return fmt.Errorf("postgres store: %w", err)
		}
		defer pg.Close()
		store = pg
		logger.Info("smds using postgres store")
	}

	srv := server.New(store)
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	logger.Info("smds listening", slog.String("addr", *listen))
	return srv.ListenAndServe(ctx, *listen)
}
