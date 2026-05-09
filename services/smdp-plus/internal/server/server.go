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

	"github.com/ajamous/aether/pkg/hsmclient"
	smdpv1 "github.com/ajamous/aether/services/smdp-plus/api/v1"
	"github.com/ajamous/aether/services/smdp-plus/internal/identity"
	"github.com/ajamous/aether/services/smdp-plus/internal/session"
	"github.com/ajamous/aether/services/smdp-plus/internal/signing"
)

// Server is the SM-DP+ HTTP server.
type Server struct {
	sessions session.Store
	hsm      *hsmclient.Client
	identity *identity.Identity
	address  string // SM-DP+ public address (goes into ServerSigned1.serverAddress)
}

// Config holds the optional dependencies. New() works with the
// zero-value config: signing is then disabled and ServerSigned1 fields
// in initiateAuthentication responses are left nil. Setting HSM and
// Identity enables signing.
type Config struct {
	HSM      *hsmclient.Client
	Identity *identity.Identity
	Address  string
}

// New constructs a Server with the given session store and (optional)
// signing dependencies.
func New(s session.Store, cfgs ...Config) *Server {
	srv := &Server{sessions: s}
	if len(cfgs) > 0 {
		srv.hsm = cfgs[0].HSM
		srv.identity = cfgs[0].Identity
		srv.address = cfgs[0].Address
	}
	return srv
}

// signingEnabled reports whether the server has all the pieces it
// needs to populate ServerSigned1/ServerSignature1/ServerCertificate.
func (s *Server) signingEnabled() bool {
	return s.hsm != nil && s.identity != nil && s.address != ""
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
// Allocates a transactionID, mints a 16-byte serverChallenge, stores
// the session, and (when signing is enabled) builds + signs
// ServerSigned1 per §5.7.13 using the DPauth key in the HSM broker.
// The ServerCertificate field carries the X.509 wrapper around the
// public half so the LPA-side test harness can verify the signature.
//
// SGP.22 mandates the eUICC challenge to be 16 bytes; we accept any
// non-empty value but the LPA's eUICC will reject anything else.
func (s *Server) handleInitiateAuthentication(w http.ResponseWriter, r *http.Request) {
	var req smdpv1.InitiateAuthenticationRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if len(req.EUICCChallenge) == 0 {
		writeProblem(w, http.StatusBadRequest, "euicc_challenge required")
		return
	}
	if len(req.EUICCChallenge) != 16 {
		writeProblem(w, http.StatusBadRequest, "euicc_challenge must be 16 bytes per SGP.22 §5.7.13")
		return
	}

	serverChallenge := make([]byte, 16)
	if _, err := rand.Read(serverChallenge); err != nil {
		writeProblem(w, http.StatusInternalServerError, fmt.Sprintf("rand: %v", err))
		return
	}
	tidHex := session.NewTransactionID()
	tidBytes, _ := hexDecode(tidHex)
	now := time.Now()
	sess := &session.Session{
		TransactionID:   tidHex,
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

	resp := smdpv1.InitiateAuthenticationResponse{TransactionID: tidHex}

	if s.signingEnabled() {
		payload := signing.ServerSigned1{
			TransactionID:   tidBytes,
			EUICCChallenge:  req.EUICCChallenge,
			ServerAddress:   s.address,
			ServerChallenge: serverChallenge,
		}
		signedDER, sig, err := signing.SignServerSigned1(r.Context(), s.hsm, s.identity.KeyID, payload)
		if err != nil {
			writeProblem(w, http.StatusInternalServerError, fmt.Sprintf("sign: %v", err))
			return
		}
		resp.ServerSigned1 = signedDER
		resp.ServerSignature1 = sig
		resp.ServerCertificate = s.identity.CertDER
	}

	writeJSON(w, http.StatusOK, resp)
}

// hexDecode is the package-level decoder we use to turn the
// session.NewTransactionID() hex string back into the bytes that go
// into the ASN.1 OCTET STRING. Wrapped so the import is local to
// this server package.
func hexDecode(s string) ([]byte, error) {
	out := make([]byte, len(s)/2)
	for i := 0; i < len(out); i++ {
		v, err := parseHexByte(s[2*i], s[2*i+1])
		if err != nil {
			return nil, err
		}
		out[i] = v
	}
	return out, nil
}

func parseHexByte(hi, lo byte) (byte, error) {
	h, err := nibble(hi)
	if err != nil {
		return 0, err
	}
	l, err := nibble(lo)
	if err != nil {
		return 0, err
	}
	return h<<4 | l, nil
}

func nibble(c byte) (byte, error) {
	switch {
	case '0' <= c && c <= '9':
		return c - '0', nil
	case 'a' <= c && c <= 'f':
		return c - 'a' + 10, nil
	case 'A' <= c && c <= 'F':
		return c - 'A' + 10, nil
	}
	return 0, fmt.Errorf("server: invalid hex nibble %q", c)
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
