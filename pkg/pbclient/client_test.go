package pbclient

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBuild_Roundtrip(t *testing.T) {
	wantSAIP := []byte{0x30, 0x03, 0x01, 0x02, 0x03}
	var gotPath string
	var gotSub SubscriberData
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotSub)
		_ = json.NewEncoder(w).Encode(BuildResponse{SAIP: wantSAIP, Note: "ok"})
	}))
	defer srv.Close()

	c := New(srv.URL)
	sub := SubscriberData{IMSI: "001010000000001", ICCID: "8900000000000000001", Ki: bytes.Repeat([]byte{1}, 16), OPc: bytes.Repeat([]byte{2}, 16)}
	out, err := c.Build(context.Background(), "lab-mvno", sub)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if gotPath != "/v1/templates/lab-mvno/build" {
		t.Errorf("path = %q", gotPath)
	}
	if gotSub.IMSI != sub.IMSI || !bytes.Equal(gotSub.Ki, sub.Ki) {
		t.Errorf("subscriber not transmitted: %+v", gotSub)
	}
	if !bytes.Equal(out.SAIP, wantSAIP) {
		t.Errorf("SAIP = %x, want %x", out.SAIP, wantSAIP)
	}
}

func TestBuild_EmptyTemplate(t *testing.T) {
	c := New("http://unused")
	if _, err := c.Build(context.Background(), "", SubscriberData{}); err == nil {
		t.Fatal("expected error on empty template name")
	}
}

func TestBuild_EmptySAIPRejected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(BuildResponse{})
	}))
	defer srv.Close()
	if _, err := New(srv.URL).Build(context.Background(), "t", SubscriberData{}); err == nil {
		t.Fatal("expected error when profile-builder returns empty SAIP")
	}
}

func TestBuild_PropagatesProblem(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"detail": "template \"x\" not found"})
	}))
	defer srv.Close()
	_, err := New(srv.URL).Build(context.Background(), "x", SubscriberData{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !IsNotFound(err) {
		t.Errorf("IsNotFound = false for 404, err = %v", err)
	}
}

func TestHealth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(HealthResponse{Ready: true})
	}))
	defer srv.Close()
	h, err := New(srv.URL).Health(context.Background())
	if err != nil {
		t.Fatalf("health: %v", err)
	}
	if !h.Ready {
		t.Error("ready = false")
	}
}
