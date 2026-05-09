package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
