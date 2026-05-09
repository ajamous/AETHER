package store

import (
	"os"
	"path/filepath"
	"testing"
)

func writeChainTo(t *testing.T) (string, *LabChain) {
	t.Helper()
	chain, err := GenerateLabChain()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	dir := t.TempDir()
	if err := chain.WriteFiles(dir, os.WriteFile); err != nil {
		t.Fatalf("write: %v", err)
	}
	// WriteFiles uses 0644 implicitly via os.WriteFile signature.
	return dir, chain
}

func TestStore_LoadsAndVerifies(t *testing.T) {
	dir, _ := writeChainTo(t)

	s, err := New(Config{
		Mode:              ModeLab,
		TrustStorePath:    filepath.Join(dir, "ci-roots.pem"),
		IntermediatesPath: filepath.Join(dir, "eum.pem"),
		IdentityPaths: map[Identity]string{
			IdentityDPTLS:  filepath.Join(dir, "DPtls.pem"),
			IdentityDPAuth: filepath.Join(dir, "DPauth.pem"),
			IdentityDPpb:   filepath.Join(dir, "DPpb.pem"),
		},
	})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	if s.Mode() != ModeLab {
		t.Fatalf("mode = %q, want lab", s.Mode())
	}
	identities := s.Identities()
	if len(identities) != 3 {
		t.Fatalf("expected 3 identity certs, got %d", len(identities))
	}
	for _, name := range []Identity{IdentityDPTLS, IdentityDPAuth, IdentityDPpb} {
		c, ok := s.Identity(name)
		if !ok {
			t.Fatalf("identity %q not loaded", name)
		}
		if c.Cert.Subject.CommonName == "" {
			t.Fatalf("identity %q has empty CN", name)
		}
	}
	if len(s.Roots()) != 1 {
		t.Fatalf("expected 1 root, got %d", len(s.Roots()))
	}
	if len(s.Intermediates()) != 1 {
		t.Fatalf("expected 1 intermediate, got %d", len(s.Intermediates()))
	}
}

func TestStore_VerifyChainAcceptsLabIdentities(t *testing.T) {
	dir, _ := writeChainTo(t)

	s, err := New(Config{
		Mode:              ModeLab,
		TrustStorePath:    filepath.Join(dir, "ci-roots.pem"),
		IntermediatesPath: filepath.Join(dir, "eum.pem"),
	})
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	// Verify each identity cert against the trust store directly.
	for _, name := range []Identity{IdentityDPTLS, IdentityDPAuth, IdentityDPpb} {
		c, err := loadCert(name, filepath.Join(dir, string(name)+".pem"))
		if err != nil {
			t.Fatalf("load %s: %v", name, err)
		}
		if err := s.VerifyChain(c.Cert, s.Intermediates()); err != nil {
			t.Fatalf("verify %s: %v", name, err)
		}
	}
}

func TestStore_RejectsEmptyTrustStore(t *testing.T) {
	dir := t.TempDir()
	emptyTrust := filepath.Join(dir, "empty.pem")
	if err := os.WriteFile(emptyTrust, []byte("# no certs here\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := New(Config{
		Mode:           ModeLab,
		TrustStorePath: emptyTrust,
	})
	if err == nil {
		t.Fatal("expected error on empty trust store")
	}
}

func TestStore_RejectsInvalidMode(t *testing.T) {
	_, err := New(Config{Mode: Mode("nonsense")})
	if err == nil {
		t.Fatal("expected error on invalid mode")
	}
}
