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
	trust    *identity.TrustMaterial
	address  string // SM-DP+ public address (goes into ServerSigned1.serverAddress)
}

// Config holds the optional dependencies. New() works with the
// zero-value config: signing and verification are disabled and the
// server falls back to the older "skeleton" behaviour. Setting HSM
// and Identity enables signing on initiateAuthentication; setting
// Trust additionally enables eUICC verification on authenticateClient.
type Config struct {
	HSM      *hsmclient.Client
	Identity *identity.Identity
	Trust    *identity.TrustMaterial
	Address  string
}

// New constructs a Server with the given session store and (optional)
// dependencies.
func New(s session.Store, cfgs ...Config) *Server {
	srv := &Server{sessions: s}
	if len(cfgs) > 0 {
		srv.hsm = cfgs[0].HSM
		srv.identity = cfgs[0].Identity
		srv.trust = cfgs[0].Trust
		srv.address = cfgs[0].Address
	}
	return srv
}

// signingEnabled reports whether the server can populate
// ServerSigned1/ServerSignature1/ServerCertificate.
func (s *Server) signingEnabled() bool {
	return s.hsm != nil && s.identity != nil && s.address != ""
}

// verificationEnabled reports whether the server can verify the
// eUICC's authenticateClient response.
func (s *Server) verificationEnabled() bool {
	return s.trust != nil && s.address != ""
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

// handleAuthenticateClient implements SGP.22 §5.6.2 / §5.7.5.
//
// When verification is enabled (Config.Trust + Config.Address set):
//
//  1. Find the open session for this transactionId. 404 on miss,
//     409 if not in `initiated` state.
//  2. Verify the eUICC's cert chain (euiccCert → eumCert → CI root)
//     against the certmgr-supplied trust store.
//  3. Verify euiccSignature1 against euiccCert's public key over
//     SHA-256(euiccSigned1).
//  4. Confirm euiccSigned1.serverAddress matches the configured
//     SM-DP+ address.
//  5. Confirm euiccSigned1.serverChallenge matches what we issued
//     in initiateAuthentication (replay defense).
//  6. Transition the session to `authenticated` and return 200.
//
// When verification is disabled the older skeleton path is preserved
// (state transition only, no cryptographic checks). This keeps unit
// tests that don't bring up a trust store working.
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

	if s.verificationEnabled() {
		if len(req.EuiccSigned1DER) == 0 || len(req.EuiccSignature1) == 0 ||
			len(req.EuiccCertDER) == 0 || len(req.EumCertDER) == 0 {
			writeProblem(w, http.StatusBadRequest,
				"euicc_signed1, euicc_signature1, euicc_certificate, eum_certificate all required")
			return
		}
		res, err := signing.VerifyEuiccAuthenticate(
			req.EuiccSigned1DER, req.EuiccSignature1,
			req.EuiccCertDER, req.EumCertDER,
			signing.VerifyOptions{
				Roots:         s.trust.Roots,
				Intermediates: s.trust.Intermediates,
				ServerAddress: s.address,
			},
		)
		if err != nil {
			writeProblem(w, http.StatusUnauthorized, fmt.Sprintf("eUICC authentication failed: %v", err))
			return
		}
		// Replay defense: serverChallenge in the eUICC's signed payload
		// must match the one we issued for this session.
		if !bytesEqual(res.EuiccSigned1.ServerChallenge, sess.ServerChallenge) {
			writeProblem(w, http.StatusUnauthorized,
				"euiccSigned1.serverChallenge does not match the value issued in initiateAuthentication")
			return
		}
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

// bytesEqual is a constant-time-ish equality check. We keep it
// non-cryptographic because the values here are public per spec
// (server- and eUICC-challenge are exchanged in the clear), but
// the helper lives close to its caller for clarity.
func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
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
