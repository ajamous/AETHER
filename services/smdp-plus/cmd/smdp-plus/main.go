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

	"github.com/ajamous/aether/pkg/certmgrclient"
	"github.com/ajamous/aether/pkg/hsmclient"
	"github.com/ajamous/aether/pkg/pbclient"
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
		certmgrURL    = flag.String("certmgr", "", "certmgr base URL; enables eUICC verification on authenticateClient")
		hsmBrokerURL  = flag.String("hsm-broker", "", "hsm-broker base URL; enables ServerSigned1 signing")
		dpauthLabel   = flag.String("dpauth-label", "DPauth", "HSM key label for the DPauth identity (signs ServerSigned1, §5.7.13)")
		dppbLabel     = flag.String("dppb-label", "", "HSM key label for the DPpb profile-binding identity (signs SmdpSigned2, §5.7.14). Empty disables SmdpSigned2 in authenticateClient responses.")
		serverAddress = flag.String("address", "aether.local", "SM-DP+ public address (goes into ServerSigned1.serverAddress)")
		ttl           = flag.Duration("session-ttl", 10*time.Minute, "session TTL")
		pgURL         = flag.String("pg-url", "", "PostgreSQL URL; if empty, the in-memory session store is used (lab default)")
		pbURL         = flag.String("profile-builder", "", "profile-builder base URL; enables POST /v1/profiles/prepare and credential-carrying BPPs")
		defaultTpl    = flag.String("default-template", "", "profile-builder template name used by /v1/profiles/prepare when a request omits one")
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

		// DPpb is the profile-binding identity used to sign
		// SmdpSigned2 in authenticateClient. Separate ceremony
		// lifecycle from DPauth (rotated on its own cadence per
		// docs/sas-sm/key-ceremony.md). Disabled when empty.
		if *dppbLabel != "" {
			dpCtx, dpCancel := context.WithTimeout(context.Background(), 10*time.Second)
			dppb, err := identity.EnsureLabIdentity(dpCtx, hc, *dppbLabel, *serverAddress)
			dpCancel()
			if err != nil {
				return fmt.Errorf("ensure DPpb identity: %w", err)
			}
			cfg.DPpb = dppb
			logger.Info("smdp-plus DPpb signing enabled (SmdpSigned2 §5.7.14)",
				slog.String("dppb_key_id", dppb.KeyID),
				slog.String("dppb_label", dppb.Label),
			)
		} else {
			logger.Warn("smdp-plus DPpb signing DISABLED (no --dppb-label); authenticateClient returns no SmdpSigned2 — eUICCs will reject this in production")
		}
	} else {
		logger.Warn("smdp-plus signing DISABLED (no --hsm-broker); initiateAuthentication returns unsigned skeleton")
	}

	// Optional eUICC verification on authenticateClient. Enabled when
	// --certmgr is set; otherwise the legacy state-machine-only path
	// is preserved.
	if *certmgrURL != "" {
		cmCtx, cmCancel := context.WithTimeout(context.Background(), 10*time.Second)
		tm, err := identity.FetchTrustMaterial(cmCtx, certmgrclient.New(*certmgrURL))
		cmCancel()
		if err != nil {
			return fmt.Errorf("fetch trust material from certmgr at %s: %w", *certmgrURL, err)
		}
		cfg.Trust = tm
		logger.Info("smdp-plus eUICC verification enabled",
			slog.Int("trust_roots", tm.RootCount()),
			slog.Int("intermediates", tm.IntermediateCount()),
		)
	} else {
		logger.Warn("smdp-plus eUICC verification DISABLED (no --certmgr); authenticateClient skips signature checks")
	}

	// Optional profile-builder integration. Enabled when
	// --profile-builder is set; lets POST /v1/profiles/prepare build a
	// credential-carrying UPP and getBoundProfilePackage seal it. When
	// absent, getBoundProfilePackage falls back to a header-only
	// placeholder UPP.
	if *pbURL != "" {
		pc := pbclient.New(*pbURL)
		probeCtx, probeCancel := context.WithTimeout(context.Background(), 10*time.Second)
		if _, err := pc.Health(probeCtx); err != nil {
			probeCancel()
			return fmt.Errorf("profile-builder health probe at %s: %w", *pbURL, err)
		}
		probeCancel()
		cfg.ProfileBuilder = pc
		cfg.DefaultTemplate = *defaultTpl
		logger.Info("smdp-plus profile-builder integration enabled",
			slog.String("profile_builder", *pbURL),
			slog.String("default_template", *defaultTpl),
		)
	} else {
		logger.Warn("smdp-plus profile-builder integration DISABLED (no --profile-builder); getBoundProfilePackage seals a header-only placeholder UPP")
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
