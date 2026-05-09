// Package server is the profile-builder HTTP transport.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/ajamous/aether/services/profile-builder/internal/template"
)

type Server struct {
	loader *template.Loader
}

func New(l *template.Loader) *Server { return &Server{loader: l} }

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/health", s.handleHealth)
	mux.HandleFunc("GET /v1/templates", s.handleListTemplates)
	mux.HandleFunc("GET /v1/templates/{name}", s.handleGetTemplate)
	mux.HandleFunc("POST /v1/templates/{name}/build", s.handleBuild)
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

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ready": true})
}

func (s *Server) handleListTemplates(w http.ResponseWriter, _ *http.Request) {
	names, err := s.loader.List()
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"templates": names})
}

func (s *Server) handleGetTemplate(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	p, err := s.loader.Load(name)
	if err != nil {
		writeProblem(w, http.StatusNotFound, err.Error())
		return
	}
	if err := p.Validate(); err != nil {
		writeProblem(w, http.StatusInternalServerError, fmt.Sprintf("template invalid on disk: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleBuild(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	p, err := s.loader.Load(name)
	if err != nil {
		writeProblem(w, http.StatusNotFound, err.Error())
		return
	}
	var sub template.SubscriberData
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&sub); err != nil {
		writeProblem(w, http.StatusBadRequest, fmt.Sprintf("invalid JSON: %v", err))
		return
	}
	upp, err := template.BuildUPP(p, &sub)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, upp)
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
