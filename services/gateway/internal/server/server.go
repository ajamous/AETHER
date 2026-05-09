// Package server is the Aether API gateway HTTP transport.
//
// Two surfaces:
//
//	/gsma/rsp2/es2plus/...  for upstream BSS (SGP.22 §5.4)
//	/v1/...                 for the admin UI and operator integrations
package server

import (
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
}

type Config struct {
	ProfileBuilder string
	SMDPPlus       string
	CertMgr        string
	SMDS           string
	EIM            string

	// TLS holds the listener configuration. Empty TLS means plain HTTP.
	TLS tlsconf.Config
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

	// Wrap in the ES2+ mTLS gate. When es2plusClientCAs is nil
	// (mTLS disabled, lab default) this is a no-op pass-through;
	// when populated it requires a verified client cert on
	// /gsma/rsp2/es2plus/* and lets everything else through
	// unchanged.
	return tlsconf.ES2PlusMTLSMiddleware(s.es2plusClientCAs)(mux)
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
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
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
}
type DownloadOrderResponse struct {
	ICCID string `json:"iccid"`
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
	// Real implementation: reserve a profile from inventory, persist
	// the order, return the ICCID. Skeleton response echoes back.
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
