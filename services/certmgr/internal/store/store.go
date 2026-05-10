// Package store loads and holds the certificate set Aether's
// SM-DP+ depends on: the trust store (CI roots) and the SM-DP+
// identity certs (DPtls, DPauth, DPpb).
//
// Per SGP.22 §4.5 the SM-DP+ presents its identity certs and verifies
// peer chains against a trusted set of CI roots. Aether keeps both
// of those concerns in one process so the verification policy and the
// PKI material stay consistent.
package store

import (
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"
)

// Mode picks between the SGP.26 test PKI (lab) and a real GSMA CI
// trust set (production). See ADR 0004.
type Mode string

const (
	ModeLab        Mode = "lab"
	ModeProduction Mode = "production"
)

// Identity names a loaded SM-DP+ identity certificate.
type Identity string

const (
	IdentityDPTLS  Identity = "DPtls"
	IdentityDPAuth Identity = "DPauth"
	IdentityDPpb   Identity = "DPpb"
)

// Cert is a loaded certificate plus the PEM source it came from.
type Cert struct {
	Name     Identity
	Cert     *x509.Certificate
	PEM      []byte
	LoadedAt time.Time
}

// Store holds the trust store and the identity certs.
type Store struct {
	mu sync.RWMutex

	mode          Mode
	roots         *x509.CertPool
	rootCerts     []*x509.Certificate
	intermediates *x509.CertPool
	intCerts      []*x509.Certificate
	identity      map[Identity]*Cert
}

// Config configures Store loading.
type Config struct {
	Mode           Mode
	TrustStorePath string
	// IntermediatesPath is an optional bundle (PEM, may contain multiple
	// certs) of intermediate CA certs used to chain the identity certs
	// up to the trust store. SGP.22 deployments typically have an EUM
	// intermediate between the leaf and the CI root.
	IntermediatesPath string
	IdentityPaths     map[Identity]string // optional; only the names provided are loaded
}

// New loads certificates from disk per cfg and returns a verified Store.
//
// Verification:
//  1. The trust store must contain at least one CA certificate.
//  2. Every identity cert must verify against the trust store.
//  3. (Caller responsibility) the configured mode must match the
//     issuer set; the public ChainsVerify() helper exposes that
//     check for the HTTP layer.
func New(cfg Config) (*Store, error) {
	if cfg.Mode != ModeLab && cfg.Mode != ModeProduction {
		return nil, fmt.Errorf("store: invalid mode %q", cfg.Mode)
	}

	rootPEM, err := os.ReadFile(cfg.TrustStorePath)
	if err != nil {
		return nil, fmt.Errorf("store: read trust store: %w", err)
	}
	pool := x509.NewCertPool()
	roots, err := decodeAllCerts(rootPEM)
	if err != nil {
		return nil, fmt.Errorf("store: decode trust store: %w", err)
	}
	if len(roots) == 0 {
		return nil, errors.New("store: trust store contains no certificates")
	}
	for _, r := range roots {
		pool.AddCert(r)
	}

	intPool := x509.NewCertPool()
	var intCerts []*x509.Certificate
	if cfg.IntermediatesPath != "" {
		intPEM, err := os.ReadFile(cfg.IntermediatesPath)
		if err != nil {
			return nil, fmt.Errorf("store: read intermediates: %w", err)
		}
		intCerts, err = decodeAllCerts(intPEM)
		if err != nil {
			return nil, fmt.Errorf("store: decode intermediates: %w", err)
		}
		for _, c := range intCerts {
			intPool.AddCert(c)
		}
	}

	s := &Store{
		mode:          cfg.Mode,
		roots:         pool,
		rootCerts:     roots,
		intermediates: intPool,
		intCerts:      intCerts,
		identity:      make(map[Identity]*Cert),
	}

	for name, path := range cfg.IdentityPaths {
		c, err := loadCert(name, path)
		if err != nil {
			return nil, err
		}
		if err := s.verify(c.Cert); err != nil {
			return nil, fmt.Errorf("store: verify %s: %w", name, err)
		}
		s.identity[name] = c
	}
	return s, nil
}

// Mode returns the configured mode.
func (s *Store) Mode() Mode {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.mode
}

// TrustStore returns a fresh CertPool of CI roots.
func (s *Store) TrustStore() *x509.CertPool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.roots.Clone()
}

// Roots returns the parsed root certificates.
func (s *Store) Roots() []*x509.Certificate {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*x509.Certificate, len(s.rootCerts))
	copy(out, s.rootCerts)
	return out
}

// Identities returns the loaded identity certs.
func (s *Store) Identities() map[Identity]*Cert {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[Identity]*Cert, len(s.identity))
	for k, v := range s.identity {
		out[k] = v
	}
	return out
}

// Identity returns the named identity cert if loaded.
func (s *Store) Identity(name Identity) (*Cert, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.identity[name]
	return c, ok
}

// VerifyChain verifies a peer chain (e.g. an eUICC's cert chain)
// against the loaded trust store.
func (s *Store) VerifyChain(leaf *x509.Certificate, intermediates []*x509.Certificate) error {
	s.mu.RLock()
	roots := s.roots
	s.mu.RUnlock()

	pool := x509.NewCertPool()
	for _, c := range intermediates {
		pool.AddCert(c)
	}
	_, err := leaf.Verify(x509.VerifyOptions{
		Roots:         roots,
		Intermediates: pool,
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
	})
	return err
}

func (s *Store) verify(c *x509.Certificate) error {
	_, err := c.Verify(x509.VerifyOptions{
		Roots:         s.roots,
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
		Intermediates: s.intermediates,
	})
	return err
}

// Intermediates returns the loaded intermediate certs (e.g. EUM).
func (s *Store) Intermediates() []*x509.Certificate {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*x509.Certificate, len(s.intCerts))
	copy(out, s.intCerts)
	return out
}

// --- helpers --------------------------------------------------------------

func loadCert(name Identity, path string) (*Cert, error) {
	if path == "" {
		return nil, fmt.Errorf("store: empty path for identity %q", name)
	}
	pemBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("store: read %s: %w", name, err)
	}
	cert, err := decodeFirstCert(pemBytes)
	if err != nil {
		return nil, fmt.Errorf("store: decode %s: %w", name, err)
	}
	return &Cert{
		Name:     name,
		Cert:     cert,
		PEM:      pemBytes,
		LoadedAt: time.Now(),
	}, nil
}

func decodeAllCerts(pemBytes []byte) ([]*x509.Certificate, error) {
	var out []*x509.Certificate
	rest := pemBytes
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
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

func decodeFirstCert(pemBytes []byte) (*x509.Certificate, error) {
	for {
		block, rest := pem.Decode(pemBytes)
		if block == nil {
			return nil, errors.New("no CERTIFICATE block found")
		}
		if block.Type == "CERTIFICATE" {
			return x509.ParseCertificate(block.Bytes)
		}
		pemBytes = rest
	}
}
