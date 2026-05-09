package store

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"time"
)

// LabChain is a programmatically generated SGP.26-style certificate
// chain useful for tests and the local lab. It is NOT a substitute
// for the real GSMA SGP.26 test certificates — those need to be
// vendored from the published GSMA test material in a follow-up.
//
// Layout: a self-signed CI root, an EUM intermediate, and three
// leaf identity certs (DPtls, DPauth, DPpb) issued by the EUM.
type LabChain struct {
	RootPEM   []byte
	EUMPEM    []byte
	IdentityPEM map[Identity][]byte
}

// GenerateLabChain produces a fresh chain valid for 1 year.
func GenerateLabChain() (*LabChain, error) {
	now := time.Now()
	notAfter := now.Add(365 * 24 * time.Hour)

	rootKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	rootTpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Aether Lab CI Root (TEST ONLY)"},
		NotBefore:             now,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	rootDER, err := x509.CreateCertificate(rand.Reader, rootTpl, rootTpl, &rootKey.PublicKey, rootKey)
	if err != nil {
		return nil, err
	}
	rootCert, _ := x509.ParseCertificate(rootDER)

	eumKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	eumTpl := &x509.Certificate{
		SerialNumber:          big.NewInt(2),
		Subject:               pkix.Name{CommonName: "Aether Lab EUM (TEST ONLY)"},
		NotBefore:             now,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	eumDER, err := x509.CreateCertificate(rand.Reader, eumTpl, rootCert, &eumKey.PublicKey, rootKey)
	if err != nil {
		return nil, err
	}
	eumCert, _ := x509.ParseCertificate(eumDER)

	out := &LabChain{
		RootPEM:     pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: rootDER}),
		EUMPEM:      pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: eumDER}),
		IdentityPEM: make(map[Identity][]byte),
	}

	for i, name := range []Identity{IdentityDPTLS, IdentityDPAuth, IdentityDPpb} {
		leafKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		leafTpl := &x509.Certificate{
			SerialNumber: big.NewInt(int64(10 + i)),
			Subject:      pkix.Name{CommonName: "Aether Lab " + string(name) + " (TEST ONLY)"},
			NotBefore:    now,
			NotAfter:     notAfter,
			KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
			ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
			DNSNames:     []string{"aether.local"},
		}
		leafDER, err := x509.CreateCertificate(rand.Reader, leafTpl, eumCert, &leafKey.PublicKey, eumKey)
		if err != nil {
			return nil, err
		}
		out.IdentityPEM[name] = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER})
	}
	return out, nil
}

// WriteFiles writes the chain to dir as ci-roots.pem, eum.pem, and
// per-identity PEM files. The directory must already exist.
//
// The writer is the function used to put each file in place; pass
// os.WriteFile (which has the signature func(string, []byte, fs.FileMode))
// via a thin closure if you want default permissions.
func (c *LabChain) WriteFiles(dir string, writer func(path string, data []byte, perm os.FileMode) error) error {
	files := map[string][]byte{
		"ci-roots.pem": c.RootPEM,
		"eum.pem":      c.EUMPEM,
		"DPtls.pem":    c.IdentityPEM[IdentityDPTLS],
		"DPauth.pem":   c.IdentityPEM[IdentityDPAuth],
		"DPpb.pem":     c.IdentityPEM[IdentityDPpb],
	}
	for name, data := range files {
		if err := writer(dir+"/"+name, data, 0o644); err != nil {
			return err
		}
	}
	return nil
}
