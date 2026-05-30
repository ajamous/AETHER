package server

import (
	"bytes"
	"context"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ajamous/aether/pkg/hsmclient"
	"github.com/ajamous/aether/pkg/pbclient"
	"github.com/ajamous/aether/pkg/saip"
	smdpv1 "github.com/ajamous/aether/services/smdp-plus/api/v1"
	"github.com/ajamous/aether/services/smdp-plus/internal/bpp"
	"github.com/ajamous/aether/services/smdp-plus/internal/identity"
	"github.com/ajamous/aether/services/smdp-plus/internal/session"
	"github.com/ajamous/aether/services/smdp-plus/internal/signing"
)

// signPDR forges a signed PrepareDownloadResponseOk the way the
// eUICC would: marshals EuiccSigned2(txid, otpk), signs the DER with
// the labChain's leaf key, and wraps both into the outer SEQUENCE.
// Returns the wire blob and the otpk bytes for the test to assert
// against the decrypted UPP.
func (c *labChain) signPDR(t *testing.T, txid []byte, euiccEphemeral *ecdh.PrivateKey) (blob, otpk []byte) {
	t.Helper()
	otpk = euiccEphemeral.PublicKey().Bytes()
	signed := signing.EuiccSigned2{TransactionID: txid, EuiccOtpk: otpk}
	signedDER, err := signed.MarshalDER()
	if err != nil {
		t.Fatalf("EuiccSigned2 marshal: %v", err)
	}
	digest := sha256.Sum256(signedDER)
	r, s, err := ecdsa.Sign(rand.Reader, c.leafKey, digest[:])
	if err != nil {
		t.Fatalf("sign PDR: %v", err)
	}
	sigDER, _ := asn1.Marshal(struct{ R, S *big.Int }{r, s})
	blob, err = signing.MarshalPrepareDownloadResponseOk(signed, sigDER)
	if err != nil {
		t.Fatalf("wrap PDR: %v", err)
	}
	return blob, otpk
}

// fakeBroker stands up an HSM broker that signs with smdpKey and
// returns smdpKey's public point on generate-key-pair. Mirrors the
// inline broker in TestGetBoundProfilePackage_HappyPath.
func fakeBroker(t *testing.T) (*hsmclient.Client, func()) {
	t.Helper()
	smdpKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	ecdhPub, _ := smdpKey.PublicKey.ECDH()
	pubPoint := ecdhPub.Bytes()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/v1/generate-key-pair"):
			_ = json.NewEncoder(w).Encode(hsmclient.GenerateKeyPairResponse{
				Handle:    hsmclient.KeyHandle{ID: "key", Label: "key", Kind: hsmclient.KeyKindECDSA, Curve: hsmclient.CurveP256},
				PublicKey: pubPoint,
			})
		case strings.HasSuffix(r.URL.Path, "/v1/sign"):
			var req struct {
				Digest []byte `json:"digest"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			r2, s2, _ := ecdsa.Sign(rand.Reader, smdpKey, req.Digest)
			der, _ := asn1.Marshal(struct{ R, S *big.Int }{R: r2, S: s2})
			_ = json.NewEncoder(w).Encode(hsmclient.SignResponse{SignatureDER: der})
		default:
			http.NotFound(w, r)
		}
	}))
	return hsmclient.New(srv.URL), srv.Close
}

// credentialUPP builds a real credential-carrying SAIP UPP (the shape
// profile-builder emits): header + PE-USIM + PE-AKAParameter + PEEnd.
func credentialUPP(t *testing.T, imsi string, ki, opc []byte) []byte {
	t.Helper()
	pkg, err := saip.Build(saip.ProfileHeader{
		MajorVersion:           saip.SAIPMajorVersion,
		MinorVersion:           saip.SAIPMinorVersion,
		ProfileType:            "lab-mvno",
		ICCID:                  bytes.Repeat([]byte{0x12}, 10),
		EUICCMandatoryServices: []string{"usim"},
	})
	if err != nil {
		t.Fatalf("saip build: %v", err)
	}
	usimDER, err := saip.BuildUSIM(saip.PEUSIM{IMSI: imsi, MCC: "001", MNC: "01"})
	if err != nil {
		t.Fatalf("usim: %v", err)
	}
	akaDER, err := saip.BuildAKAParameter(saip.PEAKAParameter{AlgorithmID: saip.AKAAlgorithmMilenage, Ki: ki, OPc: opc})
	if err != nil {
		t.Fatalf("aka: %v", err)
	}
	if err := pkg.AppendRaw(usimDER); err != nil {
		t.Fatalf("append usim: %v", err)
	}
	if err := pkg.AppendRaw(akaDER); err != nil {
		t.Fatalf("append aka: %v", err)
	}
	der, err := pkg.MarshalDER()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return der
}

// TestPrepareThenBoundProfile_CarriesCredentialsEndToEnd is the
// vertical slice: prepare a profile (smdp-plus → profile-builder),
// drive the full ES9+ flow, then DECRYPT the returned BPP with the
// eUICC's ephemeral key and confirm the operator's IMSI/Ki/OPc made
// it all the way through seal → wire → open. This is as close to
// "the eUICC installed a working profile" as we get without hardware.
func TestPrepareThenBoundProfile_CarriesCredentialsEndToEnd(t *testing.T) {
	const (
		iccid = "8900000000000000007"
		imsi  = "001010000000007"
	)
	ki := bytes.Repeat([]byte{0x77}, 16)
	opc := bytes.Repeat([]byte{0x88}, 16)
	wantUPP := credentialUPP(t, imsi, ki, opc)

	// Fake profile-builder: returns the credential UPP for any build.
	var buildHits int
	pbSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/build") {
			buildHits++
			_ = json.NewEncoder(w).Encode(pbclient.BuildResponse{SAIP: wantUPP})
			return
		}
		http.NotFound(w, r)
	}))
	defer pbSrv.Close()

	chain := newLabChain(t)
	hc, brokerClose := fakeBroker(t)
	defer brokerClose()
	dpauth, _ := identity.EnsureLabIdentity(context.Background(), hc, "DPauth", "aether.local")
	dppb, _ := identity.EnsureLabIdentity(context.Background(), hc, "DPpb", "aether.local")
	roots := x509.NewCertPool()
	roots.AddCert(chain.rootCert)
	tm := &identity.TrustMaterial{Roots: roots, Intermediates: x509.NewCertPool()}

	smdpSrv := httptest.NewServer(New(
		session.NewMemoryStore(time.Minute),
		Config{
			HSM: hc, Identity: dpauth, DPpb: dppb, Trust: tm, Address: "aether.local",
			ProfileBuilder: pbclient.New(pbSrv.URL), DefaultTemplate: "lab-mvno",
		},
	).Routes())
	defer smdpSrv.Close()

	// 1. Prepare the profile (in-tree stand-in for ES2+ DownloadOrder).
	prepBody, _ := json.Marshal(smdpv1.PrepareProfileRequest{
		Subscriber: smdpv1.PrepareSubscriber{IMSI: imsi, ICCID: iccid, Ki: ki, OPc: opc},
	})
	resp, err := http.Post(smdpSrv.URL+"/v1/profiles/prepare", "application/json", bytes.NewReader(prepBody))
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		var prob map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&prob)
		resp.Body.Close()
		t.Fatalf("prepare status = %d, body = %v", resp.StatusCode, prob)
	}
	resp.Body.Close()
	if buildHits != 1 {
		t.Fatalf("profile-builder build called %d times, want 1", buildHits)
	}

	// 2. initiateAuthentication.
	initBody, _ := json.Marshal(smdpv1.InitiateAuthenticationRequest{
		EUICCChallenge: bytes.Repeat([]byte{0xAB}, 16),
		SMDPAddress:    "aether.local",
	})
	resp, err = http.Post(smdpSrv.URL+"/gsma/rsp2/es9plus/initiateAuthentication", "application/json", bytes.NewReader(initBody))
	if err != nil {
		t.Fatalf("initiate: %v", err)
	}
	var initOut smdpv1.InitiateAuthenticationResponse
	_ = json.NewDecoder(resp.Body).Decode(&initOut)
	resp.Body.Close()
	parsed, _ := signing.UnmarshalServerSigned1(initOut.ServerSigned1)
	tidBytes := parsed.TransactionID
	serverChallenge := parsed.ServerChallenge

	// 3. authenticateClient.
	signed, sig, leafDER, eumDER := chain.signAuthenticateResponse(t, tidBytes, "aether.local", serverChallenge)
	authBody, _ := json.Marshal(smdpv1.AuthenticateClientRequest{
		TransactionID:   hexEncode(tidBytes),
		EuiccSigned1DER: signed, EuiccSignature1: sig, EuiccCertDER: leafDER, EumCertDER: eumDER,
	})
	resp, err = http.Post(smdpSrv.URL+"/gsma/rsp2/es9plus/authenticateClient", "application/json", bytes.NewReader(authBody))
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("authenticate status = %d", resp.StatusCode)
	}
	resp.Body.Close()

	// 4. getBoundProfilePackage with the eUICC ephemeral pubkey and the
	// prepared profile's ICCID.
	euiccEphemeral, _ := ecdh.P256().GenerateKey(rand.Reader)
	euiccOtpk := euiccEphemeral.PublicKey().Bytes()
	bppBody, _ := json.Marshal(smdpv1.GetBoundProfilePackageRequest{
		TransactionID: hexEncode(tidBytes),
		EUICCOtpk:     euiccOtpk,
		ICCID:         iccid,
	})
	resp, err = http.Post(smdpSrv.URL+"/gsma/rsp2/es9plus/getBoundProfilePackage", "application/json", bytes.NewReader(bppBody))
	if err != nil {
		t.Fatalf("getBPP: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		var prob map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&prob)
		resp.Body.Close()
		t.Fatalf("getBPP status = %d, body = %v", resp.StatusCode, prob)
	}
	var out smdpv1.GetBoundProfilePackageResponse
	_ = json.NewDecoder(resp.Body).Decode(&out)
	resp.Body.Close()

	// 5. Decrypt the BPP as the eUICC would: parse the preamble for the
	// SM-DP+ ephemeral pubkey, derive the same SCP03t keys, open the
	// sealed segments.
	iscr, segments, err := bpp.DisassembleBoundProfilePackage(out.BoundProfilePackage)
	if err != nil {
		t.Fatalf("disassemble BPP: %v", err)
	}
	smdpPub, err := ecdh.P256().NewPublicKey(iscr.SMDPOtpk)
	if err != nil {
		t.Fatalf("parse smdp otpk: %v", err)
	}
	keys, err := bpp.Derive(euiccEphemeral, smdpPub, bpp.SharedInfo(hexEncode(tidBytes)))
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	recovered, err := bpp.OpenSegments(keys, segments)
	if err != nil {
		t.Fatalf("open segments: %v", err)
	}

	// 6. The recovered plaintext must be the exact UPP profile-builder
	// produced — proving the prepared profile (not the placeholder)
	// was sealed.
	if !bytes.Equal(recovered, wantUPP) {
		t.Fatalf("recovered UPP != prepared UPP\n got %x\nwant %x", recovered, wantUPP)
	}

	// 7. And it carries the operator's credentials.
	elems, err := saip.Decode(recovered)
	if err != nil {
		t.Fatalf("saip.Decode recovered: %v", err)
	}
	var gotUSIM bool
	var gotAKA bool
	for _, e := range elems {
		if u, ok := saip.DecodeUSIM(e); ok {
			gotUSIM = true
			if u.IMSI != imsi {
				t.Errorf("recovered IMSI = %q, want %q", u.IMSI, imsi)
			}
		}
		if a, ok := saip.DecodeAKAParameter(e); ok {
			gotAKA = true
			if !bytes.Equal(a.Ki, ki) || !bytes.Equal(a.OPc, opc) {
				t.Errorf("recovered Ki/OPc mismatch")
			}
		}
	}
	if !gotUSIM || !gotAKA {
		t.Errorf("recovered UPP missing credential elements: usim=%v aka=%v", gotUSIM, gotAKA)
	}
}

// TestPrepareThenBoundProfile_ResolvesByMatchingID drives the
// activation-code path: prepare returns a matchingId, the LPA carries
// it on authenticateClient (in-tree stand-in for ctxParams1), and
// getBoundProfilePackage resolves the prepared profile by matchingId
// alone — no ICCID on the BPP request. Decrypts the BPP and confirms
// the operator's credentials still round-trip.
func TestPrepareThenBoundProfile_ResolvesByMatchingID(t *testing.T) {
	const (
		iccid = "8900000000000000008"
		imsi  = "001010000000008"
	)
	ki := bytes.Repeat([]byte{0x55}, 16)
	opc := bytes.Repeat([]byte{0x66}, 16)
	wantUPP := credentialUPP(t, imsi, ki, opc)

	pbSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/build") {
			_ = json.NewEncoder(w).Encode(pbclient.BuildResponse{SAIP: wantUPP})
			return
		}
		http.NotFound(w, r)
	}))
	defer pbSrv.Close()

	chain := newLabChain(t)
	hc, brokerClose := fakeBroker(t)
	defer brokerClose()
	dpauth, _ := identity.EnsureLabIdentity(context.Background(), hc, "DPauth", "aether.local")
	dppb, _ := identity.EnsureLabIdentity(context.Background(), hc, "DPpb", "aether.local")
	roots := x509.NewCertPool()
	roots.AddCert(chain.rootCert)
	tm := &identity.TrustMaterial{Roots: roots, Intermediates: x509.NewCertPool()}

	smdpSrv := httptest.NewServer(New(
		session.NewMemoryStore(time.Minute),
		Config{
			HSM: hc, Identity: dpauth, DPpb: dppb, Trust: tm, Address: "aether.local",
			ProfileBuilder: pbclient.New(pbSrv.URL), DefaultTemplate: "lab-mvno",
		},
	).Routes())
	defer smdpSrv.Close()

	// 1. Prepare: capture the matchingId the SM-DP+ mints.
	prepBody, _ := json.Marshal(smdpv1.PrepareProfileRequest{
		Subscriber: smdpv1.PrepareSubscriber{IMSI: imsi, ICCID: iccid, Ki: ki, OPc: opc},
	})
	resp, err := http.Post(smdpSrv.URL+"/v1/profiles/prepare", "application/json", bytes.NewReader(prepBody))
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	var prepOut smdpv1.PrepareProfileResponse
	_ = json.NewDecoder(resp.Body).Decode(&prepOut)
	resp.Body.Close()
	if prepOut.MatchingID == "" {
		t.Fatal("prepare returned empty matching_id")
	}
	if !strings.HasPrefix(prepOut.ActivationCode, "LPA:1$aether.local$") {
		t.Errorf("activation_code = %q, want LPA:1$aether.local$<id>", prepOut.ActivationCode)
	}
	if !strings.HasSuffix(prepOut.ActivationCode, prepOut.MatchingID) {
		t.Errorf("activation_code does not carry matching_id: %q vs %q", prepOut.ActivationCode, prepOut.MatchingID)
	}

	// 2. initiateAuthentication.
	initBody, _ := json.Marshal(smdpv1.InitiateAuthenticationRequest{
		EUICCChallenge: bytes.Repeat([]byte{0xAB}, 16), SMDPAddress: "aether.local",
	})
	resp, _ = http.Post(smdpSrv.URL+"/gsma/rsp2/es9plus/initiateAuthentication", "application/json", bytes.NewReader(initBody))
	var initOut smdpv1.InitiateAuthenticationResponse
	_ = json.NewDecoder(resp.Body).Decode(&initOut)
	resp.Body.Close()
	parsed, _ := signing.UnmarshalServerSigned1(initOut.ServerSigned1)
	tidBytes := parsed.TransactionID

	// 3. authenticateClient — carry the matchingId. In real flow it
	// lives in ctxParams1; the in-tree path uses the request field.
	signed, sig, leafDER, eumDER := chain.signAuthenticateResponse(t, tidBytes, "aether.local", parsed.ServerChallenge)
	authBody, _ := json.Marshal(smdpv1.AuthenticateClientRequest{
		TransactionID:   hexEncode(tidBytes),
		EuiccSigned1DER: signed, EuiccSignature1: sig, EuiccCertDER: leafDER, EumCertDER: eumDER,
		MatchingID: prepOut.MatchingID,
	})
	resp, _ = http.Post(smdpSrv.URL+"/gsma/rsp2/es9plus/authenticateClient", "application/json", bytes.NewReader(authBody))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("authenticate status = %d", resp.StatusCode)
	}
	resp.Body.Close()

	// 4. getBoundProfilePackage — NO iccid on the request. The
	// session's matchingId (set above) must resolve the prepared
	// profile on its own.
	euiccEphemeral, _ := ecdh.P256().GenerateKey(rand.Reader)
	bppBody, _ := json.Marshal(smdpv1.GetBoundProfilePackageRequest{
		TransactionID: hexEncode(tidBytes),
		EUICCOtpk:     euiccEphemeral.PublicKey().Bytes(),
	})
	resp, err = http.Post(smdpSrv.URL+"/gsma/rsp2/es9plus/getBoundProfilePackage", "application/json", bytes.NewReader(bppBody))
	if err != nil {
		t.Fatalf("getBPP: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		var prob map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&prob)
		resp.Body.Close()
		t.Fatalf("getBPP status = %d, body = %v", resp.StatusCode, prob)
	}
	var out smdpv1.GetBoundProfilePackageResponse
	_ = json.NewDecoder(resp.Body).Decode(&out)
	resp.Body.Close()

	// 5. Decrypt and confirm the recovered UPP is the prepared one.
	iscr, segments, err := bpp.DisassembleBoundProfilePackage(out.BoundProfilePackage)
	if err != nil {
		t.Fatalf("disassemble: %v", err)
	}
	smdpPub, _ := ecdh.P256().NewPublicKey(iscr.SMDPOtpk)
	keys, _ := bpp.Derive(euiccEphemeral, smdpPub, bpp.SharedInfo(hexEncode(tidBytes)))
	recovered, err := bpp.OpenSegments(keys, segments)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if !bytes.Equal(recovered, wantUPP) {
		t.Fatal("recovered UPP differs from the prepared one — matchingId path did not resolve correctly")
	}
}

// TestPrepareProfile_RequiresProfileBuilder confirms the endpoint is
// an honest 501 when no profile-builder is configured (lab default).
func TestPrepareProfile_RequiresProfileBuilder(t *testing.T) {
	srv := httptest.NewServer(New(session.NewMemoryStore(time.Minute)).Routes())
	defer srv.Close()
	body, _ := json.Marshal(smdpv1.PrepareProfileRequest{
		Subscriber: smdpv1.PrepareSubscriber{ICCID: "8900000000000000001"},
	})
	resp, err := http.Post(srv.URL+"/v1/profiles/prepare", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotImplemented {
		t.Errorf("status = %d, want 501", resp.StatusCode)
	}
}

// TestBoundProfile_FallsBackToPlaceholderWithoutPreparedProfile
// confirms that when no profile is prepared for the ICCID, the BPP
// still builds (sealing the header-only placeholder) — preserving the
// pre-existing lab behaviour.
func TestBoundProfile_FallsBackToPlaceholderWithoutPreparedProfile(t *testing.T) {
	chain := newLabChain(t)
	hc, brokerClose := fakeBroker(t)
	defer brokerClose()
	dpauth, _ := identity.EnsureLabIdentity(context.Background(), hc, "DPauth", "aether.local")
	dppb, _ := identity.EnsureLabIdentity(context.Background(), hc, "DPpb", "aether.local")
	roots := x509.NewCertPool()
	roots.AddCert(chain.rootCert)
	tm := &identity.TrustMaterial{Roots: roots, Intermediates: x509.NewCertPool()}

	smdpSrv := httptest.NewServer(New(
		session.NewMemoryStore(time.Minute),
		Config{HSM: hc, Identity: dpauth, DPpb: dppb, Trust: tm, Address: "aether.local"},
	).Routes())
	defer smdpSrv.Close()

	initBody, _ := json.Marshal(smdpv1.InitiateAuthenticationRequest{
		EUICCChallenge: bytes.Repeat([]byte{0xAB}, 16), SMDPAddress: "aether.local",
	})
	resp, _ := http.Post(smdpSrv.URL+"/gsma/rsp2/es9plus/initiateAuthentication", "application/json", bytes.NewReader(initBody))
	var initOut smdpv1.InitiateAuthenticationResponse
	_ = json.NewDecoder(resp.Body).Decode(&initOut)
	resp.Body.Close()
	parsed, _ := signing.UnmarshalServerSigned1(initOut.ServerSigned1)
	tidBytes := parsed.TransactionID

	signed, sig, leafDER, eumDER := chain.signAuthenticateResponse(t, tidBytes, "aether.local", parsed.ServerChallenge)
	authBody, _ := json.Marshal(smdpv1.AuthenticateClientRequest{
		TransactionID: hexEncode(tidBytes), EuiccSigned1DER: signed, EuiccSignature1: sig, EuiccCertDER: leafDER, EumCertDER: eumDER,
	})
	resp, _ = http.Post(smdpSrv.URL+"/gsma/rsp2/es9plus/authenticateClient", "application/json", bytes.NewReader(authBody))
	resp.Body.Close()

	euiccEphemeral, _ := ecdh.P256().GenerateKey(rand.Reader)
	bppBody, _ := json.Marshal(smdpv1.GetBoundProfilePackageRequest{
		TransactionID: hexEncode(tidBytes), EUICCOtpk: euiccEphemeral.PublicKey().Bytes(),
		// No ICCID and no prepared profile → placeholder path.
	})
	resp, err := http.Post(smdpSrv.URL+"/gsma/rsp2/es9plus/getBoundProfilePackage", "application/json", bytes.NewReader(bppBody))
	if err != nil {
		t.Fatalf("getBPP: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("getBPP status = %d, want 200 (placeholder fallback)", resp.StatusCode)
	}
	var out smdpv1.GetBoundProfilePackageResponse
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if len(out.BoundProfilePackage) == 0 {
		t.Fatal("placeholder BPP empty")
	}
}

// TestPrepareThenBoundProfile_VerifiesSignedPDR drives the spec-
// faithful path: the LPA forwards a signed PrepareDownloadResponse
// instead of supplying the eUICC otPK directly. The SM-DP+ verifies
// it against the eUICC cert captured at authenticateClient, extracts
// the otPK, and seals the BPP. The test decrypts the BPP and
// confirms the credentials still round-trip.
func TestPrepareThenBoundProfile_VerifiesSignedPDR(t *testing.T) {
	const (
		iccid = "8900000000000000009"
		imsi  = "001010000000009"
	)
	ki := bytes.Repeat([]byte{0x33}, 16)
	opc := bytes.Repeat([]byte{0x44}, 16)
	wantUPP := credentialUPP(t, imsi, ki, opc)

	pbSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/build") {
			_ = json.NewEncoder(w).Encode(pbclient.BuildResponse{SAIP: wantUPP})
			return
		}
		http.NotFound(w, r)
	}))
	defer pbSrv.Close()

	chain := newLabChain(t)
	hc, brokerClose := fakeBroker(t)
	defer brokerClose()
	dpauth, _ := identity.EnsureLabIdentity(context.Background(), hc, "DPauth", "aether.local")
	dppb, _ := identity.EnsureLabIdentity(context.Background(), hc, "DPpb", "aether.local")
	roots := x509.NewCertPool()
	roots.AddCert(chain.rootCert)
	tm := &identity.TrustMaterial{Roots: roots, Intermediates: x509.NewCertPool()}

	smdpSrv := httptest.NewServer(New(
		session.NewMemoryStore(time.Minute),
		Config{
			HSM: hc, Identity: dpauth, DPpb: dppb, Trust: tm, Address: "aether.local",
			ProfileBuilder: pbclient.New(pbSrv.URL), DefaultTemplate: "lab-mvno",
		},
	).Routes())
	defer smdpSrv.Close()

	prepBody, _ := json.Marshal(smdpv1.PrepareProfileRequest{
		Subscriber: smdpv1.PrepareSubscriber{IMSI: imsi, ICCID: iccid, Ki: ki, OPc: opc},
	})
	resp, _ := http.Post(smdpSrv.URL+"/v1/profiles/prepare", "application/json", bytes.NewReader(prepBody))
	var prepOut smdpv1.PrepareProfileResponse
	_ = json.NewDecoder(resp.Body).Decode(&prepOut)
	resp.Body.Close()

	initBody, _ := json.Marshal(smdpv1.InitiateAuthenticationRequest{
		EUICCChallenge: bytes.Repeat([]byte{0xAB}, 16), SMDPAddress: "aether.local",
	})
	resp, _ = http.Post(smdpSrv.URL+"/gsma/rsp2/es9plus/initiateAuthentication", "application/json", bytes.NewReader(initBody))
	var initOut smdpv1.InitiateAuthenticationResponse
	_ = json.NewDecoder(resp.Body).Decode(&initOut)
	resp.Body.Close()
	parsed, _ := signing.UnmarshalServerSigned1(initOut.ServerSigned1)
	tidBytes := parsed.TransactionID

	signed, sig, leafDER, eumDER := chain.signAuthenticateResponse(t, tidBytes, "aether.local", parsed.ServerChallenge)
	authBody, _ := json.Marshal(smdpv1.AuthenticateClientRequest{
		TransactionID:   hexEncode(tidBytes),
		EuiccSigned1DER: signed, EuiccSignature1: sig, EuiccCertDER: leafDER, EumCertDER: eumDER,
		MatchingID: prepOut.MatchingID,
	})
	resp, _ = http.Post(smdpSrv.URL+"/gsma/rsp2/es9plus/authenticateClient", "application/json", bytes.NewReader(authBody))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("authenticate status = %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Build a signed PrepareDownloadResponse and send it instead of
	// the raw euicc_otpk.
	euiccEphemeral, _ := ecdh.P256().GenerateKey(rand.Reader)
	pdr, _ := chain.signPDR(t, tidBytes, euiccEphemeral)
	bppBody, _ := json.Marshal(smdpv1.GetBoundProfilePackageRequest{
		TransactionID:           hexEncode(tidBytes),
		PrepareDownloadResponse: pdr,
	})
	resp, err := http.Post(smdpSrv.URL+"/gsma/rsp2/es9plus/getBoundProfilePackage", "application/json", bytes.NewReader(bppBody))
	if err != nil {
		t.Fatalf("getBPP: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		var prob map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&prob)
		resp.Body.Close()
		t.Fatalf("getBPP status = %d, body = %v", resp.StatusCode, prob)
	}
	var out smdpv1.GetBoundProfilePackageResponse
	_ = json.NewDecoder(resp.Body).Decode(&out)
	resp.Body.Close()

	iscr, segments, err := bpp.DisassembleBoundProfilePackage(out.BoundProfilePackage)
	if err != nil {
		t.Fatalf("disassemble: %v", err)
	}
	smdpPub, _ := ecdh.P256().NewPublicKey(iscr.SMDPOtpk)
	keys, _ := bpp.Derive(euiccEphemeral, smdpPub, bpp.SharedInfo(hexEncode(tidBytes)))
	recovered, err := bpp.OpenSegments(keys, segments)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if !bytes.Equal(recovered, wantUPP) {
		t.Fatal("recovered UPP differs — signed PDR path did not deliver credentials")
	}
}

// TestPrepareThenBoundProfile_RejectsForgedPDR confirms a PDR signed
// by a different key (i.e. not the eUICC presented at authenticate
// Client) is rejected — the verifier binds the otpk to the session's
// eUICC identity.
func TestPrepareThenBoundProfile_RejectsForgedPDR(t *testing.T) {
	chain := newLabChain(t)
	hc, brokerClose := fakeBroker(t)
	defer brokerClose()
	dpauth, _ := identity.EnsureLabIdentity(context.Background(), hc, "DPauth", "aether.local")
	dppb, _ := identity.EnsureLabIdentity(context.Background(), hc, "DPpb", "aether.local")
	roots := x509.NewCertPool()
	roots.AddCert(chain.rootCert)
	tm := &identity.TrustMaterial{Roots: roots, Intermediates: x509.NewCertPool()}

	smdpSrv := httptest.NewServer(New(
		session.NewMemoryStore(time.Minute),
		Config{HSM: hc, Identity: dpauth, DPpb: dppb, Trust: tm, Address: "aether.local"},
	).Routes())
	defer smdpSrv.Close()

	initBody, _ := json.Marshal(smdpv1.InitiateAuthenticationRequest{
		EUICCChallenge: bytes.Repeat([]byte{0xAB}, 16), SMDPAddress: "aether.local",
	})
	resp, _ := http.Post(smdpSrv.URL+"/gsma/rsp2/es9plus/initiateAuthentication", "application/json", bytes.NewReader(initBody))
	var initOut smdpv1.InitiateAuthenticationResponse
	_ = json.NewDecoder(resp.Body).Decode(&initOut)
	resp.Body.Close()
	parsed, _ := signing.UnmarshalServerSigned1(initOut.ServerSigned1)
	tidBytes := parsed.TransactionID

	signed, sig, leafDER, eumDER := chain.signAuthenticateResponse(t, tidBytes, "aether.local", parsed.ServerChallenge)
	authBody, _ := json.Marshal(smdpv1.AuthenticateClientRequest{
		TransactionID:   hexEncode(tidBytes),
		EuiccSigned1DER: signed, EuiccSignature1: sig, EuiccCertDER: leafDER, EumCertDER: eumDER,
	})
	resp, _ = http.Post(smdpSrv.URL+"/gsma/rsp2/es9plus/authenticateClient", "application/json", bytes.NewReader(authBody))
	resp.Body.Close()

	// Sign PDR with a DIFFERENT eUICC chain — not the one in this
	// session.
	otherChain := newLabChain(t)
	euiccEphemeral, _ := ecdh.P256().GenerateKey(rand.Reader)
	pdr, _ := otherChain.signPDR(t, tidBytes, euiccEphemeral)
	bppBody, _ := json.Marshal(smdpv1.GetBoundProfilePackageRequest{
		TransactionID:           hexEncode(tidBytes),
		PrepareDownloadResponse: pdr,
	})
	resp, err := http.Post(smdpSrv.URL+"/gsma/rsp2/es9plus/getBoundProfilePackage", "application/json", bytes.NewReader(bppBody))
	if err != nil {
		t.Fatalf("getBPP: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 (forged PDR signature)", resp.StatusCode)
	}
}

// TestAuthenticateClient_OuterSequenceShape drives authenticateClient
// with the spec-faithful AuthenticateServerResponse outer SEQUENCE
// (§5.7.5) instead of the four explicit JSON fields, and confirms
// verification + matchingId/cert capture all still work.
func TestAuthenticateClient_OuterSequenceShape(t *testing.T) {
	chain := newLabChain(t)
	hc, brokerClose := fakeBroker(t)
	defer brokerClose()
	dpauth, _ := identity.EnsureLabIdentity(context.Background(), hc, "DPauth", "aether.local")
	dppb, _ := identity.EnsureLabIdentity(context.Background(), hc, "DPpb", "aether.local")
	roots := x509.NewCertPool()
	roots.AddCert(chain.rootCert)
	tm := &identity.TrustMaterial{Roots: roots, Intermediates: x509.NewCertPool()}

	smdpSrv := httptest.NewServer(New(
		session.NewMemoryStore(time.Minute),
		Config{HSM: hc, Identity: dpauth, DPpb: dppb, Trust: tm, Address: "aether.local"},
	).Routes())
	defer smdpSrv.Close()

	initBody, _ := json.Marshal(smdpv1.InitiateAuthenticationRequest{
		EUICCChallenge: bytes.Repeat([]byte{0xAB}, 16), SMDPAddress: "aether.local",
	})
	resp, _ := http.Post(smdpSrv.URL+"/gsma/rsp2/es9plus/initiateAuthentication", "application/json", bytes.NewReader(initBody))
	var initOut smdpv1.InitiateAuthenticationResponse
	_ = json.NewDecoder(resp.Body).Decode(&initOut)
	resp.Body.Close()
	parsed, _ := signing.UnmarshalServerSigned1(initOut.ServerSigned1)
	tidBytes := parsed.TransactionID

	// Build the four pieces, then wrap them in the outer SEQUENCE
	// instead of sending them as separate JSON fields.
	signed, sig, leafDER, eumDER := chain.signAuthenticateResponse(t, tidBytes, "aether.local", parsed.ServerChallenge)
	outer, err := signing.MarshalAuthenticateResponseOk(signed, sig, leafDER, eumDER)
	if err != nil {
		t.Fatalf("marshal outer: %v", err)
	}
	authBody, _ := json.Marshal(smdpv1.AuthenticateClientRequest{
		TransactionID:              hexEncode(tidBytes),
		AuthenticateServerResponse: outer,
	})
	resp, err = http.Post(smdpSrv.URL+"/gsma/rsp2/es9plus/authenticateClient", "application/json", bytes.NewReader(authBody))
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var prob map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&prob)
		t.Fatalf("authenticate via outer SEQUENCE failed: status=%d body=%v", resp.StatusCode, prob)
	}
}

// TestAuthenticateClient_OuterSequenceRejectedWhenMalformed confirms
// that a garbage authenticate_server_response yields 400, not 500.
func TestAuthenticateClient_OuterSequenceRejectedWhenMalformed(t *testing.T) {
	chain := newLabChain(t)
	hc, brokerClose := fakeBroker(t)
	defer brokerClose()
	dpauth, _ := identity.EnsureLabIdentity(context.Background(), hc, "DPauth", "aether.local")
	roots := x509.NewCertPool()
	roots.AddCert(chain.rootCert)
	tm := &identity.TrustMaterial{Roots: roots, Intermediates: x509.NewCertPool()}

	smdpSrv := httptest.NewServer(New(
		session.NewMemoryStore(time.Minute),
		Config{HSM: hc, Identity: dpauth, Trust: tm, Address: "aether.local"},
	).Routes())
	defer smdpSrv.Close()

	initBody, _ := json.Marshal(smdpv1.InitiateAuthenticationRequest{
		EUICCChallenge: bytes.Repeat([]byte{0xAB}, 16), SMDPAddress: "aether.local",
	})
	resp, _ := http.Post(smdpSrv.URL+"/gsma/rsp2/es9plus/initiateAuthentication", "application/json", bytes.NewReader(initBody))
	var initOut smdpv1.InitiateAuthenticationResponse
	_ = json.NewDecoder(resp.Body).Decode(&initOut)
	resp.Body.Close()
	parsed, _ := signing.UnmarshalServerSigned1(initOut.ServerSigned1)

	authBody, _ := json.Marshal(smdpv1.AuthenticateClientRequest{
		TransactionID:              hexEncode(parsed.TransactionID),
		AuthenticateServerResponse: []byte{0x00, 0x01, 0x02},
	})
	resp, err := http.Post(smdpSrv.URL+"/gsma/rsp2/es9plus/authenticateClient", "application/json", bytes.NewReader(authBody))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 on malformed outer SEQUENCE", resp.StatusCode)
	}
}
