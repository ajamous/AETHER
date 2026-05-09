// Package hsmclient is the Go client for the Aether HSM broker.
//
// One client per service. Operations mirror the broker's HTTP+JSON
// surface 1:1; when the gRPC migration lands, this package's public
// signature does not change — only the wire encoding underneath does.
//
// All cryptographic operations are pre-hash for Sign (the caller
// supplies the digest, matching PKCS#11 semantics and SGP.22 §H.5
// algorithm-choice locality) and reference-by-handle for DeriveKey
// (the derived bytes do not cross the network).
package hsmclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client talks to one HSM broker.
type Client struct {
	base string
	hc   *http.Client
}

// New builds a Client. baseURL is the broker's HTTP root (e.g.
// "http://hsm-broker:8443"). A 10s default timeout is applied to
// each request; pass WithHTTPClient to override.
func New(baseURL string, opts ...Option) *Client {
	c := &Client{
		base: strings.TrimRight(baseURL, "/"),
		hc:   &http.Client{Timeout: 10 * time.Second},
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Option mutates a Client during construction.
type Option func(*Client)

// WithHTTPClient supplies a custom http.Client (for mTLS, retries, etc.).
func WithHTTPClient(hc *http.Client) Option { return func(c *Client) { c.hc = hc } }

// --- Wire types (mirror services/hsm-broker/api/v1) -----------------------

type Curve string

const (
	CurveP256            Curve = "P256"
	CurveBrainpoolP256r1 Curve = "BRAINPOOL_P256_R1"
)

type HashAlg string

const HashSHA256 HashAlg = "SHA256"

type KeyKind string

const (
	KeyKindECDSA KeyKind = "ECDSA"
	KeyKindECKA  KeyKind = "ECKA"
)

type KeyHandle struct {
	ID    string  `json:"id"`
	Label string  `json:"label"`
	Kind  KeyKind `json:"kind"`
	Curve Curve   `json:"curve"`
}

type GenerateKeyPairResponse struct {
	Handle    KeyHandle `json:"handle"`
	PublicKey []byte    `json:"public_key"`
}

type SignResponse struct {
	SignatureDER []byte `json:"signature_der"`
}

type DeriveKeyResponse struct {
	SessionKeyID string `json:"session_key_id"`
}

type ListKeysResponse struct {
	Keys []KeyHandle `json:"keys"`
}

type HealthResponse struct {
	Ready          bool   `json:"ready"`
	Backend        string `json:"backend"`
	ActiveSessions uint32 `json:"active_sessions"`
}

// --- Operations -----------------------------------------------------------

// Health probes the broker.
func (c *Client) Health(ctx context.Context) (*HealthResponse, error) {
	var out HealthResponse
	if err := c.do(ctx, http.MethodGet, "/v1/health", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GenerateKeyPair creates an EC keypair. Returns the handle (with
// opaque ID) and the public key (uncompressed X9.63 point).
func (c *Client) GenerateKeyPair(ctx context.Context, label string, kind KeyKind, curve Curve) (*GenerateKeyPairResponse, error) {
	req := map[string]any{"label": label, "kind": string(kind), "curve": string(curve)}
	var out GenerateKeyPairResponse
	if err := c.do(ctx, http.MethodPost, "/v1/generate-key-pair", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Sign computes ECDSA-SHA-256 over the supplied digest using the
// HSM-resident key referenced by keyID. SGP.22 §H.5.
func (c *Client) Sign(ctx context.Context, keyID string, digest []byte) (*SignResponse, error) {
	req := map[string]any{
		"key_id":     keyID,
		"digest":     digest,
		"digest_alg": string(HashSHA256),
	}
	var out SignResponse
	if err := c.do(ctx, http.MethodPost, "/v1/sign", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeriveKey runs ECKA against peerPub, KDFs with sharedInfo per
// SGP.22 §2.6.4, and returns a handle to the derived session key.
// The derived bytes never cross the network.
func (c *Client) DeriveKey(ctx context.Context, keyID string, peerPub, sharedInfo []byte, keyLen uint32) (*DeriveKeyResponse, error) {
	req := map[string]any{
		"key_id":      keyID,
		"peer_public": peerPub,
		"shared_info": sharedInfo,
		"key_len":     keyLen,
	}
	var out DeriveKeyResponse
	if err := c.do(ctx, http.MethodPost, "/v1/derive-key", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListKeys returns broker-resident keys, optionally filtered by
// label prefix.
func (c *Client) ListKeys(ctx context.Context, labelPrefix string) (*ListKeysResponse, error) {
	req := map[string]any{"label_prefix": labelPrefix}
	var out ListKeysResponse
	if err := c.do(ctx, http.MethodPost, "/v1/list-keys", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// --- internals ------------------------------------------------------------

// Error is returned for non-2xx HTTP responses. Carries the broker's
// RFC 7807 problem detail so callers can switch on status if needed.
type Error struct {
	Status int
	Detail string
}

func (e *Error) Error() string { return fmt.Sprintf("hsmclient: %d %s", e.Status, e.Detail) }

func (c *Client) do(ctx context.Context, method, path string, body any, dst any) error {
	var rdr io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("hsmclient: marshal: %w", err)
		}
		rdr = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, rdr)
	if err != nil {
		return fmt.Errorf("hsmclient: request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("hsmclient: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		var prob struct{ Detail string `json:"detail"` }
		_ = json.NewDecoder(resp.Body).Decode(&prob)
		if prob.Detail == "" {
			prob.Detail = http.StatusText(resp.StatusCode)
		}
		return &Error{Status: resp.StatusCode, Detail: prob.Detail}
	}
	if dst == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
		return fmt.Errorf("hsmclient: decode: %w", err)
	}
	return nil
}

// IsNotFound reports whether err indicates the key wasn't found
// (broker returns 404 for missing key handles).
func IsNotFound(err error) bool {
	var e *Error
	if !errors.As(err, &e) {
		return false
	}
	return e.Status == http.StatusNotFound
}
