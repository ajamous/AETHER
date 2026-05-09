// Command gateway serves the Aether API gateway.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/ajamous/aether/services/gateway/internal/server"
	"github.com/ajamous/aether/services/gateway/internal/tlsconf"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "gateway:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		listen         = flag.String("listen", ":8080", "listen address")
		profileBuilder = flag.String("profile-builder", "http://profile-builder:8446", "profile-builder base URL")
		smdpPlus       = flag.String("smdp-plus", "http://smdp-plus:8443", "smdp-plus base URL")
		certmgr        = flag.String("certmgr", "http://certmgr:8444", "certmgr base URL")
		smds           = flag.String("smds", "http://smds:8448", "smds base URL")
		eim            = flag.String("eim", "http://eim:8449", "eim base URL")

		// TLS / mTLS flags. All optional; lab default is plain HTTP.
		tlsCert    = flag.String("tls-cert", "", "TLS server certificate (PEM); enables HTTPS")
		tlsKey     = flag.String("tls-key", "", "TLS server private key (PEM); required with --tls-cert")
		es2plusCAs = flag.String("es2plus-client-ca", "", "PEM bundle of CAs whose client certs are accepted on /gsma/rsp2/es2plus/* (enables mTLS for ES2+)")

		// Rate limiting on /gsma/rsp2/* paths. Disabled by default;
		// pass both --rate-limit-rps > 0 and --rate-limit-burst >= 1
		// to enable. Keyed by RemoteAddr (the source as seen by the
		// gateway).
		rateLimitRPS   = flag.Float64("rate-limit-rps", 0, "Steady-state requests/sec per source on /gsma/rsp2/*. 0 disables.")
		rateLimitBurst = flag.Int("rate-limit-burst", 0, "Token-bucket burst size per source. 0 disables.")
	)
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	srv, err := server.New(server.Config{
		ProfileBuilder: *profileBuilder,
		SMDPPlus:       *smdpPlus,
		CertMgr:        *certmgr,
		SMDS:           *smds,
		EIM:            *eim,
		TLS: tlsconf.Config{
			CertFile:            *tlsCert,
			KeyFile:             *tlsKey,
			ES2PlusClientCAFile: *es2plusCAs,
		},
		RateLimitRPS:   *rateLimitRPS,
		RateLimitBurst: *rateLimitBurst,
	})
	if err != nil {
		return fmt.Errorf("init: %w", err)
	}

	mode := "HTTP (lab)"
	switch {
	case *tlsCert != "" && *es2plusCAs != "":
		mode = "HTTPS + mTLS for ES2+"
	case *tlsCert != "":
		mode = "HTTPS"
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	rateLimitMode := "disabled"
	if *rateLimitRPS > 0 && *rateLimitBurst >= 1 {
		rateLimitMode = fmt.Sprintf("%.1f rps per source / burst %d", *rateLimitRPS, *rateLimitBurst)
	}

	logger.Info("gateway listening",
		slog.String("addr", *listen),
		slog.String("mode", mode),
		slog.String("rate_limit", rateLimitMode),
		slog.String("profile_builder", *profileBuilder),
		slog.String("smdp_plus", *smdpPlus),
		slog.String("certmgr", *certmgr),
		slog.String("smds", *smds),
		slog.String("eim", *eim),
	)
	if mode == "HTTP (lab)" {
		logger.Warn("ES2+ mTLS DISABLED (no --es2plus-client-ca); BSS clients are not authenticated")
	}
	if rateLimitMode == "disabled" {
		logger.Warn("rate limiting DISABLED (set --rate-limit-rps and --rate-limit-burst); /gsma/rsp2/* has no per-source quota")
	}
	return srv.ListenAndServe(ctx, *listen)
}
