// Command profile-builder serves the Aether profile builder.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/ajamous/aether/services/profile-builder/internal/server"
	"github.com/ajamous/aether/services/profile-builder/internal/template"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "profile-builder:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		listen      = flag.String("listen", ":8446", "HTTP listen address")
		templateDir = flag.String("template-dir", "./templates", "directory holding profile YAML templates")
	)
	flag.Parse()

	if _, err := os.Stat(*templateDir); err != nil {
		return fmt.Errorf("template-dir %q: %w", *templateDir, err)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	loader := template.NewLoader(*templateDir)
	srv := server.New(loader)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	logger.Info("profile-builder listening",
		slog.String("addr", *listen),
		slog.String("template_dir", *templateDir),
	)
	return srv.ListenAndServe(ctx, *listen)
}
