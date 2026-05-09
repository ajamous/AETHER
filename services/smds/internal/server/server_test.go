package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	smdsv1 "github.com/ajamous/aether/services/smds/api/v1"
	"github.com/ajamous/aether/services/smds/internal/events"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(New(events.NewMemoryStore()).Routes())
	t.Cleanup(srv.Close)
	return srv
}

func postJSON(t *testing.T, url string, body any, dst any) *http.Response {
	t.Helper()
	buf, _ := json.Marshal(body)
	resp, err := http.Post(url, "application/json", bytes.NewReader(buf))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	if dst != nil && resp.StatusCode == http.StatusOK {
		_ = json.NewDecoder(resp.Body).Decode(dst)
	}
	return resp
}

func TestSMDS_Health(t *testing.T) {
	srv := newTestServer(t)
	resp, _ := http.Get(srv.URL + "/v1/health")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func TestSMDS_FullDiscoveryFlow(t *testing.T) {
	srv := newTestServer(t)
	eid := smdsv1.EID("89049032123451234512345678901234")

	// SM-DP+ side: register a pending event for the LPA's EID.
	var regResp smdsv1.RegisterEventResponse
	resp := postJSON(t, srv.URL+"/gsma/rsp2/es12/registerEvent",
		smdsv1.RegisterEventRequest{
			EID:              eid,
			RSPServerAddress: "smdp.example",
			EventID:          "evt-1",
		}, &regResp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("register status = %d", resp.StatusCode)
	}
	if regResp.EventID != "evt-1" {
		t.Fatalf("event_id = %q", regResp.EventID)
	}

	// LPA side: authenticate, then poll.
	var authResp smdsv1.AuthenticateClientResponse
	resp = postJSON(t, srv.URL+"/gsma/rsp2/es11/authenticateClient",
		smdsv1.AuthenticateClientRequest{
			EID:            eid,
			EUICCChallenge: []byte("challenge"),
		}, &authResp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("auth status = %d", resp.StatusCode)
	}
	if authResp.TransactionID == "" {
		t.Fatal("expected non-empty transaction_id")
	}

	var getResp smdsv1.GetEventsResponse
	resp = postJSON(t, srv.URL+"/gsma/rsp2/es11/getEvents",
		smdsv1.GetEventsRequest{TransactionID: authResp.TransactionID}, &getResp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("getEvents status = %d", resp.StatusCode)
	}
	if len(getResp.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(getResp.Events))
	}
	if getResp.Events[0].RSPServerAddress != "smdp.example" {
		t.Fatalf("rsp_server_address = %q", getResp.Events[0].RSPServerAddress)
	}

	// SM-DP+ deletes the event after delivery.
	resp = postJSON(t, srv.URL+"/gsma/rsp2/es12/deleteEvent",
		smdsv1.DeleteEventRequest{EID: eid, EventID: "evt-1"}, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete status = %d", resp.StatusCode)
	}

	// Polling again returns empty.
	resp = postJSON(t, srv.URL+"/gsma/rsp2/es11/getEvents",
		smdsv1.GetEventsRequest{TransactionID: authResp.TransactionID}, &getResp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("getEvents-2 status = %d", resp.StatusCode)
	}
	if len(getResp.Events) != 0 {
		t.Fatalf("expected 0 events after delete, got %d", len(getResp.Events))
	}
}

func TestSMDS_RegisterRejectsMissingFields(t *testing.T) {
	srv := newTestServer(t)
	resp := postJSON(t, srv.URL+"/gsma/rsp2/es12/registerEvent",
		smdsv1.RegisterEventRequest{}, nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestSMDS_DeleteUnknownReturns404(t *testing.T) {
	srv := newTestServer(t)
	resp := postJSON(t, srv.URL+"/gsma/rsp2/es12/deleteEvent",
		smdsv1.DeleteEventRequest{EID: "x", EventID: "y"}, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestSMDS_GetEventsUnknownTID(t *testing.T) {
	srv := newTestServer(t)
	resp := postJSON(t, srv.URL+"/gsma/rsp2/es11/getEvents",
		smdsv1.GetEventsRequest{TransactionID: "nope"}, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestSMDS_AuthenticateRejectsEmptyChallenge(t *testing.T) {
	srv := newTestServer(t)
	resp := postJSON(t, srv.URL+"/gsma/rsp2/es11/authenticateClient",
		smdsv1.AuthenticateClientRequest{EID: "x"}, nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestSMDS_AdminListEvents(t *testing.T) {
	srv := newTestServer(t)
	postJSON(t, srv.URL+"/gsma/rsp2/es12/registerEvent",
		smdsv1.RegisterEventRequest{EID: "a", RSPServerAddress: "x", EventID: "1"}, nil)
	postJSON(t, srv.URL+"/gsma/rsp2/es12/registerEvent",
		smdsv1.RegisterEventRequest{EID: "b", RSPServerAddress: "x", EventID: "2"}, nil)
	resp, _ := http.Get(srv.URL + "/v1/events")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var out map[string]any
	json.NewDecoder(resp.Body).Decode(&out)
	if out["length"].(float64) != 2 {
		t.Fatalf("length = %v", out["length"])
	}
}
