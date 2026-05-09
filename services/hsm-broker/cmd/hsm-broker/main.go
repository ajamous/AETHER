// Command hsm-broker runs the Aether HSM broker.
//
// The broker is the single PKCS#11 façade for all of Aether's services.
// See services/hsm-broker/README.md for the full design.
//
// Backends:
//   - memory: in-memory, for tests, CI, and demos. The default.
//   - softhsm: PKCS#11 module path (typically SoftHSM v2 in lab,
//     a real HSM in production). See ADR 0003.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/ajamous/aether/services/hsm-broker/internal/backend/memory"
	"github.com/ajamous/aether/services/hsm-broker/internal/backend/softhsm"
	"github.com/ajamous/aether/services/hsm-broker/internal/broker"
	"github.com/ajamous/aether/services/hsm-broker/internal/server"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "hsm-broker:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		backendName = flag.String("backend", "memory", "backend: memory | softhsm")
		listen      = flag.String("listen", ":8443", "HTTP listen address")
		pkcs11Lib   = flag.String("pkcs11-lib", "", "PKCS#11 .so path (softhsm backend only)")
		slot        = flag.Uint("slot", 0, "PKCS#11 slot id (softhsm backend only)")
		pin         = flag.String("pin", "", "PKCS#11 PIN (softhsm backend only; prefer env var HSM_PIN)")
	)
	flag.Parse()

	if env := os.Getenv("HSM_PIN"); env != "" && *pin == "" {
		*pin = env
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	var b broker.Broker
	switch *backendName {
	case "memory":
		logger.Warn("hsm-broker started with memory backend; not for production",
			slog.String("backend", "memory"))
		b = memory.New()
	case "softhsm":
		hsm, err := softhsm.New(softhsm.Config{
			LibraryPath: *pkcs11Lib,
			Slot:        *slot,
			PIN:         *pin,
		})
		if err != nil {
			return fmt.Errorf("softhsm backend: %w", err)
		}
		b = hsm
	default:
		return fmt.Errorf("unknown backend %q (use 'memory' or 'softhsm')", *backendName)
	}

	defer func() {
		if err := b.Close(); err != nil {
			logger.Error("backend close failed", slog.Any("err", err))
		}
	}()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	srv := server.New(b)
	logger.Info("hsm-broker listening", slog.String("addr", *listen), slog.String("backend", *backendName))
	if err := srv.ListenAndServe(ctx, *listen); err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	logger.Info("hsm-broker stopped")
	return nil
}
