// Package tlsconf carries the gateway's listener configuration —
// plain HTTP, HTTPS, or HTTPS with mTLS — and the middleware that
// enforces "verified client cert required on ES2+ paths" when mTLS
// is active.
//
// SGP.22 §5.4 mandates mTLS on ES2+ between BSS and SM-DP+. The
// gateway is the public-facing surface where that boundary lives.
// Internal /v1/* routes (admin UI, REST surface) do not require a
// client cert; the BSS-facing paths do. Both surfaces share a
// single TLS listener and certificate.
package tlsconf

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
)

// ListenerMode describes how the gateway exposes itself.
type ListenerMode int

const (
	// ModePlainHTTP is the lab default — no TLS.
	ModePlainHTTP ListenerMode = iota
	// ModeTLS enables TLS but no client-cert verification.
	ModeTLS
	// ModeTLSWithMTLS enables TLS and requires a verified client
	// cert on the ES2+ paths.
	ModeTLSWithMTLS
)

// Config holds the listener configuration the gateway needs.
type Config struct {
	// CertFile and KeyFile are the gateway's server cert and key.
	// Empty means plain HTTP.
	CertFile string
	KeyFile  string

	// ES2PlusClientCAFile is a PEM bundle of CAs whose-issued
	// client certs are accepted on ES2+ paths. Empty means no
	// mTLS — TLS is one-way.
	ES2PlusClientCAFile string
}

// Mode returns the inferred mode from the config.
func (c Config) Mode() ListenerMode {
	if c.CertFile == "" || c.KeyFile == "" {
		return ModePlainHTTP
	}
	if c.ES2PlusClientCAFile == "" {
		return ModeTLS
	}
	return ModeTLSWithMTLS
}

// BuildTLSConfig constructs a *tls.Config matching the supplied
// Config. Returns nil if the mode is plain HTTP.
//
// When mTLS is configured, the TLS listener uses RequestClientCert
// — not RequireAndVerifyClientCert — so non-ES2+ routes (admin UI,
// /v1/*) can still connect without a client cert. The ES2+
// middleware enforces verification per request.
func BuildTLSConfig(cfg Config) (*tls.Config, error) {
	if cfg.Mode() == ModePlainHTTP {
		return nil, nil
	}

	cert, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("tlsconf: load server cert: %w", err)
	}

	tlsCfg := &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{cert},
	}

	if cfg.Mode() == ModeTLSWithMTLS {
		clientCAs, err := loadCAPool(cfg.ES2PlusClientCAFile)
		if err != nil {
			return nil, fmt.Errorf("tlsconf: load client CA: %w", err)
		}
		tlsCfg.ClientAuth = tls.VerifyClientCertIfGiven
		tlsCfg.ClientCAs = clientCAs
	}

	return tlsCfg, nil
}

// LoadES2PlusCAPool loads the same CA pool that BuildTLSConfig uses
// for client-cert verification. The middleware re-uses it to verify
// the request-level chain when a client cert is presented.
func LoadES2PlusCAPool(path string) (*x509.CertPool, error) {
	if path == "" {
		return nil, errors.New("tlsconf: ES2PlusClientCAFile is empty")
	}
	return loadCAPool(path)
}

func loadCAPool(path string) (*x509.CertPool, error) {
	pemBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	rest := pemBytes
	count := 0
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		c, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("tlsconf: parse CA cert: %w", err)
		}
		pool.AddCert(c)
		count++
	}
	if count == 0 {
		return nil, fmt.Errorf("tlsconf: no CA certs found in %s", path)
	}
	return pool, nil
}

// ES2PlusMTLSMiddleware returns middleware that requires a
// verified client cert on ES2+ paths and lets everything else
// through. If pool is nil, mTLS is disabled and the middleware
// is a no-op.
func ES2PlusMTLSMiddleware(pool *x509.CertPool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if pool == nil || !strings.HasPrefix(r.URL.Path, "/gsma/rsp2/es2plus/") {
				next.ServeHTTP(w, r)
				return
			}
			if r.TLS == nil {
				writeProblem(w, http.StatusUpgradeRequired,
					"ES2+ requires TLS; reconnect over HTTPS")
				return
			}
			if len(r.TLS.PeerCertificates) == 0 {
				writeProblem(w, http.StatusUnauthorized,
					"ES2+ requires a client certificate (mTLS)")
				return
			}
			// Verify the chain explicitly. tls.VerifyClientCertIfGiven
			// already validated against ClientCAs at handshake; this
			// is belt-and-suspenders against future config drift.
			leaf := r.TLS.PeerCertificates[0]
			intermediates := x509.NewCertPool()
			for _, c := range r.TLS.PeerCertificates[1:] {
				intermediates.AddCert(c)
			}
			if _, err := leaf.Verify(x509.VerifyOptions{
				Roots:         pool,
				Intermediates: intermediates,
				KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageAny},
			}); err != nil {
				writeProblem(w, http.StatusUnauthorized,
					fmt.Sprintf("client cert chain does not verify: %v", err))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func writeProblem(w http.ResponseWriter, status int, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w,
		`{"type":"about:blank","title":%q,"status":%d,"detail":%q}`,
		http.StatusText(status), status, detail)
}
