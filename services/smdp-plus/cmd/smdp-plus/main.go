// Command smdp-plus serves the Aether SM-DP+.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ajamous/aether/pkg/hsmclient"
	"github.com/ajamous/aether/services/smdp-plus/internal/identity"
	"github.com/ajamous/aether/services/smdp-plus/internal/server"
	"github.com/ajamous/aether/services/smdp-plus/internal/session"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "smdp-plus:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		listen        = flag.String("listen", ":8443", "HTTP(S) listen address")
		tlsCert       = flag.String("tls-cert", "", "TLS server cert (DPtls)")
		tlsKey        = flag.String("tls-key", "", "TLS server key")
		_             = flag.String("certmgr", "", "certmgr base URL (reserved)")
		hsmBrokerURL  = flag.String("hsm-broker", "", "hsm-broker base URL; enables ServerSigned1 signing")
		dpauthLabel   = flag.String("dpauth-label", "DPauth", "HSM key label for the DPauth identity")
		serverAddress = flag.String("address", "aether.local", "SM-DP+ public address (goes into ServerSigned1.serverAddress)")
		ttl           = flag.Duration("session-ttl", 10*time.Minute, "session TTL")
		pgURL         = flag.String("pg-url", "", "PostgreSQL URL; if empty, the in-memory session store is used (lab default)")
	)
	flag.Parse()

	if env := os.Getenv("AETHER_PG_URL"); env != "" && *pgURL == "" {
		*pgURL = env
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	// Session store
	var store session.Store
	if *pgURL == "" {
		logger.Warn("smdp-plus using in-memory session store; sessions lost on restart")
		store = session.NewMemoryStore(*ttl)
	} else {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		pg, err := session.NewPGStore(ctx, *pgURL, *ttl)
		if err != nil {
			return fmt.Errorf("postgres session store: %w", err)
		}
		defer pg.Close()
		store = pg
		logger.Info("smdp-plus using postgres session store")
	}

	// Optional signing pipeline. Enabled only when --hsm-broker is set;
	// otherwise initiateAuthentication returns the skeleton response.
	cfg := server.Config{Address: *serverAddress}
	if *hsmBrokerURL != "" {
		hc := hsmclient.New(*hsmBrokerURL)
		probeCtx, probeCancel := context.WithTimeout(context.Background(), 10*time.Second)
		if _, err := hc.Health(probeCtx); err != nil {
			probeCancel()
			return fmt.Errorf("hsm-broker health probe at %s: %w", *hsmBrokerURL, err)
		}
		probeCancel()

		idCtx, idCancel := context.WithTimeout(context.Background(), 10*time.Second)
		id, err := identity.EnsureLabIdentity(idCtx, hc, *dpauthLabel, *serverAddress)
		idCancel()
		if err != nil {
			return fmt.Errorf("ensure DPauth identity: %w", err)
		}
		cfg.HSM = hc
		cfg.Identity = id
		logger.Info("smdp-plus signing enabled",
			slog.String("dpauth_key_id", id.KeyID),
			slog.String("dpauth_label", id.Label),
			slog.String("address", *serverAddress),
		)
	} else {
		logger.Warn("smdp-plus signing DISABLED (no --hsm-broker); initiateAuthentication returns unsigned skeleton")
	}

	srv := server.New(store, cfg)
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	mode := "HTTP (lab)"
	if *tlsCert != "" && *tlsKey != "" {
		mode = "HTTPS"
	}
	logger.Info("smdp-plus listening",
		slog.String("addr", *listen),
		slog.String("mode", mode),
		slog.Duration("session_ttl", *ttl),
	)
	return srv.ListenAndServeTLS(ctx, *listen, *tlsCert, *tlsKey)
}
