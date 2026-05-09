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
	"log/slog"
	"net/http"
	"sync"
	"time"

	smdsv1 "github.com/ajamous/aether/services/smds/api/v1"
	"github.com/ajamous/aether/services/smds/internal/events"
	"github.com/ajamous/aether/services/smds/internal/signing"

	"github.com/ajamous/aether/pkg/hsmclient"
)

// session is the LPA-side bookkeeping between authenticateClient and
// getEvents.
type session struct {
	tid       string
	eid       smdsv1.EID
	createdAt time.Time

	// signedPayload + signature are populated when the SM-DS is
	// configured with HSM signing (Config.Signer non-nil). Stored on
	// the session so subsequent handler logic and tests can inspect
	// what was returned to the LPA.
	signedPayload []byte
	signature     []byte
}

// Signer holds everything the AuthenticateClient handler needs to
// produce a SGP.22 §5.5.4 ServerSigned1 payload + ECDSA signature.
//
// All three fields must be set to enable signing. ServerAddress is
// the public-facing SM-DS hostname the LPA will see in the signed
// payload (and which it cross-checks against the SM-DS certificate's
// SAN). KeyID is the broker-side identifier for the SM-DS auth key.
//
// Signing is OFF by default: lab deployments don't need it, and
// requiring an HSM at startup would block `make lab-up`. Production
// deployments wire this in via main flags.
type Signer struct {
	Broker        *hsmclient.Client
	KeyID         string
	ServerAddress string
}

// Server adapts events.Store over HTTP.
type Server struct {
	store  events.Store
	signer *Signer
	logger *slog.Logger

	mu       sync.Mutex
	sessions map[string]*session
}

// Config bundles the optional knobs. Pass nil for lab defaults.
type Config struct {
	Signer *Signer
	Logger *slog.Logger
}

func New(s events.Store, cfgs ...Config) *Server {
	srv := &Server{
		store:    s,
		sessions: make(map[string]*session),
		logger:   slog.Default(),
	}
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

// SigningEnabled reports whether the SM-DS will return a signed
// ServerSigned1 payload from AuthenticateClient. Used by tests and
// the /v1/health response.
func (s *Server) SigningEnabled() bool { return s.signer != nil }

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
// Allocates a transactionID, captures the LPA's EID for later
// GetEvents calls, and (when configured with an HSM signer) returns
// a DER-encoded serverSigned1 + ECDSA-SHA-256 signature so the LPA
// can verify the SM-DS's response against its identity certificate.
//
// When the signer is nil (lab default), the handler returns the
// transactionID alone — same shape as before HSM wiring landed.
// This keeps `make lab-up` HSM-free.
//
// SGP.22 §5.5.4 requires the eUICC challenge to be exactly 16
// octets; we enforce that here so misbehaving LPAs fail loudly
// instead of producing a bogus signature.
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
	if s.signer != nil && len(req.EUICCChallenge) != 16 {
		writeProblem(w, http.StatusBadRequest, fmt.Sprintf("euicc_challenge must be 16 bytes when signing is enabled, got %d", len(req.EUICCChallenge)))
		return
	}

	tidBytes := newRandom(16)
	tid := hex.EncodeToString(tidBytes)

	resp := smdsv1.AuthenticateClientResponse{TransactionID: tid}
	sess := &session{tid: tid, eid: req.EID, createdAt: time.Now()}

	if s.signer != nil {
		serverChallenge := newRandom(16)
		payload := signing.ServerSigned1{
			TransactionID:   tidBytes,
			EUICCChallenge:  req.EUICCChallenge,
			ServerAddress:   s.signer.ServerAddress,
			ServerChallenge: serverChallenge,
		}
		signed, sig, err := signing.SignServerSigned1(r.Context(), s.signer.Broker, s.signer.KeyID, payload)
		if err != nil {
			s.logger.Error("smds signServerSigned1 failed", slog.String("err", err.Error()))
			writeProblem(w, http.StatusInternalServerError, "sign failed")
			return
		}
		resp.ServerSigned1 = signed
		resp.ServerSignature1 = sig
		sess.signedPayload = signed
		sess.signature = sig
	}

	s.mu.Lock()
	s.sessions[tid] = sess
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, resp)
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
	return hex.EncodeToString(newRandom(16))
}

// newRandom returns n bytes of cryptographic randomness. Panics on
// rand failure — that's not a recoverable error in the request path.
func newRandom(n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return b
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
