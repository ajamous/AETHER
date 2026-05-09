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
		tlsCert      = flag.String("tls-cert", "", "TLS server certificate (PEM); enables HTTPS")
		tlsKey       = flag.String("tls-key", "", "TLS server private key (PEM); required with --tls-cert")
		es2plusCAs   = flag.String("es2plus-client-ca", "", "PEM bundle of CAs whose client certs are accepted on /gsma/rsp2/es2plus/* (enables mTLS for ES2+)")
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

	logger.Info("gateway listening",
		slog.String("addr", *listen),
		slog.String("mode", mode),
		slog.String("profile_builder", *profileBuilder),
		slog.String("smdp_plus", *smdpPlus),
		slog.String("certmgr", *certmgr),
		slog.String("smds", *smds),
		slog.String("eim", *eim),
	)
	if mode == "HTTP (lab)" {
		logger.Warn("ES2+ mTLS DISABLED (no --es2plus-client-ca); BSS clients are not authenticated")
	}
	return srv.ListenAndServe(ctx, *listen)
}
