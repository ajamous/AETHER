package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGateway_Health(t *testing.T) {
	s, _ := New(Config{ProfileBuilder: "http://pb", SMDPPlus: "http://smdp", CertMgr: "http://cm"})
	srv := httptest.NewServer(s.Routes())
	defer srv.Close()
	resp, _ := http.Get(srv.URL + "/v1/health")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var out map[string]any
	json.NewDecoder(resp.Body).Decode(&out)
	if out["ready"] != true {
		t.Fatal("not ready")
	}
}

func TestGateway_DownloadOrder_HappyPath(t *testing.T) {
	s, _ := New(Config{})
	srv := httptest.NewServer(s.Routes())
	defer srv.Close()
	body, _ := json.Marshal(DownloadOrderRequest{ICCID: "8900000000000000001"})
	resp, err := http.Post(srv.URL+"/gsma/rsp2/es2plus/downloadOrder", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var out DownloadOrderResponse
	json.NewDecoder(resp.Body).Decode(&out)
	if out.ICCID != "8900000000000000001" {
		t.Fatalf("iccid not echoed: %q", out.ICCID)
	}
}

func TestGateway_DownloadOrder_RejectsEmpty(t *testing.T) {
	s, _ := New(Config{})
	srv := httptest.NewServer(s.Routes())
	defer srv.Close()
	resp, _ := http.Post(srv.URL+"/gsma/rsp2/es2plus/downloadOrder", "application/json", strings.NewReader("{}"))
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestGateway_ProxyToProfileBuilder(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/templates" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"templates":["lab-mvno"]}`))
	}))
	defer upstream.Close()

	s, _ := New(Config{ProfileBuilder: upstream.URL})
	srv := httptest.NewServer(s.Routes())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/templates")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var out map[string][]string
	json.NewDecoder(resp.Body).Decode(&out)
	if len(out["templates"]) != 1 || out["templates"][0] != "lab-mvno" {
		t.Fatalf("unexpected proxy result: %+v", out)
	}
}
