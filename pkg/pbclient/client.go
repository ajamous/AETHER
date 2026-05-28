// Package pbclient is the Go client for the Aether profile-builder.
//
// One client per service, mirroring pkg/hsmclient and
// pkg/certmgrclient: operations map 1:1 to profile-builder's
// HTTP+JSON surface. smdp-plus uses it at profile-prepare time to
// turn a template name + per-subscriber data into a DER-encoded SAIP
// UPP.
package pbclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client talks to one profile-builder.
type Client struct {
	base string
	hc   *http.Client
}

// New builds a Client. baseURL is the profile-builder's HTTP root
// (e.g. "http://profile-builder:8446"). A 10s default timeout applies
// to each request; pass WithHTTPClient to override.
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

// SubscriberData is the per-activation data profile-builder merges
// with a template to produce a UPP. Mirrors the request body of
// services/profile-builder/internal/template.SubscriberData.
type SubscriberData struct {
	IMSI   string `json:"imsi"`
	ICCID  string `json:"iccid"`
	MSISDN string `json:"msisdn"`
	Ki     []byte `json:"ki"`
	OPc    []byte `json:"opc"`
}

// BuildResponse carries the DER-encoded SAIP UPP profile-builder
// produced. profile-builder's full envelope also echoes the profile
// and subscriber for human inspection; we decode only the bytes
// smdp-plus needs (unknown JSON fields are ignored).
type BuildResponse struct {
	SAIP []byte `json:"saip_der"`
	Note string `json:"_note,omitempty"`
}

// HealthResponse is profile-builder's readiness probe.
type HealthResponse struct {
	Ready bool `json:"ready"`
}

// Health probes profile-builder.
func (c *Client) Health(ctx context.Context) (*HealthResponse, error) {
	var out HealthResponse
	if err := c.do(ctx, http.MethodGet, "/v1/health", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Build asks profile-builder to merge the named template with sub and
// return the resulting SAIP UPP. POST /v1/templates/{name}/build.
func (c *Client) Build(ctx context.Context, template string, sub SubscriberData) (*BuildResponse, error) {
	if template == "" {
		return nil, errors.New("pbclient: empty template name")
	}
	path := "/v1/templates/" + url.PathEscape(template) + "/build"
	var out BuildResponse
	if err := c.do(ctx, http.MethodPost, path, sub, &out); err != nil {
		return nil, err
	}
	if len(out.SAIP) == 0 {
		return nil, errors.New("pbclient: profile-builder returned an empty SAIP UPP")
	}
	return &out, nil
}

// Error is returned for non-2xx HTTP responses, carrying
// profile-builder's RFC 7807 problem detail.
type Error struct {
	Status int
	Detail string
}

func (e *Error) Error() string { return fmt.Sprintf("pbclient: %d %s", e.Status, e.Detail) }

func (c *Client) do(ctx context.Context, method, path string, body, dst any) error {
	var rdr io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("pbclient: marshal: %w", err)
		}
		rdr = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, rdr)
	if err != nil {
		return fmt.Errorf("pbclient: request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("pbclient: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		var prob struct {
			Detail string `json:"detail"`
		}
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
		return fmt.Errorf("pbclient: decode: %w", err)
	}
	return nil
}

// IsNotFound reports whether err indicates the template wasn't found
// (profile-builder returns 404 for unknown templates).
func IsNotFound(err error) bool {
	var e *Error
	if !errors.As(err, &e) {
		return false
	}
	return e.Status == http.StatusNotFound
}
