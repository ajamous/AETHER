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

func postJSON(t *testing.T, url string, body any, dst any) int {
	t.Helper()
	buf, _ := json.Marshal(body)
	resp, err := http.Post(url, "application/json", bytes.NewReader(buf))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if dst != nil && resp.StatusCode/100 == 2 {
		_ = json.NewDecoder(resp.Body).Decode(dst)
	}
	return resp.StatusCode
}

func mustGet(t *testing.T, url string, dst any) int {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("get %s: %v", url, err)
	}
	defer resp.Body.Close()
	if dst != nil && resp.StatusCode/100 == 2 {
		_ = json.NewDecoder(resp.Body).Decode(dst)
	}
	return resp.StatusCode
}

func TestEIM_Health(t *testing.T) {
	srv := newTestServer(t)
	if status := mustGet(t, srv.URL+"/v1/health", nil); status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
}

func TestEIM_FleetLifecycle(t *testing.T) {
	srv := newTestServer(t)

	// Operator registers a device.
	var dev eimv1.Device
	if status := postJSON(t, srv.URL+"/v1/devices", eimv1.RegisterDeviceRequest{
		EID: "89049032123451234512345678901234", Label: "iot-meter-01",
		Tags: []string{"meter", "fleet-A"},
	}, &dev); status != http.StatusCreated {
		t.Fatalf("register: %d", status)
	}

	// Operator queues a download_profile command.
	var cmd eimv1.Command
	if status := postJSON(t, srv.URL+"/v1/devices/"+string(dev.EID)+"/commands",
		eimv1.EnqueueCommandRequest{
			Kind:        eimv1.CommandDownloadProfile,
			SMDPAddress: "smdp.example", MatchingID: "ABC-123",
		}, &cmd); status != http.StatusCreated {
		t.Fatalf("enqueue: %d", status)
	}
	if cmd.State != eimv1.CommandStatePending {
		t.Fatalf("state = %q, want pending", cmd.State)
	}

	// IPA polls — should see the command and it should now be Delivered.
	var pollOut eimv1.ListCommandsResponse
	mustGet(t, srv.URL+"/v1/ipa/"+string(dev.EID)+"/poll", &pollOut)
	if len(pollOut.Commands) != 1 {
		t.Fatalf("expected 1 command on poll, got %d", len(pollOut.Commands))
	}
	if pollOut.Commands[0].State != eimv1.CommandStateDelivered {
		t.Fatalf("state after poll = %q, want delivered", pollOut.Commands[0].State)
	}

	// Second poll: same command still Delivered (not re-Pending), but
	// it's still in the active list because IPA hasn't acked yet.
	mustGet(t, srv.URL+"/v1/ipa/"+string(dev.EID)+"/poll", &pollOut)
	if len(pollOut.Commands) != 1 || pollOut.Commands[0].State != eimv1.CommandStateDelivered {
		t.Fatalf("re-poll should still see the delivered command: %+v", pollOut.Commands)
	}

	// IPA acks success.
	if status := postJSON(t, srv.URL+"/v1/ipa/"+string(dev.EID)+"/commands/"+cmd.ID+"/ack",
		eimv1.AckCommandRequest{State: eimv1.CommandStateCompleted}, nil); status != http.StatusNoContent {
		t.Fatalf("ack: %d", status)
	}

	// Third poll: command is Completed and not in the active list anymore.
	pollOut = eimv1.ListCommandsResponse{}
	mustGet(t, srv.URL+"/v1/ipa/"+string(dev.EID)+"/poll", &pollOut)
	if len(pollOut.Commands) != 0 {
		t.Fatalf("expected 0 active commands after ack, got %d", len(pollOut.Commands))
	}

	// Operator's view (with completed) still shows it.
	var adminOut eimv1.ListCommandsResponse
	mustGet(t, srv.URL+"/v1/devices/"+string(dev.EID)+"/commands", &adminOut)
	if len(adminOut.Commands) != 1 || adminOut.Commands[0].State != eimv1.CommandStateCompleted {
		t.Fatalf("operator view should retain completed command, got %+v", adminOut.Commands)
	}
}

func TestEIM_RegisterRejectsEmpty(t *testing.T) {
	srv := newTestServer(t)
	if status := postJSON(t, srv.URL+"/v1/devices", eimv1.RegisterDeviceRequest{}, nil); status != http.StatusBadRequest {
		t.Fatalf("status = %d", status)
	}
}

func TestEIM_RegisterRejectsDuplicate(t *testing.T) {
	srv := newTestServer(t)
	postJSON(t, srv.URL+"/v1/devices", eimv1.RegisterDeviceRequest{EID: "x"}, nil)
	if status := postJSON(t, srv.URL+"/v1/devices", eimv1.RegisterDeviceRequest{EID: "x"}, nil); status != http.StatusConflict {
		t.Fatalf("status = %d, want 409", status)
	}
}

func TestEIM_GetUnknownDevice(t *testing.T) {
	srv := newTestServer(t)
	if status := mustGet(t, srv.URL+"/v1/devices/missing", nil); status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", status)
	}
}

func TestEIM_EnqueueRejectsBadKind(t *testing.T) {
	srv := newTestServer(t)
	postJSON(t, srv.URL+"/v1/devices", eimv1.RegisterDeviceRequest{EID: "x"}, nil)
	if status := postJSON(t, srv.URL+"/v1/devices/x/commands",
		map[string]any{"kind": "bogus_kind"}, nil); status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
}

func TestEIM_AckRejectsBadState(t *testing.T) {
	srv := newTestServer(t)
	postJSON(t, srv.URL+"/v1/devices", eimv1.RegisterDeviceRequest{EID: "x"}, nil)
	var cmd eimv1.Command
	postJSON(t, srv.URL+"/v1/devices/x/commands",
		eimv1.EnqueueCommandRequest{Kind: eimv1.CommandEnableProfile}, &cmd)

	// state=pending isn't valid for an ack.
	if status := postJSON(t, srv.URL+"/v1/ipa/x/commands/"+cmd.ID+"/ack",
		eimv1.AckCommandRequest{State: eimv1.CommandStatePending}, nil); status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
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
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}
	if status := mustGet(t, srv.URL+"/v1/devices/x", nil); status != http.StatusNotFound {
		t.Fatalf("expected 404 after delete, got %d", status)
	}
}

func TestEIM_RejectsBadJSON(t *testing.T) {
	srv := newTestServer(t)
	resp, err := http.Post(srv.URL+"/v1/devices", "application/json", strings.NewReader("not json"))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}
