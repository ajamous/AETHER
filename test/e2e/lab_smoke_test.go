// Package e2e holds the smoke tests that run against a live lab stack.
//
// Run after `make lab-up`:
//
//	go test ./test/e2e/... -tags=lab
//
// Without the `-tags=lab` build tag these tests skip, so they don't
// fail in CI on environments that haven't started the stack.
//go:build lab

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// gatewayURL points at the lab gateway. Override via AETHER_GATEWAY env.
func gatewayURL() string {
	if v := os.Getenv("AETHER_GATEWAY"); v != "" {
		return strings.TrimRight(v, "/")
	}
	return "http://localhost:8080"
}

func certmgrURL() string {
	if v := os.Getenv("AETHER_CERTMGR"); v != "" {
		return strings.TrimRight(v, "/")
	}
	return "http://localhost:8444"
}

func auditURL() string {
	if v := os.Getenv("AETHER_AUDIT"); v != "" {
		return strings.TrimRight(v, "/")
	}
	return "http://localhost:8447"
}

func TestLab_GatewayHealth(t *testing.T) {
	resp := mustGet(t, gatewayURL()+"/v1/health")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("gateway /v1/health: %d", resp.StatusCode)
	}
}

func TestLab_TemplatesProxiedThroughGateway(t *testing.T) {
	resp := mustGet(t, gatewayURL()+"/v1/templates")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/v1/templates: %d", resp.StatusCode)
	}
	var out struct {
		Templates []string `json:"templates"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Templates) == 0 {
		t.Fatal("expected at least one template (lab-mvno)")
	}
}

func TestLab_CertmgrMetricsReachable(t *testing.T) {
	resp := mustGet(t, certmgrURL()+"/metrics")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("certmgr /metrics: %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "aether_cert_loaded") {
		t.Fatalf("metrics output missing aether_cert_loaded:\n%s", body)
	}
}

func TestLab_AuditChainAppendAndVerify(t *testing.T) {
	body := []byte(`{"event":"e2e.smoke","actor":"lab"}`)
	resp, err := http.Post(auditURL()+"/v1/events", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post audit: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("audit append: %d", resp.StatusCode)
	}
	v := mustGet(t, auditURL()+"/v1/verify")
	defer v.Body.Close()
	if v.StatusCode != http.StatusOK {
		t.Fatalf("audit verify: %d", v.StatusCode)
	}
}

func TestLab_DownloadOrderRoundTrip(t *testing.T) {
	req := []byte(`{"iccid":"8900000000000000001","profile_type":"lab-mvno"}`)
	resp, err := http.Post(gatewayURL()+"/gsma/rsp2/es2plus/downloadOrder", "application/json", bytes.NewReader(req))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("downloadOrder: %d", resp.StatusCode)
	}
	var out struct {
		ICCID string `json:"iccid"`
	}
	json.NewDecoder(resp.Body).Decode(&out)
	if out.ICCID == "" {
		t.Fatal("expected ICCID echo")
	}
}

func mustGet(t *testing.T, url string) *http.Response {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get %s: %v", url, err)
	}
	return resp
}
