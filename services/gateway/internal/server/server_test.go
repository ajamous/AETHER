package server

import (
	"bytes"
	"encoding/json"
	"io"
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

// TestGateway_RateLimit_RejectsAfterBurst drives the wired
// middleware end to end: 3 requests with burst=2 → third gets
// 429, the rate-limit counter advances, admin paths are not
// rate-limited, and the counter is exposed on /metrics.
func TestGateway_RateLimit_RejectsAfterBurst(t *testing.T) {
	s, _ := New(Config{
		RateLimitRPS:   0.001, // effectively no refill within the test
		RateLimitBurst: 2,
	})
	srv := httptest.NewServer(s.Routes())
	defer srv.Close()

	// Burst: 2 successful, 3rd rejected with 429.
	for i := 0; i < 2; i++ {
		body, _ := json.Marshal(DownloadOrderRequest{ICCID: "8900000000000000001"})
		resp, err := http.Post(srv.URL+"/gsma/rsp2/es2plus/downloadOrder", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("call %d: %v", i+1, err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("call %d: status = %d, want 200", i+1, resp.StatusCode)
		}
	}
	body, _ := json.Marshal(DownloadOrderRequest{ICCID: "8900000000000000001"})
	resp, err := http.Post(srv.URL+"/gsma/rsp2/es2plus/downloadOrder", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("burst+1: %v", err)
	}
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("burst+1 status = %d, want 429", resp.StatusCode)
	}
	if resp.Header.Get("Retry-After") == "" {
		t.Error("Retry-After header missing on 429")
	}

	// Admin path bypasses: many requests stay 200 even after the
	// public surface is exhausted.
	for i := 0; i < 10; i++ {
		r, err := http.Get(srv.URL + "/v1/health")
		if err != nil {
			t.Fatalf("admin call %d: %v", i+1, err)
		}
		if r.StatusCode != http.StatusOK {
			t.Fatalf("admin call %d: status = %d, want 200", i+1, r.StatusCode)
		}
	}

	// /metrics exposes the rejected counter.
	mr, err := http.Get(srv.URL + "/metrics")
	if err != nil {
		t.Fatalf("metrics: %v", err)
	}
	defer mr.Body.Close()
	buf, _ := io.ReadAll(mr.Body)
	got := string(buf)
	if !strings.Contains(got, "aether_gateway_ratelimit_rejected_total") {
		t.Errorf("metrics output missing rate-limit counter:\n%s", got)
	}
	if !strings.Contains(got, `class="es2plus"} 1`) {
		t.Errorf("expected rate-limit counter for es2plus to be 1; got:\n%s", got)
	}
}
