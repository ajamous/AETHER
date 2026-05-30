// Package server is the Aether API gateway HTTP transport.
//
// Two surfaces:
//
//	/gsma/rsp2/es2plus/...  for upstream BSS (SGP.22 §5.4)
//	/v1/...                 for the admin UI and operator integrations
package server

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	gatewayv1 "github.com/ajamous/aether/services/gateway/api/v1"
	"github.com/ajamous/aether/services/gateway/internal/metrics"
	"github.com/ajamous/aether/services/gateway/internal/oidc"
	"github.com/ajamous/aether/services/gateway/internal/ratelimit"
	"github.com/ajamous/aether/services/gateway/internal/tlsconf"
)

type Server struct {
	profileBuilder string
	smdpPlus       string
	certmgr        string
	smds           string
	eim            string
	httpClient     *http.Client

	// TLS state. tlsConfig is the listener's TLS settings (nil = plain HTTP).
	// es2plusClientCAs is non-nil when ES2+ mTLS is enforced.
	tlsConfig        *tls.Config
	es2plusClientCAs *x509.CertPool

	// metrics exposed at /metrics. The 401 counter on the ES2+
	// surface drives the AetherES2PlusUnauthorizedSpike alert in
	// deployments/observability/.
	es2plusUnauthorized *metrics.LabeledCounter

	// Rate limiter for /gsma/rsp2/* paths. Nil = disabled.
	rateLimiter       *ratelimit.Limiter
	rateLimitRejected *metrics.LabeledCounter

	// OIDC verifier for /v1/* admin paths. Nil = disabled.
	oidcVerifier      *oidc.Verifier
	adminUnauthorized *metrics.LabeledCounter
}

type Config struct {
	ProfileBuilder string
	SMDPPlus       string
	CertMgr        string
	SMDS           string
	EIM            string

	// TLS holds the listener configuration. Empty TLS means plain HTTP.
	TLS tlsconf.Config

	// Rate limiting for the public /gsma/rsp2/* surface. Both must
	// be > 0 to enable; lab default is disabled.
	RateLimitRPS   float64
	RateLimitBurst int

	// OIDC for the /v1/* admin surface. Nil OIDCVerifier disables
	// the gate (lab default). The verifier is built outside the
	// server constructor because OIDC discovery is a network call
	// and should be done at startup with an explicit context.
	OIDCVerifier *oidc.Verifier
}

// New constructs a Server. Returns an error if the TLS configuration
// cannot be loaded.
func New(cfg Config) (*Server, error) {
	tlsCfg, err := tlsconf.BuildTLSConfig(cfg.TLS)
	if err != nil {
		return nil, err
	}
	var clientCAs *x509.CertPool
	if cfg.TLS.Mode() == tlsconf.ModeTLSWithMTLS {
		clientCAs, err = tlsconf.LoadES2PlusCAPool(cfg.TLS.ES2PlusClientCAFile)
		if err != nil {
			return nil, err
		}
	}
	return &Server{
		profileBuilder:   strings.TrimRight(cfg.ProfileBuilder, "/"),
		smdpPlus:         strings.TrimRight(cfg.SMDPPlus, "/"),
		certmgr:          strings.TrimRight(cfg.CertMgr, "/"),
		smds:             strings.TrimRight(cfg.SMDS, "/"),
		eim:              strings.TrimRight(cfg.EIM, "/"),
		httpClient:       &http.Client{Timeout: 10 * time.Second},
		tlsConfig:        tlsCfg,
		es2plusClientCAs: clientCAs,
		es2plusUnauthorized: metrics.NewLabeledCounter(
			"aether_gateway_es2plus_unauthorized_total",
			"Count of 401 Unauthorized responses on /gsma/rsp2/es2plus/* by reason.",
			"reason",
			"no_tls", "no_client_cert", "chain_invalid",
		),
		rateLimiter: ratelimit.New(cfg.RateLimitRPS, cfg.RateLimitBurst),
		rateLimitRejected: metrics.NewLabeledCounter(
			"aether_gateway_ratelimit_rejected_total",
			"Count of 429 Too Many Requests responses on /gsma/rsp2/* by path class.",
			"class",
			"es2plus", "es9plus",
		),
		oidcVerifier: cfg.OIDCVerifier,
		adminUnauthorized: metrics.NewLabeledCounter(
			"aether_gateway_admin_unauthorized_total",
			"Count of 401 Unauthorized responses on /v1/* admin paths by reason.",
			"reason",
			string(oidc.ReasonNoToken),
			string(oidc.ReasonMalformed),
			string(oidc.ReasonUnsupportedAlg),
			string(oidc.ReasonUnknownKID),
			string(oidc.ReasonBadSignature),
			string(oidc.ReasonWrongIssuer),
			string(oidc.ReasonWrongAudience),
			string(oidc.ReasonExpired),
			string(oidc.ReasonNotYetValid),
			string(oidc.ReasonJWKSFetchFailed),
		),
	}, nil
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	// ES2+ (BSS-facing) — SGP.22 §5.4
	mux.HandleFunc("POST /gsma/rsp2/es2plus/downloadOrder", s.handleDownloadOrder)
	mux.HandleFunc("POST /gsma/rsp2/es2plus/confirmOrder", s.handleConfirmOrder)
	mux.HandleFunc("POST /gsma/rsp2/es2plus/cancelOrder", s.handleCancelOrder)
	mux.HandleFunc("POST /gsma/rsp2/es2plus/releaseProfile", s.handleReleaseProfile)
	mux.HandleFunc("POST /gsma/rsp2/es2plus/handleNotification", s.handleES2HandleNotification)

	// UI / operator
	mux.HandleFunc("GET /v1/health", s.handleHealth)
	mux.HandleFunc("GET /v1/templates", s.proxy("profile-builder", "/v1/templates"))
	mux.HandleFunc("GET /v1/templates/{name}", s.proxyPath("profile-builder", "/v1/templates"))
	mux.HandleFunc("GET /v1/certs", s.proxy("certmgr", "/v1/certs"))
	mux.HandleFunc("GET /v1/trust-store", s.proxy("certmgr", "/v1/trust-store"))
	mux.HandleFunc("GET /v1/smds/events", s.proxy("smds", "/v1/events"))
	mux.HandleFunc("GET /v1/eim/devices", s.proxy("eim", "/v1/devices"))
	mux.HandleFunc("GET /v1/openapi.yaml", s.handleOpenAPI)
	mux.HandleFunc("GET /metrics", s.handleMetrics)

	// Middleware order: rate limit FIRST, then mTLS gate, then OIDC.
	// rate-limit before mTLS so a flood of cert-less requests can't
	// burn CPU on chain checks. OIDC is innermost because it only
	// applies to /v1/* and is a no-op for /gsma/rsp2/* — putting it
	// at the outside would be wasteful work for the public surface.
	// All three are no-op pass-throughs when their config is unset
	// (lab default).
	mtls := tlsconf.ES2PlusMTLSMiddleware(s.es2plusClientCAs, s.es2plusUnauthorized)
	rate := ratelimit.Middleware(s.rateLimiter, func(class string) {
		s.rateLimitRejected.Inc(class)
	})
	admin := oidc.Middleware(s.oidcVerifier, func(reason oidc.Reason) {
		s.adminUnauthorized.Inc(string(reason))
	})
	return rate(mtls(admin(mux)))
}

func (s *Server) handleMetrics(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	s.es2plusUnauthorized.Write(w)
	s.rateLimitRejected.Write(w)
	s.adminUnauthorized.Write(w)
}

// handleOpenAPI serves the embedded OpenAPI 3.1 spec. /v1/openapi.yaml
// bypasses the OIDC gate (same shape as /v1/health and /metrics) so
// operators and CLI tooling can discover the API without first
// authenticating. The API surface stays gated.
func (s *Server) handleOpenAPI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write(gatewayv1.SpecBytes())
}

// ListenAndServe runs the gateway. If the configured mode includes
// TLS, it serves over HTTPS; otherwise plain HTTP (lab default).
func (s *Server) ListenAndServe(ctx context.Context, addr string) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           s.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
		TLSConfig:         s.tlsConfig,
	}
	errCh := make(chan error, 1)
	go func() {
		if s.tlsConfig != nil {
			// Cert/key already loaded into srv.TLSConfig.Certificates;
			// passing empty strings tells ListenAndServeTLS to use those.
			errCh <- srv.ListenAndServeTLS("", "")
			return
		}
		errCh <- srv.ListenAndServe()
	}()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

// TLSConfig exposes the loaded *tls.Config for tests that need to
// drive the listener via httptest.NewUnstartedServer.
func (s *Server) TLSConfig() *tls.Config { return s.tlsConfig }

// ES2PlusClientCAs exposes the client CA pool for tests.
func (s *Server) ES2PlusClientCAs() *x509.CertPool { return s.es2plusClientCAs }

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ready": true,
		"upstream": map[string]string{
			"profile_builder": s.profileBuilder,
			"smdp_plus":       s.smdpPlus,
			"certmgr":         s.certmgr,
		},
	})
}

// --- ES2+ skeletons -------------------------------------------------------

// SGP.22 §5.4.1 DownloadOrder: BSS asks the SM-DP+ to reserve a profile
// for a given EID (or any EID). Response carries an ICCID.
type DownloadOrderRequest struct {
	EID         string `json:"eid"`
	ICCID       string `json:"iccid"`
	ProfileType string `json:"profile_type"`

	// Subscriber, when present, makes the gateway prepare the profile
	// on the SM-DP+ (POST /v1/profiles/prepare). In a fully
	// spec-faithful deployment the profile content lives in the
	// SM-DP+'s inventory and ES2+ references it by ICCID / profile_type;
	// the in-tree flow carries it on the wire so a BSS can drive the
	// whole download path without a separate inventory-load step.
	Subscriber *OrderSubscriber `json:"subscriber,omitempty"`
}

// OrderSubscriber is the per-activation data the in-tree ES2+ flow
// carries so the SM-DP+ can build the profile. profile_type names the
// SM-DP+ template.
type OrderSubscriber struct {
	IMSI   string `json:"imsi"`
	MSISDN string `json:"msisdn"`
	Ki     []byte `json:"ki"`
	OPc    []byte `json:"opc"`
}

type DownloadOrderResponse struct {
	ICCID string `json:"iccid"`
	// MatchingID and ActivationCode are populated when the SM-DP+
	// prepared the profile (i.e. the order carried a subscriber
	// block). The BSS hands ActivationCode to the user as the
	// SGP.22 §4.1 string "LPA:1$<smdp address>$<matching id>".
	MatchingID     string `json:"matching_id,omitempty"`
	ActivationCode string `json:"activation_code,omitempty"`
}

// smdpPrepareRequest/Response mirror smdp-plus's POST
// /v1/profiles/prepare contract. The gateway owns this shape as a
// client; it must match smdp-plus's DisallowUnknownFields decoder.
type smdpPrepareRequest struct {
	Template   string                `json:"template,omitempty"`
	Subscriber smdpPrepareSubscriber `json:"subscriber"`
}
type smdpPrepareSubscriber struct {
	IMSI   string `json:"imsi"`
	ICCID  string `json:"iccid"`
	MSISDN string `json:"msisdn"`
	Ki     []byte `json:"ki"`
	OPc    []byte `json:"opc"`
}
type smdpPrepareResponse struct {
	ICCID          string `json:"iccid"`
	MatchingID     string `json:"matching_id"`
	ActivationCode string `json:"activation_code"`
}

func (s *Server) handleDownloadOrder(w http.ResponseWriter, r *http.Request) {
	var req DownloadOrderRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.ICCID == "" && req.ProfileType == "" {
		writeProblem(w, http.StatusBadRequest, "iccid or profile_type required")
		return
	}

	// When the order carries profile data, prepare it on the SM-DP+.
	if req.Subscriber != nil {
		if s.smdpPlus == "" {
			writeProblem(w, http.StatusServiceUnavailable, "smdp-plus not configured; cannot prepare profile")
			return
		}
		if req.ICCID == "" {
			writeProblem(w, http.StatusBadRequest, "iccid required to prepare a profile")
			return
		}
		prep := smdpPrepareRequest{
			Template: req.ProfileType,
			Subscriber: smdpPrepareSubscriber{
				IMSI:   req.Subscriber.IMSI,
				ICCID:  req.ICCID,
				MSISDN: req.Subscriber.MSISDN,
				Ki:     req.Subscriber.Ki,
				OPc:    req.Subscriber.OPc,
			},
		}
		var out smdpPrepareResponse
		status, err := s.postJSON(r.Context(), s.smdpPlus+"/v1/profiles/prepare", prep, &out)
		if err != nil {
			writeProblem(w, http.StatusBadGateway, fmt.Sprintf("smdp-plus prepare: %v", err))
			return
		}
		if status/100 != 2 {
			writeProblem(w, status, fmt.Sprintf("smdp-plus prepare returned %d", status))
			return
		}
		writeJSON(w, http.StatusOK, DownloadOrderResponse{
			ICCID:          out.ICCID,
			MatchingID:     out.MatchingID,
			ActivationCode: out.ActivationCode,
		})
		return
	}

	// Skeleton path: reserve a profile from inventory (not yet
	// implemented); echo the ICCID back.
	writeJSON(w, http.StatusOK, DownloadOrderResponse{ICCID: req.ICCID})
}

// SGP.22 §5.4.2 ConfirmOrder: BSS confirms the order and provides
// optional matching ID + SMDP address.
type ConfirmOrderRequest struct {
	ICCID       string `json:"iccid"`
	MatchingID  string `json:"matching_id"`
	SMDPAddress string `json:"smdp_address"`
}
type ConfirmOrderResponse struct {
	MatchingID string `json:"matching_id"`
}

func (s *Server) handleConfirmOrder(w http.ResponseWriter, r *http.Request) {
	var req ConfirmOrderRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.ICCID == "" {
		writeProblem(w, http.StatusBadRequest, "iccid required")
		return
	}
	writeJSON(w, http.StatusOK, ConfirmOrderResponse{MatchingID: req.MatchingID})
}

func (s *Server) handleCancelOrder(w http.ResponseWriter, r *http.Request) {
	var req map[string]any
	if !decodeJSON(w, r, &req) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"cancelled": true})
}

func (s *Server) handleReleaseProfile(w http.ResponseWriter, r *http.Request) {
	var req map[string]any
	if !decodeJSON(w, r, &req) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"released": true})
}

func (s *Server) handleES2HandleNotification(w http.ResponseWriter, r *http.Request) {
	var req map[string]any
	if !decodeJSON(w, r, &req) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{})
}

// --- proxy helpers --------------------------------------------------------

func (s *Server) proxy(target, path string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		base := s.upstream(target)
		if base == "" {
			writeProblem(w, http.StatusServiceUnavailable, fmt.Sprintf("%s not configured", target))
			return
		}
		s.do(w, r, base+path)
	}
}

func (s *Server) proxyPath(target, prefix string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		base := s.upstream(target)
		if base == "" {
			writeProblem(w, http.StatusServiceUnavailable, fmt.Sprintf("%s not configured", target))
			return
		}
		// preserve the trailing path component (e.g. /v1/templates/foo)
		u, _ := url.Parse(base + prefix + r.URL.Path[len(prefix):])
		s.do(w, r, u.String())
	}
}

func (s *Server) upstream(name string) string {
	switch name {
	case "profile-builder":
		return s.profileBuilder
	case "smdp-plus":
		return s.smdpPlus
	case "certmgr":
		return s.certmgr
	case "smds":
		return s.smds
	case "eim":
		return s.eim
	}
	return ""
}

// postJSON sends body as JSON to dest and decodes a 2xx response into
// dst (when non-nil). Returns the upstream status code. Used for
// transformed upstream calls (e.g. ES2+ DownloadOrder → smdp-plus
// prepare) where the raw proxy in do() doesn't fit.
func (s *Server) postJSON(ctx context.Context, dest string, body, dst any) (int, error) {
	buf, err := json.Marshal(body)
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, dest, bytes.NewReader(buf))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if dst != nil && resp.StatusCode/100 == 2 {
		if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
			return resp.StatusCode, err
		}
	}
	return resp.StatusCode, nil
}

func (s *Server) do(w http.ResponseWriter, r *http.Request, dest string) {
	req, err := http.NewRequestWithContext(r.Context(), r.Method, dest, r.Body)
	if err != nil {
		writeProblem(w, http.StatusBadGateway, err.Error())
		return
	}
	for k, v := range r.Header {
		req.Header[k] = v
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		writeProblem(w, http.StatusBadGateway, err.Error())
		return
	}
	defer resp.Body.Close()
	for k, v := range resp.Header {
		w.Header()[k] = v
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

// --- helpers --------------------------------------------------------------

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeProblem(w, http.StatusBadRequest, fmt.Sprintf("invalid JSON: %v", err))
		return false
	}
	return true
}

//nolint:unparam // keep `status` for future non-200 success responses (201, 202)
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeProblem(w http.ResponseWriter, status int, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type":   "about:blank",
		"title":  http.StatusText(status),
		"status": status,
		"detail": detail,
	})
}
