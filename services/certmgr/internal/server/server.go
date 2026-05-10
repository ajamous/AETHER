// Package server is the certmgr HTTP transport.
//
// Endpoints:
//
//	GET /v1/health        readiness with cert-expiry summary
//	GET /v1/certs         list loaded identity certs
//	GET /v1/certs/{name}  fetch one identity cert as PEM
//	GET /v1/trust-store   list CI roots
//	GET /metrics          Prometheus exposition (text format)
package server

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/ajamous/aether/services/certmgr/internal/store"
)

// ExpiryWarnDays sets the threshold below which a cert is considered
// "expiring soon" for the purposes of /v1/health and metrics labels.
const ExpiryWarnDays = 30

type Server struct {
	st *store.Store
}

func New(st *store.Store) *Server {
	return &Server{st: st}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/health", s.handleHealth)
	mux.HandleFunc("GET /v1/certs", s.handleListCerts)
	mux.HandleFunc("GET /v1/certs/{name}", s.handleGetCert)
	mux.HandleFunc("GET /v1/trust-store", s.handleTrustStore)
	mux.HandleFunc("GET /v1/trust-store/pem", s.handleTrustStorePEM)
	mux.HandleFunc("GET /v1/intermediates/pem", s.handleIntermediatesPEM)
	mux.HandleFunc("GET /metrics", s.handleMetrics)
	return mux
}

func (s *Server) ListenAndServe(ctx context.Context, addr string) error {
	srv := &http.Server{Addr: addr, Handler: s.Routes(), ReadHeaderTimeout: 10 * time.Second}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

// --- handlers -------------------------------------------------------------

type certView struct {
	Name            string    `json:"name"`
	Subject         string    `json:"subject"`
	Issuer          string    `json:"issuer"`
	NotBefore       time.Time `json:"not_before"`
	NotAfter        time.Time `json:"not_after"`
	DaysUntilExpiry int       `json:"days_until_expiry"`
	SerialNumber    string    `json:"serial_number"`
	LoadedAt        time.Time `json:"loaded_at"`
}

func toView(c *store.Cert) certView {
	d := int(time.Until(c.Cert.NotAfter).Hours() / 24)
	return certView{
		Name:            string(c.Name),
		Subject:         c.Cert.Subject.String(),
		Issuer:          c.Cert.Issuer.String(),
		NotBefore:       c.Cert.NotBefore,
		NotAfter:        c.Cert.NotAfter,
		DaysUntilExpiry: d,
		SerialNumber:    c.Cert.SerialNumber.String(),
		LoadedAt:        c.LoadedAt,
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	identities := s.st.Identities()
	expiringSoon := []string{}
	earliest := -1
	for name, c := range identities {
		days := int(time.Until(c.Cert.NotAfter).Hours() / 24)
		if days <= ExpiryWarnDays {
			expiringSoon = append(expiringSoon, string(name))
		}
		if earliest == -1 || days < earliest {
			earliest = days
		}
	}
	sort.Strings(expiringSoon)
	writeJSON(w, http.StatusOK, map[string]any{
		"ready":                len(identities) > 0,
		"mode":                 string(s.st.Mode()),
		"identities":           len(identities),
		"trust_store_size":     len(s.st.Roots()),
		"expiring_soon":        expiringSoon,
		"earliest_expiry_days": earliest,
	})
}

func (s *Server) handleListCerts(w http.ResponseWriter, _ *http.Request) {
	out := []certView{}
	for _, c := range s.st.Identities() {
		out = append(out, toView(c))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleGetCert(w http.ResponseWriter, r *http.Request) {
	name := store.Identity(r.PathValue("name"))
	c, ok := s.st.Identity(name)
	if !ok {
		writeProblem(w, http.StatusNotFound, fmt.Sprintf("identity %q not loaded", name))
		return
	}
	w.Header().Set("Content-Type", "application/x-pem-file")
	_, _ = w.Write(c.PEM)
}

func (s *Server) handleTrustStore(w http.ResponseWriter, _ *http.Request) {
	out := []map[string]any{}
	for _, c := range s.st.Roots() {
		out = append(out, map[string]any{
			"subject":   c.Subject.String(),
			"not_after": c.NotAfter,
			"serial":    c.SerialNumber.String(),
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleTrustStorePEM returns every loaded CI root concatenated as PEM,
// for callers that need to verify peer cert chains (e.g. smdp-plus
// validating an eUICC's response per SGP.22 §5.7.5).
func (s *Server) handleTrustStorePEM(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/x-pem-file")
	for _, c := range s.st.Roots() {
		_ = pem.Encode(w, &pem.Block{Type: "CERTIFICATE", Bytes: c.Raw})
	}
}

// handleIntermediatesPEM returns the loaded intermediate CAs (e.g. EUM)
// concatenated as PEM. Empty body if the store has no intermediates.
func (s *Server) handleIntermediatesPEM(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/x-pem-file")
	for _, c := range s.st.Intermediates() {
		_ = pem.Encode(w, &pem.Block{Type: "CERTIFICATE", Bytes: c.Raw})
	}
}

func (s *Server) handleMetrics(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	var b strings.Builder

	b.WriteString("# HELP aether_certmgr_mode Active certificate mode (1=active).\n")
	b.WriteString("# TYPE aether_certmgr_mode gauge\n")
	for _, m := range []store.Mode{store.ModeLab, store.ModeProduction} {
		v := 0
		if m == s.st.Mode() {
			v = 1
		}
		fmt.Fprintf(&b, "aether_certmgr_mode{mode=%q} %d\n", string(m), v)
	}

	b.WriteString("# HELP aether_cert_expiry_days Days until cert notAfter.\n")
	b.WriteString("# TYPE aether_cert_expiry_days gauge\n")
	for name, c := range s.st.Identities() {
		days := int(time.Until(c.Cert.NotAfter).Hours() / 24)
		fmt.Fprintf(&b, "aether_cert_expiry_days{name=%q,subject=%q} %d\n",
			string(name), c.Cert.Subject.CommonName, days)
	}

	b.WriteString("# HELP aether_cert_loaded Identity cert loaded (1=loaded).\n")
	b.WriteString("# TYPE aether_cert_loaded gauge\n")
	for name, c := range s.st.Identities() {
		fmt.Fprintf(&b, "aether_cert_loaded{name=%q,issuer=%q,subject=%q} 1\n",
			string(name), c.Cert.Issuer.CommonName, c.Cert.Subject.CommonName)
	}

	b.WriteString("# HELP aether_trust_store_size Number of CI roots loaded.\n")
	b.WriteString("# TYPE aether_trust_store_size gauge\n")
	fmt.Fprintf(&b, "aether_trust_store_size %d\n", len(s.st.Roots()))

	_, _ = w.Write([]byte(b.String()))
}

// --- helpers --------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeProblem(w http.ResponseWriter, status int, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type":   "about:blank",
		"title":  http.StatusText(status),
		"status": status,
		"detail": detail,
	})
}
