// Command hsm-broker runs the Aether HSM broker.
//
// The broker is the single PKCS#11 façade for all of Aether's services.
// See services/hsm-broker/README.md for the full design.
//
// Backends:
//   - memory: in-memory, for tests, CI, and demos. The default.
//   - pkcs11 (alias: softhsm): generic PKCS#11 module path. SoftHSM
//     v2 is the lab default; AWS CloudHSM, GCP Cloud KMS PKCS#11,
//     Azure Managed HSM (via shim), Thales Luna, and Utimaco
//     SecurityServer all use this same code path with a different
//     `.so` and credential plumbing — see
//     docs/sas-sm/hsm-vendors.md for per-vendor configuration.
//     See ADR 0003 for the architecture rationale.
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
		backendName = flag.String("backend", "memory", "backend: memory | pkcs11 (alias: softhsm)")
		listen      = flag.String("listen", ":8443", "HTTP listen address")
		pkcs11Lib   = flag.String("pkcs11-lib", "", "PKCS#11 .so path (pkcs11 backend only). See docs/sas-sm/hsm-vendors.md for per-vendor paths.")
		slot        = flag.Uint("slot", 0, "PKCS#11 slot id (pkcs11 backend only)")
		pin         = flag.String("pin", "", "PKCS#11 PIN (pkcs11 backend only; prefer env var HSM_PIN)")
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
	case "pkcs11", "softhsm":
		// `softhsm` is the historical name from when SoftHSM v2 was
		// the only PKCS#11 backend exercised in CI. The same code
		// path serves AWS CloudHSM, GCP Cloud KMS PKCS#11, Azure
		// Managed HSM, Thales Luna, and Utimaco SecurityServer.
		// `pkcs11` is the preferred name; `softhsm` is kept as an
		// alias for backward compatibility.
		hsm, err := softhsm.New(softhsm.Config{
			LibraryPath: *pkcs11Lib,
			Slot:        *slot,
			PIN:         *pin,
		})
		if err != nil {
			return fmt.Errorf("pkcs11 backend: %w", err)
		}
		b = hsm
	default:
		return fmt.Errorf("unknown backend %q (use 'memory', 'pkcs11', or 'softhsm')", *backendName)
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
