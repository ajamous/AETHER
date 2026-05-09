package ratelimit

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestLimiter_AllowsBurst(t *testing.T) {
	l := New(1.0, 5)
	for i := 0; i < 5; i++ {
		if !l.Allow("1.1.1.1") {
			t.Fatalf("call %d should be allowed inside burst", i+1)
		}
	}
	if l.Allow("1.1.1.1") {
		t.Fatal("6th call should be rejected after burst exhausted")
	}
}

func TestLimiter_Refills(t *testing.T) {
	l := New(10.0, 5)
	t0 := time.Unix(0, 0)
	now := t0
	l.now = func() time.Time { return now }

	for i := 0; i < 5; i++ {
		if !l.Allow("1.1.1.1") {
			t.Fatalf("call %d should be allowed", i+1)
		}
	}
	if l.Allow("1.1.1.1") {
		t.Fatal("burst should be exhausted")
	}

	// 1 second later: 10 tokens replenished, capped at burst (5).
	now = t0.Add(time.Second)
	for i := 0; i < 5; i++ {
		if !l.Allow("1.1.1.1") {
			t.Fatalf("call %d after refill should be allowed", i+1)
		}
	}
	if l.Allow("1.1.1.1") {
		t.Fatal("burst should be exhausted again")
	}
}

func TestLimiter_IndependentSources(t *testing.T) {
	l := New(1.0, 3)
	for i := 0; i < 3; i++ {
		if !l.Allow("1.1.1.1") {
			t.Fatalf("source A call %d should be allowed", i+1)
		}
	}
	if l.Allow("1.1.1.1") {
		t.Fatal("source A should be rate-limited")
	}
	// Source B unaffected.
	if !l.Allow("2.2.2.2") {
		t.Fatal("source B first call should be allowed")
	}
}

func TestLimiter_NilPassesThrough(t *testing.T) {
	var l *Limiter
	for i := 0; i < 1000; i++ {
		if !l.Allow("anything") {
			t.Fatal("nil limiter must pass everything")
		}
	}
}

func TestNew_RejectsInvalidConfig(t *testing.T) {
	if New(0, 5) != nil {
		t.Fatal("rps=0 should yield nil")
	}
	if New(-1, 5) != nil {
		t.Fatal("rps<0 should yield nil")
	}
	if New(1, 0) != nil {
		t.Fatal("burst=0 should yield nil")
	}
}

func TestSourceIP(t *testing.T) {
	cases := []struct {
		addr string
		want string
	}{
		{"1.2.3.4:5678", "1.2.3.4"},
		{"[::1]:8080", "::1"},
		{"[2001:db8::1]:443", "2001:db8::1"},
		{"unix-socket", "unix-socket"},
		{"", "unknown"},
	}
	for _, c := range cases {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = c.addr
		got := SourceIP(r)
		if got != c.want {
			t.Errorf("SourceIP(%q) = %q, want %q", c.addr, got, c.want)
		}
	}
}

func TestClassifyPath(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"/gsma/rsp2/es2plus/downloadOrder", "es2plus"},
		{"/gsma/rsp2/es9plus/initiateAuthentication", "es9plus"},
		{"/v1/health", ""},
		{"/v1/templates", ""},
		{"/metrics", ""},
		{"/", ""},
	}
	for _, c := range cases {
		r := httptest.NewRequest("POST", c.path, nil)
		got := ClassifyPath(r)
		if got != c.want {
			t.Errorf("ClassifyPath(%q) = %q, want %q", c.path, got, c.want)
		}
	}
}

func TestMiddleware_RejectsAfterBurst(t *testing.T) {
	l := New(1.0, 2)
	var rejections atomic.Uint64
	reporter := func(class string) { rejections.Add(1) }

	mw := Middleware(l, reporter)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	makeReq := func() *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/gsma/rsp2/es2plus/downloadOrder", strings.NewReader("{}"))
		r.RemoteAddr = "9.9.9.9:1234"
		handler.ServeHTTP(w, r)
		return w
	}

	for i := 0; i < 2; i++ {
		w := makeReq()
		if w.Code != http.StatusOK {
			t.Fatalf("call %d: status = %d, want 200", i+1, w.Code)
		}
	}
	w := makeReq()
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("burst+1 status = %d, want 429", w.Code)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Error("Retry-After header missing on 429")
	}
	if got := rejections.Load(); got != 1 {
		t.Errorf("reporter called %d times, want 1", got)
	}
}

func TestMiddleware_AdminPathsBypass(t *testing.T) {
	l := New(1.0, 1)
	mw := Middleware(l, nil)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < 100; i++ {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/v1/health", nil)
		r.RemoteAddr = "9.9.9.9:1234"
		handler.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("admin path call %d should not be rate-limited (status %d)", i+1, w.Code)
		}
	}
}

func TestMiddleware_NilLimiterPassesThrough(t *testing.T) {
	mw := Middleware(nil, nil)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	for i := 0; i < 100; i++ {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/gsma/rsp2/es2plus/downloadOrder", nil)
		r.RemoteAddr = "9.9.9.9:1234"
		handler.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("nil limiter must pass everything (got %d on call %d)", w.Code, i+1)
		}
	}
}
