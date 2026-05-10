package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	smdpv1 "github.com/ajamous/aether/services/smdp-plus/api/v1"
	"github.com/ajamous/aether/services/smdp-plus/internal/session"
)

func newTestServer(t *testing.T) (*httptest.Server, session.Store) {
	t.Helper()
	st := session.NewMemoryStore(10 * time.Minute)
	srv := httptest.NewServer(New(st).Routes())
	t.Cleanup(srv.Close)
	return srv, st
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

func TestInitiateAuthentication_HappyPath(t *testing.T) {
	srv, _ := newTestServer(t)
	var resp smdpv1.InitiateAuthenticationResponse
	status := postJSON(t, srv.URL+"/gsma/rsp2/es9plus/initiateAuthentication",
		smdpv1.InitiateAuthenticationRequest{
			EUICCChallenge: bytes.Repeat([]byte{0xAA}, 16),
			SMDPAddress:    "aether.local",
		}, &resp)
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	if resp.TransactionID == "" {
		t.Fatal("expected non-empty transaction_id")
	}
}

func TestInitiateAuthentication_RejectsEmptyChallenge(t *testing.T) {
	srv, _ := newTestServer(t)
	status := postJSON(t, srv.URL+"/gsma/rsp2/es9plus/initiateAuthentication",
		smdpv1.InitiateAuthenticationRequest{}, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
}

func TestAuthenticateClient_StateProgression(t *testing.T) {
	srv, _ := newTestServer(t)

	var initResp smdpv1.InitiateAuthenticationResponse
	postJSON(t, srv.URL+"/gsma/rsp2/es9plus/initiateAuthentication",
		smdpv1.InitiateAuthenticationRequest{EUICCChallenge: bytes.Repeat([]byte{0xCC}, 16), SMDPAddress: "x"},
		&initResp)
	tid := initResp.TransactionID
	if tid == "" {
		t.Fatal("no transaction id")
	}

	status := postJSON(t, srv.URL+"/gsma/rsp2/es9plus/authenticateClient",
		smdpv1.AuthenticateClientRequest{TransactionID: tid, AuthenticateServerResponse: []byte("x")}, nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}

	// Calling authenticateClient again should now fail (state != initiated).
	status = postJSON(t, srv.URL+"/gsma/rsp2/es9plus/authenticateClient",
		smdpv1.AuthenticateClientRequest{TransactionID: tid}, nil)
	if status != http.StatusConflict {
		t.Fatalf("re-auth status = %d, want 409", status)
	}
}

func TestAuthenticateClient_UnknownTID(t *testing.T) {
	srv, _ := newTestServer(t)
	status := postJSON(t, srv.URL+"/gsma/rsp2/es9plus/authenticateClient",
		smdpv1.AuthenticateClientRequest{TransactionID: "nope"}, nil)
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", status)
	}
}

func TestGetBoundProfilePackage_ReturnsNotImplemented(t *testing.T) {
	srv, _ := newTestServer(t)
	var initResp smdpv1.InitiateAuthenticationResponse
	postJSON(t, srv.URL+"/gsma/rsp2/es9plus/initiateAuthentication",
		smdpv1.InitiateAuthenticationRequest{EUICCChallenge: bytes.Repeat([]byte{0xCC}, 16), SMDPAddress: "x"},
		&initResp)
	postJSON(t, srv.URL+"/gsma/rsp2/es9plus/authenticateClient",
		smdpv1.AuthenticateClientRequest{TransactionID: initResp.TransactionID}, nil)

	status := postJSON(t, srv.URL+"/gsma/rsp2/es9plus/getBoundProfilePackage",
		smdpv1.GetBoundProfilePackageRequest{TransactionID: initResp.TransactionID}, nil)
	if status != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501 — BPP generation should be honestly unimplemented", status)
	}
}

func TestHandleNotification_HappyPath(t *testing.T) {
	srv, _ := newTestServer(t)
	status := postJSON(t, srv.URL+"/gsma/rsp2/es9plus/handleNotification",
		smdpv1.HandleNotificationRequest{PendingNotification: []byte("notification-bytes")}, nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
}

func TestHealth(t *testing.T) {
	srv, _ := newTestServer(t)
	resp, err := http.Get(srv.URL + "/v1/health")
	if err != nil {
		t.Fatalf("get health: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}
