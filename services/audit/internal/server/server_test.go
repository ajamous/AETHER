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
	"github.com/ajamous/aether/services/audit/internal/anchor"
	"github.com/ajamous/aether/services/audit/internal/chain"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(New(chain.NewLedger()).Routes())
	t.Cleanup(srv.Close)
	return srv
}

func TestAudit_AppendListVerify(t *testing.T) {
	srv := newTestServer(t)

	for i := 0; i < 5; i++ {
		body := []byte(`{"event":"login","actor":"op-1"}`)
		resp, err := http.Post(srv.URL+"/v1/events", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("post: %v", err)
		}
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("status = %d", resp.StatusCode)
		}
	}

	resp, _ := http.Get(srv.URL + "/v1/events")
	var list map[string]any
	json.NewDecoder(resp.Body).Decode(&list)
	if list["length"].(float64) != 5 {
		t.Fatalf("length = %v", list["length"])
	}

	resp, _ = http.Get(srv.URL + "/v1/verify")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("verify status = %d", resp.StatusCode)
	}
	var v map[string]any
	json.NewDecoder(resp.Body).Decode(&v)
	if v["ok"] != true {
		t.Fatalf("verify not ok: %+v", v)
	}
}

func TestAudit_GetByseq(t *testing.T) {
	srv := newTestServer(t)
	http.Post(srv.URL+"/v1/events", "application/json", strings.NewReader(`{"event":"a"}`))
	http.Post(srv.URL+"/v1/events", "application/json", strings.NewReader(`{"event":"b"}`))

	resp, _ := http.Get(srv.URL + "/v1/events/2")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func TestAudit_AppendRejectsBadJSON(t *testing.T) {
	srv := newTestServer(t)
	resp, _ := http.Post(srv.URL+"/v1/events", "application/json", strings.NewReader("not json"))
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

// TestAudit_Anchor_LabUnsigned drives /v1/anchor in the default
// no-signer mode: length, tail_hash, and timestamp are returned;
// no signature fields appear.
func TestAudit_Anchor_LabUnsigned(t *testing.T) {
	srv := newTestServer(t)
	// Empty chain anchor.
	resp, err := http.Get(srv.URL + "/v1/anchor")
	if err != nil {
		t.Fatalf("get empty anchor: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var out map[string]any
	json.NewDecoder(resp.Body).Decode(&out)
	if out["length"].(float64) != 0 {
		t.Errorf("length = %v, want 0", out["length"])
	}
	if th, ok := out["tail_hash"].(string); !ok || len(th) != 64 {
		t.Errorf("tail_hash = %v (want 64-hex zero string)", out["tail_hash"])
	}
	if _, ok := out["signature"]; ok {
		t.Error("unsigned anchor must not include signature field")
	}
	if _, ok := out["signed_payload"]; ok {
		t.Error("unsigned anchor must not include signed_payload field")
	}

	// Append two entries and re-anchor; length should advance and
	// tail_hash should be non-zero.
	http.Post(srv.URL+"/v1/events", "application/json", strings.NewReader(`{"event":"a"}`))
	http.Post(srv.URL+"/v1/events", "application/json", strings.NewReader(`{"event":"b"}`))
	resp2, _ := http.Get(srv.URL + "/v1/anchor")
	var out2 map[string]any
	json.NewDecoder(resp2.Body).Decode(&out2)
	if out2["length"].(float64) != 2 {
		t.Errorf("length after appends = %v", out2["length"])
	}
	if th := out2["tail_hash"].(string); th == strings.Repeat("0", 64) {
		t.Errorf("tail_hash should be non-zero after appends, got %s", th)
	}
}

// TestAudit_Anchor_SignedEndToEnd stands up a fake hsm-broker that
// ECDSA-signs requests, configures the server with that broker,
// and verifies the returned anchor's signature against the
// broker's public key.
func TestAudit_Anchor_SignedEndToEnd(t *testing.T) {
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
		json.NewDecoder(r.Body).Decode(&req)
		if req.KeyID != "audit-anchor-key" {
			http.Error(w, "wrong key id", 400)
			return
		}
		rr, ss, _ := ecdsa.Sign(rand.Reader, priv, req.Digest)
		der, _ := asn1.Marshal(struct{ R, S *big.Int }{rr, ss})
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"signature_der": der})
	}))
	defer hsmSrv.Close()

	hc := hsmclient.New(hsmSrv.URL)
	srv := httptest.NewServer(New(chain.NewLedger(), Config{
		Signer: &AnchorSigner{Broker: hc, KeyID: "audit-anchor-key"},
	}).Routes())
	defer srv.Close()

	// Append one entry so the anchor is non-trivial.
	http.Post(srv.URL+"/v1/events", "application/json", strings.NewReader(`{"event":"first"}`))

	resp, err := http.Get(srv.URL + "/v1/anchor")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var out struct {
		Length        int64  `json:"length"`
		TailHash      string `json:"tail_hash"`
		Timestamp     string `json:"timestamp"`
		SignedPayload []byte `json:"signed_payload"`
		Signature     []byte `json:"signature"`
		SignatureAlg  string `json:"signature_alg"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Length != 1 {
		t.Errorf("length = %d, want 1", out.Length)
	}
	if len(out.SignedPayload) == 0 {
		t.Fatal("signed_payload empty — signing did not run")
	}
	if len(out.Signature) == 0 {
		t.Fatal("signature empty")
	}
	if out.SignatureAlg != "ECDSA-SHA-256" {
		t.Errorf("signature_alg = %q", out.SignatureAlg)
	}

	// The signed_payload must DER-decode to an Anchor matching the
	// JSON-side fields, and the signature must verify.
	parsed, err := anchor.UnmarshalAnchor(out.SignedPayload)
	if err != nil {
		t.Fatalf("unmarshal anchor: %v", err)
	}
	if parsed.Length != out.Length {
		t.Errorf("DER length %d != json length %d", parsed.Length, out.Length)
	}
	if hex.EncodeToString(parsed.TailHash) != out.TailHash {
		t.Errorf("tail hash mismatch DER vs json")
	}

	digest := sha256.Sum256(out.SignedPayload)
	var sig struct{ R, S *big.Int }
	if _, err := asn1.Unmarshal(out.Signature, &sig); err != nil {
		t.Fatalf("sig unmarshal: %v", err)
	}
	if !ecdsa.Verify(&priv.PublicKey, digest[:], sig.R, sig.S) {
		t.Fatal("ECDSA verify failed against the broker's public key")
	}
}
