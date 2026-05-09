// Package server is the SM-DS HTTP transport.
//
// Two endpoint groups under the SGP.22-defined paths:
//
//	POST /gsma/rsp2/es12/registerEvent     SM-DP+ → SM-DS  (§5.5.1)
//	POST /gsma/rsp2/es12/deleteEvent       SM-DP+ → SM-DS  (§5.5.2)
//	POST /gsma/rsp2/es11/authenticateClient LPA   → SM-DS  (§5.5.4)
//	POST /gsma/rsp2/es11/getEvents         LPA   → SM-DS  (§5.5.3)
//
// Plus an admin /v1/events endpoint for the operator UI to browse
// registered events.
package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	smdsv1 "github.com/ajamous/aether/services/smds/api/v1"
	"github.com/ajamous/aether/services/smds/internal/events"
)

// session is the LPA-side bookkeeping between authenticateClient and
// getEvents.
type session struct {
	tid       string
	eid       smdsv1.EID
	createdAt time.Time
}

// Server adapts events.Store over HTTP.
type Server struct {
	store events.Store

	mu       sync.Mutex
	sessions map[string]*session
}

func New(s events.Store) *Server {
	return &Server{store: s, sessions: make(map[string]*session)}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	// ES12 (SM-DP+ side)
	mux.HandleFunc("POST /gsma/rsp2/es12/registerEvent", s.handleRegisterEvent)
	mux.HandleFunc("POST /gsma/rsp2/es12/deleteEvent", s.handleDeleteEvent)
	// ES11 (LPA side)
	mux.HandleFunc("POST /gsma/rsp2/es11/authenticateClient", s.handleAuthenticateClient)
	mux.HandleFunc("POST /gsma/rsp2/es11/getEvents", s.handleGetEvents)
	// Admin
	mux.HandleFunc("GET /v1/health", s.handleHealth)
	mux.HandleFunc("GET /v1/events", s.handleAdminListEvents)
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

// --- ES12 handlers --------------------------------------------------------

// handleRegisterEvent implements SGP.22 §5.5.1.
func (s *Server) handleRegisterEvent(w http.ResponseWriter, r *http.Request) {
	var req smdsv1.RegisterEventRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.EID == "" {
		writeProblem(w, http.StatusBadRequest, "eid required")
		return
	}
	if req.RSPServerAddress == "" {
		writeProblem(w, http.StatusBadRequest, "rsp_server_address required")
		return
	}
	if req.EventID == "" {
		req.EventID = newToken()
	}
	if err := s.store.Register(&events.Stored{
		EID:        req.EID,
		EventID:    req.EventID,
		RSPAddress: req.RSPServerAddress,
		Forwarding: req.ForwardingIndicator,
	}); err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, smdsv1.RegisterEventResponse{EventID: req.EventID})
}

// handleDeleteEvent implements SGP.22 §5.5.2.
func (s *Server) handleDeleteEvent(w http.ResponseWriter, r *http.Request) {
	var req smdsv1.DeleteEventRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.EID == "" || req.EventID == "" {
		writeProblem(w, http.StatusBadRequest, "eid and event_id required")
		return
	}
	if err := s.store.Delete(req.EID, req.EventID); err != nil {
		if errors.Is(err, events.ErrEventNotFound) {
			writeProblem(w, http.StatusNotFound, err.Error())
			return
		}
		writeProblem(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, smdsv1.DeleteEventResponse{})
}

// --- ES11 handlers --------------------------------------------------------

// handleAuthenticateClient implements SGP.22 §5.5.4.
//
// The skeleton allocates a transactionID, captures the LPA's EID for
// later GetEvents calls, and returns empty serverSigned1/Signature
// fields. Signing those requires hsm-broker SoftHSM Sign + the SGP.22
// ASN.1 codec — same dependency chain as smdp-plus's
// initiateAuthentication.
func (s *Server) handleAuthenticateClient(w http.ResponseWriter, r *http.Request) {
	var req smdsv1.AuthenticateClientRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.EID == "" {
		writeProblem(w, http.StatusBadRequest, "eid required")
		return
	}
	if len(req.EUICCChallenge) == 0 {
		writeProblem(w, http.StatusBadRequest, "euicc_challenge required")
		return
	}
	tid := newToken()
	s.mu.Lock()
	s.sessions[tid] = &session{tid: tid, eid: req.EID, createdAt: time.Now()}
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, smdsv1.AuthenticateClientResponse{
		TransactionID: tid,
	})
}

// handleGetEvents implements SGP.22 §5.5.3.
func (s *Server) handleGetEvents(w http.ResponseWriter, r *http.Request) {
	var req smdsv1.GetEventsRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	s.mu.Lock()
	sess, ok := s.sessions[req.TransactionID]
	s.mu.Unlock()
	if !ok {
		writeProblem(w, http.StatusNotFound, "unknown transaction_id")
		return
	}
	stored := s.store.ListForEID(sess.eid)
	out := make([]smdsv1.Event, 0, len(stored))
	for _, e := range stored {
		out = append(out, smdsv1.Event{
			EventID:          e.EventID,
			RSPServerAddress: e.RSPAddress,
		})
	}
	writeJSON(w, http.StatusOK, smdsv1.GetEventsResponse{
		TransactionID: req.TransactionID,
		Events:        out,
	})
}

// --- admin handlers -------------------------------------------------------

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ready": true})
}

func (s *Server) handleAdminListEvents(w http.ResponseWriter, _ *http.Request) {
	all := s.store.All()
	writeJSON(w, http.StatusOK, map[string]any{
		"length": len(all),
		"events": all,
	})
}

// --- helpers --------------------------------------------------------------

func newToken() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}

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
