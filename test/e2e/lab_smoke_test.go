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

func TestLab_SMDSDiscoveryFlow(t *testing.T) {
	smds := os.Getenv("AETHER_SMDS")
	if smds == "" {
		smds = "http://localhost:8448"
	}
	smds = strings.TrimRight(smds, "/")

	// Register an event as if we were the SM-DP+.
	eid := "89049032123451234512345678901234"
	reg := []byte(`{"eid":"` + eid + `","rsp_server_address":"smdp.example","event_id":"e2e-test"}`)
	resp, err := http.Post(smds+"/gsma/rsp2/es12/registerEvent", "application/json", bytes.NewReader(reg))
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("register: %d", resp.StatusCode)
	}

	// Auth as the LPA.
	auth := []byte(`{"eid":"` + eid + `","euicc_challenge":"Y2g="}`)
	resp, err = http.Post(smds+"/gsma/rsp2/es11/authenticateClient", "application/json", bytes.NewReader(auth))
	if err != nil {
		t.Fatalf("auth: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("auth: %d", resp.StatusCode)
	}
	var ar struct {
		TransactionID string `json:"transaction_id"`
	}
	json.NewDecoder(resp.Body).Decode(&ar)

	// Poll for events.
	get := []byte(`{"transaction_id":"` + ar.TransactionID + `"}`)
	resp, err = http.Post(smds+"/gsma/rsp2/es11/getEvents", "application/json", bytes.NewReader(get))
	if err != nil {
		t.Fatalf("getEvents: %v", err)
	}
	defer resp.Body.Close()
	var gr struct {
		Events []struct {
			RSPServerAddress string `json:"rsp_server_address"`
		} `json:"events"`
	}
	json.NewDecoder(resp.Body).Decode(&gr)
	if len(gr.Events) == 0 || gr.Events[0].RSPServerAddress != "smdp.example" {
		t.Fatalf("expected discovery to find smdp.example, got %+v", gr.Events)
	}

	// Cleanup.
	http.Post(smds+"/gsma/rsp2/es12/deleteEvent", "application/json",
		bytes.NewReader([]byte(`{"eid":"`+eid+`","event_id":"e2e-test"}`)))
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
