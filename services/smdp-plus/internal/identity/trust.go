// Package identity also exposes the SM-DP+'s trust material — the
// CI roots and intermediate CAs it uses to verify the eUICC's cert
// chain on authenticateClient (SGP.22 §5.7.5).
//
// Loaded once at startup from certmgr; refreshed on operator
// signal in a future iteration.

package identity

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"

	"github.com/ajamous/aether/pkg/certmgrclient"
)

// TrustMaterial holds the parsed certmgr-served trust set.
type TrustMaterial struct {
	Roots         *x509.CertPool
	Intermediates *x509.CertPool

	rootCount         int
	intermediateCount int
}

// RootCount returns the number of CI root certs loaded.
func (t *TrustMaterial) RootCount() int { return t.rootCount }

// IntermediateCount returns the number of intermediate certs loaded.
func (t *TrustMaterial) IntermediateCount() int { return t.intermediateCount }

// FetchTrustMaterial pulls the trust store + intermediates from
// certmgr and builds the cert pools the verifier needs. An empty
// trust store is treated as a configuration error — without it the
// SM-DP+ has no way to authenticate eUICCs.
func FetchTrustMaterial(ctx context.Context, c *certmgrclient.Client) (*TrustMaterial, error) {
	if c == nil {
		return nil, errors.New("identity: certmgr client is required")
	}
	roots, err := c.TrustStore(ctx)
	if err != nil {
		return nil, fmt.Errorf("identity: fetch trust store: %w", err)
	}
	if len(roots) == 0 {
		return nil, errors.New("identity: certmgr returned an empty trust store")
	}

	rootPool := x509.NewCertPool()
	for _, r := range roots {
		rootPool.AddCert(r)
	}

	intCerts, err := c.Intermediates(ctx)
	if err != nil {
		return nil, fmt.Errorf("identity: fetch intermediates: %w", err)
	}
	intPool := x509.NewCertPool()
	for _, i := range intCerts {
		intPool.AddCert(i)
	}

	return &TrustMaterial{
		Roots:             rootPool,
		Intermediates:     intPool,
		rootCount:         len(roots),
		intermediateCount: len(intCerts),
	}, nil
}
