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
	listen := flag.String("listen", ":8448", "HTTP listen address")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	store := events.NewMemoryStore()
	srv := server.New(store)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	logger.Info("smds listening", slog.String("addr", *listen))
	return srv.ListenAndServe(ctx, *listen)
}
