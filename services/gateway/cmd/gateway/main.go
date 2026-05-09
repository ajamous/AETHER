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
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "gateway:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		listen         = flag.String("listen", ":8080", "HTTP listen address")
		profileBuilder = flag.String("profile-builder", "http://profile-builder:8446", "profile-builder base URL")
		smdpPlus       = flag.String("smdp-plus", "http://smdp-plus:8443", "smdp-plus base URL")
		certmgr        = flag.String("certmgr", "http://certmgr:8444", "certmgr base URL")
		smds           = flag.String("smds", "http://smds:8448", "smds base URL")
		eim            = flag.String("eim", "http://eim:8449", "eim base URL")
	)
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	srv := server.New(server.Config{
		ProfileBuilder: *profileBuilder,
		SMDPPlus:       *smdpPlus,
		CertMgr:        *certmgr,
		SMDS:           *smds,
		EIM:            *eim,
	})

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	logger.Info("gateway listening",
		slog.String("addr", *listen),
		slog.String("profile_builder", *profileBuilder),
		slog.String("smdp_plus", *smdpPlus),
		slog.String("certmgr", *certmgr),
		slog.String("smds", *smds),
		slog.String("eim", *eim),
	)
	return srv.ListenAndServe(ctx, *listen)
}
