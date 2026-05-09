// Package server is the audit log HTTP transport.
package server

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/ajamous/aether/pkg/hsmclient"
	"github.com/ajamous/aether/services/audit/internal/anchor"
	"github.com/ajamous/aether/services/audit/internal/chain"
)

// AnchorSigner bundles everything the /v1/anchor handler needs to
// produce a SAS-SM-style signed timeline anchor (length, tail hash,
// timestamp; ECDSA-SHA-256 over the DER encoding via hsm-broker).
//
// All fields must be set to enable signing; nil signer means
// /v1/anchor returns an unsigned anchor — same shape minus the
// signature. Lab default is unsigned to keep `make lab-up`
// HSM-free.
type AnchorSigner struct {
	Broker *hsmclient.Client
	KeyID  string
}

type Server struct {
	ledger chain.Backend
	signer *AnchorSigner
	logger *slog.Logger
}

// Config bundles the optional knobs. Pass nil for lab defaults.
type Config struct {
	Signer *AnchorSigner
	Logger *slog.Logger
}

func New(l chain.Backend, cfgs ...Config) *Server {
	srv := &Server{ledger: l, logger: slog.Default()}
	for _, c := range cfgs {
		if c.Signer != nil {
			srv.signer = c.Signer
		}
		if c.Logger != nil {
			srv.logger = c.Logger
		}
	}
	return srv
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/events", s.handleAppend)
	mux.HandleFunc("GET /v1/events", s.handleList)
	mux.HandleFunc("GET /v1/events/{seq}", s.handleGet)
	mux.HandleFunc("GET /v1/verify", s.handleVerify)
	mux.HandleFunc("GET /v1/anchor", s.handleAnchor)
	mux.HandleFunc("GET /v1/health", s.handleHealth)
	mux.HandleFunc("GET /metrics", s.handleMetrics)
	return mux
}

// handleMetrics emits a tiny Prometheus exposition. The
// deployments/observability/ alert rules consume these. The
// chain-OK gauge is the most important: a 0 means tamper
// detection has fired, which is always Sev-1.
func (s *Server) handleMetrics(w http.ResponseWriter, _ *http.Request) {
	r := s.ledger.Verify()
	ok := 0
	if r.OK {
		ok = 1
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	_, _ = fmt.Fprintf(w, `# HELP aether_audit_chain_ok 1 when the hash chain verifies cleanly end to end.
# TYPE aether_audit_chain_ok gauge
aether_audit_chain_ok %d
# HELP aether_audit_chain_length Number of entries in the audit ledger.
# TYPE aether_audit_chain_length gauge
aether_audit_chain_length %d
`, ok, r.Length)
}

func (s *Server) ListenAndServe(ctx context.Context, addr string) error {
	srv := &http.Server{Addr: addr, Handler: s.Routes(), ReadHeaderTimeout: 10 * time.Second}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()
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

func (s *Server) handleAppend(w http.ResponseWriter, r *http.Request) {
	body, err := readAllJSON(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	e, err := s.ledger.Append(body)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, e)
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	var since uint64
	if v := r.URL.Query().Get("since"); v != "" {
		n, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			writeProblem(w, http.StatusBadRequest, "since must be a non-negative integer")
			return
		}
		since = n
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"length":  s.ledger.Len(),
		"entries": s.ledger.List(since),
	})
}

func (s *Server) handleGet(w http.ResponseWriter, r *http.Request) {
	n, err := strconv.ParseUint(r.PathValue("seq"), 10, 64)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid seq")
		return
	}
	e, err := s.ledger.Get(n)
	if err != nil {
		writeProblem(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, e)
}

func (s *Server) handleVerify(w http.ResponseWriter, _ *http.Request) {
	r := s.ledger.Verify()
	status := http.StatusOK
	if !r.OK {
		status = http.StatusUnprocessableEntity
	}
	writeJSON(w, status, r)
}

// handleAnchor returns a timeline anchor for the audit chain.
//
// Lab default (no signer): JSON shape with length, tail_hash,
// and timestamp.
// Production (signer wired): same shape plus a base64-encoded
// `signed_payload` (DER of the Anchor SEQUENCE) and `signature`
// (DER ECDSA over SHA-256 of the signed_payload). Auditors verify
// the signature against the published audit-anchor public key.
//
// Empty chain returns length=0 with an all-zero tail hash — the
// same convention the chain itself uses for the first entry's
// prev_hash.
func (s *Server) handleAnchor(w http.ResponseWriter, r *http.Request) {
	length := s.ledger.Len()
	tail := make([]byte, sha256.Size)
	if length > 0 {
		e, err := s.ledger.Get(uint64(length))
		if err != nil {
			writeProblem(w, http.StatusInternalServerError, fmt.Sprintf("read tail: %v", err))
			return
		}
		tail = e.Hash
	}
	now := time.Now().UTC().Truncate(time.Second)

	resp := map[string]any{
		"length":    length,
		"tail_hash": anchor.HexHash(tail),
		"timestamp": now.Format(time.RFC3339),
	}

	if s.signer != nil {
		a := anchor.Anchor{
			Timestamp: now,
			Length:    int64(length),
			TailHash:  tail,
		}
		signed, sig, err := anchor.Sign(r.Context(), s.signer.Broker, s.signer.KeyID, a)
		if err != nil {
			s.logger.Error("audit anchor sign failed", slog.String("err", err.Error()))
			writeProblem(w, http.StatusInternalServerError, "sign failed")
			return
		}
		resp["signed_payload"] = signed
		resp["signature"] = sig
		resp["signature_alg"] = "ECDSA-SHA-256"
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ready":  true,
		"length": s.ledger.Len(),
	})
}

func readAllJSON(r *http.Request) (json.RawMessage, error) {
	dec := json.NewDecoder(r.Body)
	var raw json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	return raw, nil
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
