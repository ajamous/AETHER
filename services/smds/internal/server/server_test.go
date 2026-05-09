package server

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/asn1"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ajamous/aether/pkg/hsmclient"
	smdsv1 "github.com/ajamous/aether/services/smds/api/v1"
	"github.com/ajamous/aether/services/smds/internal/events"
	"github.com/ajamous/aether/services/smds/internal/signing"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(New(events.NewMemoryStore()).Routes())
	t.Cleanup(srv.Close)
	return srv
}

func postJSON(t *testing.T, url string, body any, dst any) int {
	t.Helper()
	buf, _ := json.Marshal(body)
	resp, err := http.Post(url, "application/json", bytes.NewReader(buf))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if dst != nil && resp.StatusCode == http.StatusOK {
		_ = json.NewDecoder(resp.Body).Decode(dst)
	}
	return resp.StatusCode
}

func TestSMDS_Health(t *testing.T) {
	srv := newTestServer(t)
	resp, err := http.Get(srv.URL + "/v1/health")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func TestSMDS_FullDiscoveryFlow(t *testing.T) {
	srv := newTestServer(t)
	eid := smdsv1.EID("89049032123451234512345678901234")

	// SM-DP+ side: register a pending event for the LPA's EID.
	var regResp smdsv1.RegisterEventResponse
	if status := postJSON(t, srv.URL+"/gsma/rsp2/es12/registerEvent",
		smdsv1.RegisterEventRequest{
			EID:              eid,
			RSPServerAddress: "smdp.example",
			EventID:          "evt-1",
		}, &regResp); status != http.StatusOK {
		t.Fatalf("register status = %d", status)
	}
	if regResp.EventID != "evt-1" {
		t.Fatalf("event_id = %q", regResp.EventID)
	}

	// LPA side: authenticate, then poll.
	var authResp smdsv1.AuthenticateClientResponse
	if status := postJSON(t, srv.URL+"/gsma/rsp2/es11/authenticateClient",
		smdsv1.AuthenticateClientRequest{
			EID:            eid,
			EUICCChallenge: []byte("challenge"),
		}, &authResp); status != http.StatusOK {
		t.Fatalf("auth status = %d", status)
	}
	if authResp.TransactionID == "" {
		t.Fatal("expected non-empty transaction_id")
	}

	var getResp smdsv1.GetEventsResponse
	if status := postJSON(t, srv.URL+"/gsma/rsp2/es11/getEvents",
		smdsv1.GetEventsRequest{TransactionID: authResp.TransactionID}, &getResp); status != http.StatusOK {
		t.Fatalf("getEvents status = %d", status)
	}
	if len(getResp.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(getResp.Events))
	}
	if getResp.Events[0].RSPServerAddress != "smdp.example" {
		t.Fatalf("rsp_server_address = %q", getResp.Events[0].RSPServerAddress)
	}

	// SM-DP+ deletes the event after delivery.
	if status := postJSON(t, srv.URL+"/gsma/rsp2/es12/deleteEvent",
		smdsv1.DeleteEventRequest{EID: eid, EventID: "evt-1"}, nil); status != http.StatusOK {
		t.Fatalf("delete status = %d", status)
	}

	// Polling again returns empty.
	getResp = smdsv1.GetEventsResponse{}
	if status := postJSON(t, srv.URL+"/gsma/rsp2/es11/getEvents",
		smdsv1.GetEventsRequest{TransactionID: authResp.TransactionID}, &getResp); status != http.StatusOK {
		t.Fatalf("getEvents-2 status = %d", status)
	}
	if len(getResp.Events) != 0 {
		t.Fatalf("expected 0 events after delete, got %d", len(getResp.Events))
	}
}

func TestSMDS_RegisterRejectsMissingFields(t *testing.T) {
	srv := newTestServer(t)
	if status := postJSON(t, srv.URL+"/gsma/rsp2/es12/registerEvent",
		smdsv1.RegisterEventRequest{}, nil); status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
}

func TestSMDS_DeleteUnknownReturns404(t *testing.T) {
	srv := newTestServer(t)
	if status := postJSON(t, srv.URL+"/gsma/rsp2/es12/deleteEvent",
		smdsv1.DeleteEventRequest{EID: "x", EventID: "y"}, nil); status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", status)
	}
}

func TestSMDS_GetEventsUnknownTID(t *testing.T) {
	srv := newTestServer(t)
	if status := postJSON(t, srv.URL+"/gsma/rsp2/es11/getEvents",
		smdsv1.GetEventsRequest{TransactionID: "nope"}, nil); status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", status)
	}
}

func TestSMDS_AuthenticateRejectsEmptyChallenge(t *testing.T) {
	srv := newTestServer(t)
	if status := postJSON(t, srv.URL+"/gsma/rsp2/es11/authenticateClient",
		smdsv1.AuthenticateClientRequest{EID: "x"}, nil); status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
}

func TestSMDS_AdminListEvents(t *testing.T) {
	srv := newTestServer(t)
	postJSON(t, srv.URL+"/gsma/rsp2/es12/registerEvent",
		smdsv1.RegisterEventRequest{EID: "a", RSPServerAddress: "x", EventID: "1"}, nil)
	postJSON(t, srv.URL+"/gsma/rsp2/es12/registerEvent",
		smdsv1.RegisterEventRequest{EID: "b", RSPServerAddress: "x", EventID: "2"}, nil)
	resp, err := http.Get(srv.URL + "/v1/events")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var out map[string]any
	json.NewDecoder(resp.Body).Decode(&out)
	if out["length"].(float64) != 2 {
		t.Fatalf("length = %v", out["length"])
	}
}

// TestSMDS_AuthenticateClient_SigningEndToEnd stands up a fake HSM
// broker that signs with a generated P-256 key, configures the
// SM-DS server to use it, drives an authenticateClient request,
// and verifies that the returned ServerSigned1 + ECDSA-SHA-256
// signature validate against the broker's public key.
//
// Mirrors the smdp-plus equivalent. Closes the smds README's
// "AuthenticateClient — Skeleton (no signature verification yet)"
// caveat for the SM-DS-side signing half; LPA-side verification
// against an SM-DS identity certificate is a separate test.
func TestSMDS_AuthenticateClient_SigningEndToEnd(t *testing.T) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("gen key: %v", err)
	}
	hsmSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/sign") {
			http.NotFound(w, r)
			return
		}
		var req struct {
			KeyID  string `json:"key_id"`
			Digest []byte `json:"digest"`
			Hash   string `json:"hash"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		if req.KeyID != "smds-auth-key" {
			http.Error(w, "wrong key id", 400)
			return
		}
		rr, ss, err := ecdsa.Sign(rand.Reader, priv, req.Digest)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		der, _ := asn1.Marshal(struct{ R, S *big.Int }{rr, ss})
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"signature_der": der})
	}))
	defer hsmSrv.Close()

	hc := hsmclient.New(hsmSrv.URL)
	srv := httptest.NewServer(New(events.NewMemoryStore(), Config{
		Signer: &Signer{
			Broker:        hc,
			KeyID:         "smds-auth-key",
			ServerAddress: "smds.aether.local",
		},
	}).Routes())
	defer srv.Close()

	euiccChallenge := bytes.Repeat([]byte{0xCC}, 16)
	body, _ := json.Marshal(smdsv1.AuthenticateClientRequest{
		EID:            "8900000000000000000000000000000001",
		EUICCChallenge: euiccChallenge,
	})
	resp, err := http.Post(srv.URL+"/gsma/rsp2/es11/authenticateClient", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}

	var out smdsv1.AuthenticateClientResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.TransactionID == "" {
		t.Fatal("transaction_id empty")
	}
	if len(out.ServerSigned1) == 0 {
		t.Fatal("server_signed1 empty — signing did not run")
	}
	if len(out.ServerSignature1) == 0 {
		t.Fatal("server_signature1 empty")
	}

	// The ServerSigned1 must decode and the signature must verify.
	parsed, err := signing.UnmarshalServerSigned1(out.ServerSigned1)
	if err != nil {
		t.Fatalf("unmarshal server_signed1: %v", err)
	}
	if parsed.ServerAddress != "smds.aether.local" {
		t.Errorf("server_address in payload = %q", parsed.ServerAddress)
	}
	if !bytes.Equal(parsed.EUICCChallenge, euiccChallenge) {
		t.Error("euicc_challenge in payload did not match request")
	}
	if len(parsed.ServerChallenge) != 16 {
		t.Errorf("server_challenge length = %d, want 16", len(parsed.ServerChallenge))
	}
	if hex.EncodeToString(parsed.TransactionID) != out.TransactionID {
		t.Errorf("transactionId in payload = %x, response said %s", parsed.TransactionID, out.TransactionID)
	}

	digest := sha256.Sum256(out.ServerSigned1)
	var sig struct{ R, S *big.Int }
	if _, err := asn1.Unmarshal(out.ServerSignature1, &sig); err != nil {
		t.Fatalf("sig unmarshal: %v", err)
	}
	if !ecdsa.Verify(&priv.PublicKey, digest[:], sig.R, sig.S) {
		t.Fatal("ECDSA verify failed against the broker's public key")
	}
}

func TestSMDS_AuthenticateClient_RejectsBadChallengeWhenSigning(t *testing.T) {
	hsmSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("HSM Sign must not be called for a bad challenge")
	}))
	defer hsmSrv.Close()

	hc := hsmclient.New(hsmSrv.URL)
	srv := httptest.NewServer(New(events.NewMemoryStore(), Config{
		Signer: &Signer{Broker: hc, KeyID: "smds-auth-key", ServerAddress: "x"},
	}).Routes())
	defer srv.Close()

	body, _ := json.Marshal(smdsv1.AuthenticateClientRequest{
		EID:            "8900000000000000000000000000000001",
		EUICCChallenge: []byte{0x01, 0x02, 0x03}, // not 16 bytes
	})
	resp, _ := http.Post(srv.URL+"/gsma/rsp2/es11/authenticateClient", "application/json", bytes.NewReader(body))
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestSMDS_AuthenticateClient_LabModeNoSigning(t *testing.T) {
	srv := newTestServer(t)
	body, _ := json.Marshal(smdsv1.AuthenticateClientRequest{
		EID:            "8900000000000000000000000000000001",
		EUICCChallenge: []byte{0x01, 0x02, 0x03}, // any non-empty length OK in lab
	})
	resp, _ := http.Post(srv.URL+"/gsma/rsp2/es11/authenticateClient", "application/json", bytes.NewReader(body))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var out smdsv1.AuthenticateClientResponse
	json.NewDecoder(resp.Body).Decode(&out)
	if len(out.ServerSigned1) != 0 || len(out.ServerSignature1) != 0 {
		t.Fatal("lab mode must not produce signed payloads")
	}
}
