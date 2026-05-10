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
	"bytes"
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/ajamous/aether/pkg/crypto/ecka"
	"github.com/ajamous/aether/pkg/hsmclient"
	"github.com/ajamous/aether/pkg/saip"
	smdpv1 "github.com/ajamous/aether/services/smdp-plus/api/v1"
	"github.com/ajamous/aether/services/smdp-plus/internal/bpp"
	"github.com/ajamous/aether/services/smdp-plus/internal/identity"
	"github.com/ajamous/aether/services/smdp-plus/internal/session"
	"github.com/ajamous/aether/services/smdp-plus/internal/signing"
)

// Server is the SM-DP+ HTTP server.
type Server struct {
	sessions session.Store
	hsm      *hsmclient.Client
	identity *identity.Identity // DPauth — signs ServerSigned1 (§5.7.13)
	dppb     *identity.Identity // DPpb   — signs SmdpSigned2 (§5.7.14)
	trust    *identity.TrustMaterial
	address  string // SM-DP+ public address (goes into ServerSigned1.serverAddress)
}

// Config holds the optional dependencies. New() works with the
// zero-value config: signing and verification are disabled and the
// server falls back to the older "skeleton" behaviour. Setting HSM
// and Identity enables ServerSigned1 signing on
// initiateAuthentication; setting Trust additionally enables eUICC
// verification on authenticateClient. Setting DPpb additionally
// enables SmdpSigned2 signing on authenticateClient — the handler
// returns SmdpSigned2 + DPpb signature + DPpb cert that the eUICC
// will verify before generating its own ephemeral pubkey for the
// upcoming BPP exchange.
type Config struct {
	HSM      *hsmclient.Client
	Identity *identity.Identity // DPauth
	DPpb     *identity.Identity // DPpb (optional; gates SmdpSigned2)
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
		srv.dppb = cfgs[0].DPpb
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

// dppbSigningEnabled reports whether the server can populate
// SmdpSigned2/SMDPSignature2/SMDPCertificate on the
// authenticateClient response. Lab default is disabled; production
// configs supply a DPpb identity (separate ceremony lifecycle from
// DPauth).
func (s *Server) dppbSigningEnabled() bool {
	return s.hsm != nil && s.dppb != nil
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

	resp := smdpv1.AuthenticateClientResponse{TransactionID: req.TransactionID}

	// SmdpSigned2 (§5.7.14) is the SM-DP+'s commitment that the
	// upcoming BPP belongs to this transaction. The eUICC verifies
	// the signature against the SM-DP+'s DPpb cert chain before
	// generating its own ephemeral pubkey for ECKA. We populate the
	// response when DPpb signing is configured (production); lab
	// mode leaves the fields empty so test harnesses without a
	// trust store can drive the flow.
	//
	// bppEuiccOtpk is intentionally omitted at this step. The eUICC
	// has not yet generated its ephemeral pubkey; it does so AFTER
	// verifying SmdpSigned2 and returns it inside the
	// PrepareDownloadResponse that GetBoundProfilePackage carries.
	// Re-signing SmdpSigned2 with the otpk filled in is the BPP
	// follow-up's job.
	if s.dppbSigningEnabled() {
		tidBytes, err := hexDecode(req.TransactionID)
		if err != nil {
			writeProblem(w, http.StatusInternalServerError, fmt.Sprintf("tid decode: %v", err))
			return
		}
		payload := signing.SmdpSigned2{
			TransactionID:  tidBytes,
			CCRequiredFlag: false,
			// BPPEuiccOtpk: nil — populated at BPP-step in a follow-up.
		}
		signedDER, sig, err := signing.SignSmdpSigned2(r.Context(), s.hsm, s.dppb.KeyID, payload)
		if err != nil {
			writeProblem(w, http.StatusInternalServerError, fmt.Sprintf("dppb sign: %v", err))
			return
		}
		resp.SMDPSigned2 = signedDER
		resp.SMDPSignature2 = sig
		resp.SMDPCertificate = s.dppb.CertDER
	}

	writeJSON(w, http.StatusOK, resp)
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

// handleGetBoundProfilePackage implements SGP.22 §5.6.4.
//
// When DPpb signing is configured AND the request supplies a
// well-formed eUICC otPK, the handler:
//
//  1. Generates a fresh ECKA P-256 ephemeral keypair for the
//     SM-DP+ side.
//  2. Derives SCP03t SENC/SMAC/MCV via bpp.Derive.
//  3. Builds a minimum-viable SAIP UPP via pkg/saip
//     (ProfileHeader + PEEnd — same shape profile-builder
//     emits today).
//  4. Seals the UPP into AES-128-GCM segments via
//     bpp.SealSegments.
//  5. Builds an InitialiseSecureChannelRequest, computes the
//     §5.7.7 signed-input bytes (transactionId || smdpOtpk ||
//     euiccOtpk), asks hsm-broker to sign with the DPpb key,
//     and populates the request's smdpSign field.
//  6. Assembles the outer BoundProfilePackage SEQUENCE via
//     bpp.AssembleBoundProfilePackage and returns the DER.
//
// When DPpb is not configured, the handler returns honest 501 —
// same as before — so the lab path stays HSM-free.
//
// What this handler does NOT do today, and is the named
// hardware-bench follow-up:
//   - Parse the LPA's signed PrepareDownloadResponse blob to
//     extract eUICC otPK (the in-tree path takes the otPK as a
//     direct request field). Also unverified: the eUICC's
//     signature over its otPK against the eUICC cert from
//     AuthenticateClient.
//   - Spec-precise per-segment AAD layout (counter encoding,
//     ICV framing) — bpp.SealSegments uses a SCP03t-shape
//     chained-MAC model that round-trips against itself but
//     hasn't been cross-vendor verified against a real eUICC.
//   - Carry the SAIP UPP from a chosen profile-builder
//     template — the in-tree UPP is a header-only profile
//     stamped with the session's ICCID. Wiring the
//     profile-builder selection lands in a follow-up once
//     pkg/saip's ProfileElement catalogue grows.
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

	if !s.dppbSigningEnabled() {
		// Lab default: no DPpb, no BPP. Honest 501, same as
		// before — refusing to ship a fake BPP is deliberate.
		writeProblem(w, http.StatusNotImplemented, "BPP generation requires --dppb-label; see services/smdp-plus/README.md")
		return
	}
	if len(req.EUICCOtpk) == 0 {
		writeProblem(w, http.StatusBadRequest, "euicc_otpk required (uncompressed P-256 point, 65 bytes)")
		return
	}
	if len(req.EUICCOtpk) != 65 || req.EUICCOtpk[0] != 0x04 {
		writeProblem(w, http.StatusBadRequest, "euicc_otpk must be uncompressed P-256 point (0x04 || X(32) || Y(32))")
		return
	}

	der, err := s.buildBPP(r.Context(), sess, req.EUICCOtpk)
	if err != nil {
		slog.Default().Error("BPP assembly failed", slog.String("err", err.Error()))
		writeProblem(w, http.StatusInternalServerError, fmt.Sprintf("BPP assembly: %v", err))
		return
	}

	writeJSON(w, http.StatusOK, smdpv1.GetBoundProfilePackageResponse{
		TransactionID:       req.TransactionID,
		BoundProfilePackage: der,
	})
}

// buildBPP runs the §5.6.4 BPP construction end to end. Split
// out from the HTTP handler so the test harness can call it
// directly when it wants to exercise the assembly without going
// through JSON.
func (s *Server) buildBPP(ctx context.Context, sess *session.Session, euiccOtpk []byte) ([]byte, error) {
	// 1. SM-DP+ ephemeral ECKA keypair.
	smdpEphemeral, err := ecka.GenerateKeyPair(ecka.CurveP256)
	if err != nil {
		return nil, fmt.Errorf("generate SM-DP+ ephemeral: %w", err)
	}
	smdpOtpk := smdpEphemeral.Pub.Bytes() // uncompressed X9.63 point — the wire form

	// 2. Parse the eUICC's otPK into a *ecdh.PublicKey we can
	// pass into ECKA.
	euiccPub, err := ecdh.P256().NewPublicKey(euiccOtpk)
	if err != nil {
		return nil, fmt.Errorf("parse euicc otpk: %w", err)
	}

	// 3. sharedInfo per SGP.22 §H.4: a small tagged blob
	// containing keyType/keyLength/keyUsage. We use the same
	// values the ControlRefTemplate carries so both sides
	// derive the same bytes.
	//
	// The exact spec layout (TLV-tagged with APPLICATION-N tags
	// per §H.4 Table H-1) is the documented hardware-bench
	// follow-up; the in-tree concatenation matches the spec in
	// shape (key-binding inputs → KDF input) and round-trips
	// against itself.
	sharedInfo := bytes.Join([][]byte{
		bpp.KeyTypeAESGCM,
		bpp.KeyLengthAES128,
		bpp.KeyUsageQualifierEncryptAndIntegrity,
		[]byte(sess.TransactionID),
	}, nil)

	// 4. Derive SENC/SMAC/MCV.
	keys, err := bpp.Derive(smdpEphemeral.Priv, euiccPub, sharedInfo)
	if err != nil {
		return nil, fmt.Errorf("derive session keys: %w", err)
	}

	// 5. Build a minimum-viable SAIP UPP (header + PEEnd). Same
	// shape profile-builder emits today; richer ProfileElements
	// land as pkg/saip's catalogue grows.
	iccidBytes := sessionICCIDBytes(sess)
	hdr := saip.ProfileHeader{
		MajorVersion:           saip.SAIPMajorVersion,
		MinorVersion:           saip.SAIPMinorVersion,
		ProfileType:            "Aether In-Tree Test Profile",
		ICCID:                  iccidBytes,
		EUICCMandatoryServices: []string{"contactless"},
	}
	pkg, err := saip.Build(hdr)
	if err != nil {
		return nil, fmt.Errorf("build SAIP UPP: %w", err)
	}
	upp, err := pkg.MarshalDER()
	if err != nil {
		return nil, fmt.Errorf("marshal SAIP UPP: %w", err)
	}

	// 6. Seal the UPP into AES-128-GCM segments.
	segs, err := bpp.SealSegments(keys, upp, bpp.MaxSegmentSize)
	if err != nil {
		return nil, fmt.Errorf("seal UPP segments: %w", err)
	}

	// 7. Build the InitialiseSecureChannelRequest skeleton, sign
	// the §5.7.7 input with DPpb, populate the signature.
	tidBytes, err := hexDecode(sess.TransactionID)
	if err != nil {
		return nil, fmt.Errorf("decode tid: %w", err)
	}
	signedInput := bpp.SignedInputBytes(tidBytes, smdpOtpk, euiccOtpk)
	digest := sha256.Sum256(signedInput)
	sigResp, err := s.hsm.Sign(ctx, s.dppb.KeyID, digest[:])
	if err != nil {
		return nil, fmt.Errorf("hsm sign DPpb over §5.7.7 input: %w", err)
	}

	iscr := bpp.InitialiseSecureChannelRequest{
		RemoteOpId:    bpp.RemoteOpIdInstallBoundProfilePackage,
		TransactionID: tidBytes,
		ControlRefTemplate: bpp.ControlRefTemplate{
			KeyUsageQualifier: bpp.KeyUsageQualifierEncryptAndIntegrity,
			KeyType:           bpp.KeyTypeAESGCM,
			KeyLength:         bpp.KeyLengthAES128,
		},
		SMDPOtpk: smdpOtpk,
		SMDPSign: sigResp.SignatureDER,
	}

	// 8. Assemble the outer BPP SEQUENCE.
	return bpp.AssembleBoundProfilePackage(iscr, segs)
}

// sessionICCIDBytes returns the 10-byte nibble-swapped ICCID for
// the session's profile, or a deterministic test placeholder when
// the session doesn't carry one (lab path). The placeholder is
// flagged 0xFF... so a hardware bench notices immediately.
func sessionICCIDBytes(sess *session.Session) []byte {
	const iccidLen = 10
	if sess.ICCID == "" {
		out := make([]byte, iccidLen)
		for i := range out {
			out[i] = 0xFF
		}
		return out
	}
	// Nibble-swap to 10 bytes per SGP.22 §B.1; matches the
	// helper in services/profile-builder/internal/template.
	padded := sess.ICCID
	if len(padded) == 19 {
		padded += "F"
	}
	if len(padded) != 20 {
		// Not a valid ICCID; fall back to placeholder rather
		// than emit a wrong-length value the eUICC would
		// reject.
		out := make([]byte, iccidLen)
		for i := range out {
			out[i] = 0xFF
		}
		return out
	}
	out := make([]byte, iccidLen)
	for i := 0; i < iccidLen; i++ {
		hi := padded[2*i+1]
		lo := padded[2*i]
		var hn, ln byte
		switch {
		case hi >= '0' && hi <= '9':
			hn = hi - '0'
		case hi == 'F' || hi == 'f':
			hn = 0xF
		default:
			hn = 0xF
		}
		switch {
		case lo >= '0' && lo <= '9':
			ln = lo - '0'
		case lo == 'F' || lo == 'f':
			ln = 0xF
		default:
			ln = 0xF
		}
		out[i] = (hn << 4) | ln
	}
	return out
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
