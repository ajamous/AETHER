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

	"github.com/ajamous/aether/pkg/hsmclient"
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

		// HSM signing for AuthenticateClient (SGP.22 §5.5.4). All
		// three flags must be set to enable; lab default is unsigned.
		hsmBroker     = flag.String("hsm-broker", "", "HSM broker base URL (e.g. http://hsm-broker:8443); enables SGP.22 §5.5.4 ServerSigned1 signing on AuthenticateClient")
		signingKeyID  = flag.String("signing-key", "smds-auth-key", "HSM key ID for the SM-DS auth key used to sign ServerSigned1")
		serverAddress = flag.String("server-address", "", "SM-DS server address as it appears in the signed payload; must match the SM-DS certificate's SAN. Required when --hsm-broker is set.")
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

	cfg := server.Config{Logger: logger}
	signingMode := "disabled"
	if *hsmBroker != "" {
		if *serverAddress == "" {
			return fmt.Errorf("--server-address is required when --hsm-broker is set")
		}
		cfg.Signer = &server.Signer{
			Broker:        hsmclient.New(*hsmBroker),
			KeyID:         *signingKeyID,
			ServerAddress: *serverAddress,
		}
		signingMode = fmt.Sprintf("hsm broker=%s key=%s server_address=%s", *hsmBroker, *signingKeyID, *serverAddress)
	}

	srv := server.New(store, cfg)
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	logger.Info("smds listening",
		slog.String("addr", *listen),
		slog.String("signing", signingMode),
	)
	if signingMode == "disabled" {
		logger.Warn("SGP.22 §5.5.4 ServerSigned1 signing DISABLED (no --hsm-broker); LPAs cannot verify AuthenticateClient responses")
	}
	return srv.ListenAndServe(ctx, *listen)
}
