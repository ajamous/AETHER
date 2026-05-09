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
		listen   = flag.String("listen", ":8443", "HTTP(S) listen address")
		tlsCert  = flag.String("tls-cert", "", "TLS server cert (DPtls)")
		tlsKey   = flag.String("tls-key", "", "TLS server key")
		_        = flag.String("certmgr", "", "certmgr base URL (reserved)")
		_        = flag.String("hsm-broker", "", "hsm-broker base URL (reserved)")
		ttl      = flag.Duration("session-ttl", 10*time.Minute, "in-memory session TTL")
	)
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	store := session.NewMemoryStore(*ttl)
	srv := server.New(store)

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
