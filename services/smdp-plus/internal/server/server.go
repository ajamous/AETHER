// Package server implements the SM-DP+ HTTP transport: ES9+ on the
// public side, plus a small admin surface for health and metrics.
//
// The four ES9+ endpoints follow the SGP.22 §5.6 message shapes. The
// JSON wire format is for development convenience; production
// deployments will negotiate the SGP.22 application protocol where
// the LPA expects raw ASN.1 over HTTP — that switch lands in the same
// PR that vendors the Annex B ASN.1 modules and the BPP codec.
package server

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	smdpv1 "github.com/ajamous/aether/services/smdp-plus/api/v1"
	"github.com/ajamous/aether/services/smdp-plus/internal/session"
)

// Server is the SM-DP+ HTTP server.
type Server struct {
	sessions session.Store
}

// New constructs a Server with the given session store.
func New(s session.Store) *Server {
	return &Server{sessions: s}
}

// Routes returns an http.Handler with all endpoints mounted.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /gsma/rsp2/es9plus/initiateAuthentication", s.handleInitiateAuthentication)
	mux.HandleFunc("POST /gsma/rsp2/es9plus/authenticateClient", s.handleAuthenticateClient)
	mux.HandleFunc("POST /gsma/rsp2/es9plus/getBoundProfilePackage", s.handleGetBoundProfilePackage)
	mux.HandleFunc("POST /gsma/rsp2/es9plus/handleNotification", s.handleHandleNotification)
	mux.HandleFunc("GET /v1/health", s.handleHealth)
	return mux
}

// ListenAndServeTLS runs the server on addr until ctx is cancelled.
// If certFile and keyFile are empty, runs plain HTTP (lab only).
func (s *Server) ListenAndServeTLS(ctx context.Context, addr, certFile, keyFile string) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           s.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		if certFile != "" && keyFile != "" {
			errCh <- srv.ListenAndServeTLS(certFile, keyFile)
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

// --- ES9+ handlers --------------------------------------------------------

// handleInitiateAuthentication implements SGP.22 §5.6.1.
//
// Today we accept the LPA's request, mint a serverChallenge, allocate a
// transactionID, and store the session. We do NOT yet sign serverSigned1
// or attach a real SM-DP+ certificate — those land alongside hsm-broker
// SoftHSM Sign and the SAIP / SGP.22 ASN.1 codec.
func (s *Server) handleInitiateAuthentication(w http.ResponseWriter, r *http.Request) {
	var req smdpv1.InitiateAuthenticationRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if len(req.EUICCChallenge) == 0 {
		writeProblem(w, http.StatusBadRequest, "euicc_challenge required")
		return
	}

	serverChallenge := make([]byte, 16)
	if _, err := rand.Read(serverChallenge); err != nil {
		writeProblem(w, http.StatusInternalServerError, fmt.Sprintf("rand: %v", err))
		return
	}
	tid := session.NewTransactionID()
	now := time.Now()
	sess := &session.Session{
		TransactionID:   tid,
		State:           session.StateInitiated,
		CreatedAt:       now,
		UpdatedAt:       now,
		EUICCChallenge:  req.EUICCChallenge,
		ServerChallenge: serverChallenge,
	}
	if err := s.sessions.Create(sess); err != nil {
		writeProblem(w, http.StatusInternalServerError, fmt.Sprintf("session: %v", err))
		return
	}

	writeJSON(w, http.StatusOK, smdpv1.InitiateAuthenticationResponse{
		TransactionID:   tid,
		ServerSigned1:   nil, // populated when signing pipeline lands
		ServerSignature1: nil,
		EuiccCiPKIDToBeUsed: nil,
		ServerCertificate: nil,
	})
}

// handleAuthenticateClient implements SGP.22 §5.6.3 (skeleton).
func (s *Server) handleAuthenticateClient(w http.ResponseWriter, r *http.Request) {
	var req smdpv1.AuthenticateClientRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	sess, err := s.sessions.Get(req.TransactionID)
	if err != nil {
		writeProblem(w, http.StatusNotFound, "unknown transaction_id")
		return
	}
	if sess.State != session.StateInitiated {
		writeProblem(w, http.StatusConflict, fmt.Sprintf("session in state %q, expected initiated", sess.State))
		return
	}
	sess.State = session.StateAuthenticated
	if err := s.sessions.Update(sess); err != nil {
		writeProblem(w, http.StatusInternalServerError, fmt.Sprintf("update: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, smdpv1.AuthenticateClientResponse{
		TransactionID: req.TransactionID,
	})
}

// handleGetBoundProfilePackage implements SGP.22 §5.6.4 (skeleton).
func (s *Server) handleGetBoundProfilePackage(w http.ResponseWriter, r *http.Request) {
	var req smdpv1.GetBoundProfilePackageRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	sess, err := s.sessions.Get(req.TransactionID)
	if err != nil {
		writeProblem(w, http.StatusNotFound, "unknown transaction_id")
		return
	}
	if sess.State != session.StateAuthenticated {
		writeProblem(w, http.StatusConflict, fmt.Sprintf("session in state %q, expected authenticated", sess.State))
		return
	}

	// BPP generation lands when SAIP codec + spec ASN.1 modules are in
	// the tree. Refusing to ship a fake BPP is deliberate — see README.
	writeProblem(w, http.StatusNotImplemented, "BPP generation pending SAIP codec; see services/smdp-plus/README.md")
}

// handleHandleNotification implements SGP.22 §5.6.5 (skeleton).
func (s *Server) handleHandleNotification(w http.ResponseWriter, r *http.Request) {
	var req smdpv1.HandleNotificationRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if len(req.PendingNotification) == 0 {
		writeProblem(w, http.StatusBadRequest, "pending_notification required")
		return
	}
	writeJSON(w, http.StatusOK, smdpv1.HandleNotificationResponse{})
}

// --- admin handlers -------------------------------------------------------

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ready": true,
	})
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
