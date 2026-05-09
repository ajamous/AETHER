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
	listen := flag.String("listen", ":8447", "HTTP listen address")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ledger := chain.NewLedger()
	srv := server.New(ledger)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	logger.Info("audit listening", slog.String("addr", *listen))
	return srv.ListenAndServe(ctx, *listen)
}
