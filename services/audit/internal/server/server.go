// Package server is the audit log HTTP transport.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/ajamous/aether/services/audit/internal/chain"
)

type Server struct {
	ledger chain.Backend
}

func New(l chain.Backend) *Server { return &Server{ledger: l} }

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/events", s.handleAppend)
	mux.HandleFunc("GET /v1/events", s.handleList)
	mux.HandleFunc("GET /v1/events/{seq}", s.handleGet)
	mux.HandleFunc("GET /v1/verify", s.handleVerify)
	mux.HandleFunc("GET /v1/health", s.handleHealth)
	return mux
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
