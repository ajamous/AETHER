package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	eimv1 "github.com/ajamous/aether/services/eim/api/v1"
	"github.com/ajamous/aether/services/eim/internal/devices"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(New(devices.NewMemoryStore()).Routes())
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
	if dst != nil && resp.StatusCode/100 == 2 {
		_ = json.NewDecoder(resp.Body).Decode(dst)
	}
	return resp
}

func TestEIM_Health(t *testing.T) {
	srv := newTestServer(t)
	resp, _ := http.Get(srv.URL + "/v1/health")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func TestEIM_FleetLifecycle(t *testing.T) {
	srv := newTestServer(t)

	// Operator registers a device.
	var dev eimv1.Device
	resp := postJSON(t, srv.URL+"/v1/devices", eimv1.RegisterDeviceRequest{
		EID: "89049032123451234512345678901234", Label: "iot-meter-01",
		Tags: []string{"meter", "fleet-A"},
	}, &dev)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register: %d", resp.StatusCode)
	}

	// Operator queues a download_profile command.
	var cmd eimv1.Command
	resp = postJSON(t, srv.URL+"/v1/devices/"+string(dev.EID)+"/commands",
		eimv1.EnqueueCommandRequest{
			Kind:        eimv1.CommandDownloadProfile,
			SMDPAddress: "smdp.example", MatchingID: "ABC-123",
		}, &cmd)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("enqueue: %d", resp.StatusCode)
	}
	if cmd.State != eimv1.CommandStatePending {
		t.Fatalf("state = %q, want pending", cmd.State)
	}

	// IPA polls — should see the command and it should now be Delivered.
	resp, err := http.Get(srv.URL + "/v1/ipa/" + string(dev.EID) + "/poll")
	if err != nil {
		t.Fatalf("poll: %v", err)
	}
	defer resp.Body.Close()
	var pollOut eimv1.ListCommandsResponse
	json.NewDecoder(resp.Body).Decode(&pollOut)
	if len(pollOut.Commands) != 1 {
		t.Fatalf("expected 1 command on poll, got %d", len(pollOut.Commands))
	}
	if pollOut.Commands[0].State != eimv1.CommandStateDelivered {
		t.Fatalf("state after poll = %q, want delivered", pollOut.Commands[0].State)
	}

	// Second poll: same command still Delivered (not re-Pending), but
	// it's still in the active list because IPA hasn't acked yet.
	resp, _ = http.Get(srv.URL + "/v1/ipa/" + string(dev.EID) + "/poll")
	json.NewDecoder(resp.Body).Decode(&pollOut)
	if len(pollOut.Commands) != 1 || pollOut.Commands[0].State != eimv1.CommandStateDelivered {
		t.Fatalf("re-poll should still see the delivered command: %+v", pollOut.Commands)
	}

	// IPA acks success.
	resp = postJSON(t, srv.URL+"/v1/ipa/"+string(dev.EID)+"/commands/"+cmd.ID+"/ack",
		eimv1.AckCommandRequest{State: eimv1.CommandStateCompleted}, nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("ack: %d", resp.StatusCode)
	}

	// Third poll: command is Completed and not in the active list anymore.
	resp, _ = http.Get(srv.URL + "/v1/ipa/" + string(dev.EID) + "/poll")
	json.NewDecoder(resp.Body).Decode(&pollOut)
	if len(pollOut.Commands) != 0 {
		t.Fatalf("expected 0 active commands after ack, got %d", len(pollOut.Commands))
	}

	// Operator's view (with completed) still shows it.
	resp, _ = http.Get(srv.URL + "/v1/devices/" + string(dev.EID) + "/commands")
	var adminOut eimv1.ListCommandsResponse
	json.NewDecoder(resp.Body).Decode(&adminOut)
	if len(adminOut.Commands) != 1 || adminOut.Commands[0].State != eimv1.CommandStateCompleted {
		t.Fatalf("operator view should retain completed command, got %+v", adminOut.Commands)
	}
}

func TestEIM_RegisterRejectsEmpty(t *testing.T) {
	srv := newTestServer(t)
	resp := postJSON(t, srv.URL+"/v1/devices", eimv1.RegisterDeviceRequest{}, nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func TestEIM_RegisterRejectsDuplicate(t *testing.T) {
	srv := newTestServer(t)
	postJSON(t, srv.URL+"/v1/devices", eimv1.RegisterDeviceRequest{EID: "x"}, nil)
	resp := postJSON(t, srv.URL+"/v1/devices", eimv1.RegisterDeviceRequest{EID: "x"}, nil)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want 409", resp.StatusCode)
	}
}

func TestEIM_GetUnknownDevice(t *testing.T) {
	srv := newTestServer(t)
	resp, _ := http.Get(srv.URL + "/v1/devices/missing")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestEIM_EnqueueRejectsBadKind(t *testing.T) {
	srv := newTestServer(t)
	postJSON(t, srv.URL+"/v1/devices", eimv1.RegisterDeviceRequest{EID: "x"}, nil)
	resp := postJSON(t, srv.URL+"/v1/devices/x/commands",
		map[string]any{"kind": "bogus_kind"}, nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestEIM_AckRejectsBadState(t *testing.T) {
	srv := newTestServer(t)
	postJSON(t, srv.URL+"/v1/devices", eimv1.RegisterDeviceRequest{EID: "x"}, nil)
	var cmd eimv1.Command
	postJSON(t, srv.URL+"/v1/devices/x/commands",
		eimv1.EnqueueCommandRequest{Kind: eimv1.CommandEnableProfile}, &cmd)

	// state=pending isn't valid for an ack.
	resp := postJSON(t, srv.URL+"/v1/ipa/x/commands/"+cmd.ID+"/ack",
		eimv1.AckCommandRequest{State: eimv1.CommandStatePending}, nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestEIM_DeleteDevice(t *testing.T) {
	srv := newTestServer(t)
	postJSON(t, srv.URL+"/v1/devices", eimv1.RegisterDeviceRequest{EID: "x"}, nil)
	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/v1/devices/x", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}
	resp, _ = http.Get(srv.URL + "/v1/devices/x")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 after delete, got %d", resp.StatusCode)
	}
}

func TestEIM_RejectsBadJSON(t *testing.T) {
	srv := newTestServer(t)
	resp, _ := http.Post(srv.URL+"/v1/devices", "application/json", strings.NewReader("not json"))
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}
