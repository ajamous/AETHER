// Package certmgrclient is the Go client for the Aether certmgr service.
//
// Services that need to verify a peer's certificate chain — most
// notably smdp-plus when handling the eUICC's authenticateClient
// response per SGP.22 §5.7.5 — fetch the trust store and any
// intermediates from certmgr at startup using this client.
package certmgrclient

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	base string
	hc   *http.Client
}

func New(baseURL string) *Client {
	return &Client{
		base: strings.TrimRight(baseURL, "/"),
		hc:   &http.Client{Timeout: 10 * time.Second},
	}
}

// TrustStore fetches and parses the configured CI roots from
// `GET /v1/trust-store/pem`.
func (c *Client) TrustStore(ctx context.Context) ([]*x509.Certificate, error) {
	return c.getCerts(ctx, "/v1/trust-store/pem")
}

// Intermediates fetches and parses the configured intermediate CAs
// (e.g. EUM) from `GET /v1/intermediates/pem`. Returns nil, nil if
// none are loaded.
func (c *Client) Intermediates(ctx context.Context) ([]*x509.Certificate, error) {
	return c.getCerts(ctx, "/v1/intermediates/pem")
}

func (c *Client) getCerts(ctx context.Context, path string) ([]*x509.Certificate, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return nil, fmt.Errorf("certmgrclient: request: %w", err)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("certmgrclient: GET %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("certmgrclient: GET %s: status %d", path, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("certmgrclient: read: %w", err)
	}

	var out []*x509.Certificate
	rest := body
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
			return nil, fmt.Errorf("certmgrclient: parse: %w", err)
		}
		out = append(out, c)
	}
	return out, nil
}
