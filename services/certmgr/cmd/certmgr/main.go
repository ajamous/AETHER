// Command certmgr serves the Aether certificate manager.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/ajamous/aether/services/certmgr/internal/server"
	"github.com/ajamous/aether/services/certmgr/internal/store"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "certmgr:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		mode           = flag.String("mode", "lab", "lab | production")
		trustStore     = flag.String("trust-store", "", "PEM file of CI roots")
		intermediates  = flag.String("intermediates", "", "PEM bundle of intermediates (optional, e.g. EUM)")
		dpTLS          = flag.String("dp-tls-cert", "", "DPtls certificate PEM (optional)")
		dpAuth         = flag.String("dp-auth-cert", "", "DPauth certificate PEM (optional)")
		dpPb           = flag.String("dp-pb-cert", "", "DPpb certificate PEM (optional)")
		listen         = flag.String("listen", ":8444", "HTTP listen address")
		generateLab    = flag.String("generate-lab", "", "if set, write a fresh lab chain to this directory and exit (lab mode only)")
	)
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	if *generateLab != "" {
		return generateLabChain(*generateLab)
	}

	if *trustStore == "" {
		return fmt.Errorf("--trust-store is required (use --generate-lab to mint a fresh test chain)")
	}

	identityPaths := map[store.Identity]string{}
	if *dpTLS != "" {
		identityPaths[store.IdentityDPTLS] = *dpTLS
	}
	if *dpAuth != "" {
		identityPaths[store.IdentityDPAuth] = *dpAuth
	}
	if *dpPb != "" {
		identityPaths[store.IdentityDPpb] = *dpPb
	}

	st, err := store.New(store.Config{
		Mode:              store.Mode(strings.ToLower(*mode)),
		TrustStorePath:    *trustStore,
		IntermediatesPath: *intermediates,
		IdentityPaths:     identityPaths,
	})
	if err != nil {
		return err
	}

	bannerForMode(logger, st.Mode())

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	srv := server.New(st)
	logger.Info("certmgr listening",
		slog.String("addr", *listen),
		slog.String("mode", string(st.Mode())),
		slog.Int("identities", len(st.Identities())),
	)
	if err := srv.ListenAndServe(ctx, *listen); err != nil {
		return err
	}
	return nil
}

func bannerForMode(logger *slog.Logger, m store.Mode) {
	switch m {
	case store.ModeLab:
		logger.Warn("CERTMGR MODE: LAB — SGP.26 test certificates only, NOT for production",
			slog.String("mode", "lab"))
	case store.ModeProduction:
		logger.Info("CERTMGR MODE: PRODUCTION — GSMA CI trust set",
			slog.String("mode", "production"))
	}
}

func generateLabChain(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	chain, err := store.GenerateLabChain()
	if err != nil {
		return fmt.Errorf("generate: %w", err)
	}
	if err := chain.WriteFiles(dir, os.WriteFile); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	abs, _ := filepath.Abs(dir)
	fmt.Printf("wrote lab chain to %s:\n", abs)
	for _, name := range []string{"ci-roots.pem", "eum.pem", "DPtls.pem", "DPauth.pem", "DPpb.pem"} {
		fmt.Printf("  %s\n", name)
	}
	return nil
}
