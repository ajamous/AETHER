// Package server implements the HSM broker's HTTP+JSON transport.
//
// The contract is the broker.Broker interface; the server is a thin
// adapter that translates JSON requests to broker calls and back. A
// gRPC server can be added behind the same broker.Broker without
// touching backends or callers — see services/hsm-broker/api/v1/hsm.proto.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	hsmv1 "github.com/ajamous/aether/services/hsm-broker/api/v1"
	"github.com/ajamous/aether/services/hsm-broker/internal/broker"
	"github.com/ajamous/aether/services/hsm-broker/internal/metrics"
)

// Server adapts a broker.Broker over HTTP+JSON.
type Server struct {
	b              broker.Broker
	signLatencySec *metrics.LatencyHistogram
}

// New constructs a Server backed by b.
func New(b broker.Broker) *Server {
	return &Server{
		b: b,
		signLatencySec: metrics.NewLatencyHistogram(
			"aether_hsm_sign_duration_seconds",
			"Wall-clock latency of HSM Sign operations from the broker's perspective.",
		),
	}
}

// Routes returns an http.Handler with all endpoints mounted.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/sign", s.handleSign)
	mux.HandleFunc("POST /v1/decrypt", s.handleDecrypt)
	mux.HandleFunc("POST /v1/derive-key", s.handleDeriveKey)
	mux.HandleFunc("POST /v1/generate-key-pair", s.handleGenerateKeyPair)
	mux.HandleFunc("POST /v1/list-keys", s.handleListKeys)
	mux.HandleFunc("GET /v1/health", s.handleHealth)
	mux.HandleFunc("GET /metrics", s.handleMetrics)
	return mux
}

// handleMetrics emits a small text/plain Prometheus exposition.
// Hand-rolled rather than pulling in client_golang — see
// internal/metrics/metrics.go for the rationale.
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	hr, err := s.b.Health(r.Context())
	ready := 0
	backend := "unknown"
	if err == nil && hr != nil {
		if hr.Ready {
			ready = 1
		}
		backend = hr.Backend
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	_, _ = fmt.Fprintf(w, `# HELP aether_hsm_broker_ready 1 when the broker reports a live PKCS#11 session.
# TYPE aether_hsm_broker_ready gauge
aether_hsm_broker_ready{backend=%q} %d
`, backend, ready)
	s.signLatencySec.Write(w)
}

// ListenAndServe runs the HTTP server until ctx is cancelled.
func (s *Server) ListenAndServe(ctx context.Context, addr string) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           s.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
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

// --- handlers -------------------------------------------------------------

func (s *Server) handleSign(w http.ResponseWriter, r *http.Request) {
	var req hsmv1.SignRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	start := time.Now()
	resp, err := s.b.Sign(r.Context(), &req)
	s.signLatencySec.Observe(time.Since(start))
	writeJSON(w, resp, err)
}

func (s *Server) handleDecrypt(w http.ResponseWriter, r *http.Request) {
	var req hsmv1.DecryptRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	resp, err := s.b.Decrypt(r.Context(), &req)
	writeJSON(w, resp, err)
}

func (s *Server) handleDeriveKey(w http.ResponseWriter, r *http.Request) {
	var req hsmv1.DeriveKeyRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	resp, err := s.b.DeriveKey(r.Context(), &req)
	writeJSON(w, resp, err)
}

func (s *Server) handleGenerateKeyPair(w http.ResponseWriter, r *http.Request) {
	var req hsmv1.GenerateKeyPairRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	resp, err := s.b.GenerateKeyPair(r.Context(), &req)
	writeJSON(w, resp, err)
}

func (s *Server) handleListKeys(w http.ResponseWriter, r *http.Request) {
	var req hsmv1.ListKeysRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	resp, err := s.b.ListKeys(r.Context(), &req)
	writeJSON(w, resp, err)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	resp, err := s.b.Health(r.Context())
	writeJSON(w, resp, err)
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

func writeJSON(w http.ResponseWriter, v any, err error) {
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

func writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, broker.ErrKeyNotFound):
		writeProblem(w, http.StatusNotFound, err.Error())
	case errors.Is(err, broker.ErrInvalidArgument),
		errors.Is(err, broker.ErrUnsupportedCurve),
		errors.Is(err, broker.ErrUnsupportedKind):
		writeProblem(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, broker.ErrBackendUnhealthy):
		writeProblem(w, http.StatusServiceUnavailable, err.Error())
	default:
		writeProblem(w, http.StatusInternalServerError, err.Error())
	}
}

// writeProblem emits an RFC 7807 problem+json response.
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
