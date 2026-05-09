// Command audit serves the Aether audit log.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/ajamous/aether/services/audit/internal/chain"
	"github.com/ajamous/aether/services/audit/internal/server"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "audit:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		listen = flag.String("listen", ":8447", "HTTP listen address")
		pgURL  = flag.String("pg-url", "", "PostgreSQL URL; if empty, the in-memory ledger is used (lab default)")
	)
	flag.Parse()

	if env := os.Getenv("AETHER_PG_URL"); env != "" && *pgURL == "" {
		*pgURL = env
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	var backend chain.Backend
	if *pgURL == "" {
		logger.Warn("audit using in-memory ledger; data lost on restart")
		backend = chain.NewLedger()
	} else {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		pg, err := chain.NewPGLedger(ctx, *pgURL)
		if err != nil {
			return fmt.Errorf("postgres ledger: %w", err)
		}
		defer pg.Close()
		backend = pg
		logger.Info("audit using postgres ledger")
	}

	srv := server.New(backend)
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	logger.Info("audit listening", slog.String("addr", *listen))
	return srv.ListenAndServe(ctx, *listen)
}
