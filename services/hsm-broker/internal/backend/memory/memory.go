// Package memory implements an in-memory HSM broker backend.
//
// It is the default for unit tests and the CI smoke test where no
// real PKCS#11 module is present. Keys live in process memory only;
// they do not survive a restart and they are explicitly NOT for
// production use.
//
// The backend deliberately mirrors the constraints a real HSM
// imposes: keys are referenced by handle, never by raw material.
// `Sign` operates on a digest, not a message. `DeriveKey` returns
// a handle, not bytes. A caller that works against this backend
// will work against SoftHSM and against AWS CloudHSM.
package memory

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"

	hsmv1 "github.com/ajamous/aether/services/hsm-broker/api/v1"
	"github.com/ajamous/aether/services/hsm-broker/internal/broker"

	cryptoecdsa "github.com/ajamous/aether/pkg/crypto/ecdsa"
	cryptoecka "github.com/ajamous/aether/pkg/crypto/ecka"
)

type entry struct {
	handle hsmv1.KeyHandle
	ecdsa  *ecdsa.PrivateKey
	ecka   *cryptoecka.PrivateKey
	// session keys derived via DeriveKey: raw bytes kept HSM-side only
	sessionBytes []byte
}

// Backend is an in-memory implementation of broker.Broker.
type Backend struct {
	mu   sync.RWMutex
	keys map[string]*entry
}

// New constructs an empty in-memory backend.
func New() *Backend {
	return &Backend{keys: make(map[string]*entry)}
}

func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Errorf("memory: rand: %w", err))
	}
	return hex.EncodeToString(b[:])
}

// Health reports the backend is always ready (no external dependencies).
func (b *Backend) Health(_ context.Context) (*hsmv1.HealthResponse, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return &hsmv1.HealthResponse{
		Ready:          true,
		Backend:        "memory",
		ActiveSessions: uint32(len(b.keys)), //nolint:gosec // in-memory map size bounded by RAM, won't approach 2^32
	}, nil
}

// GenerateKeyPair creates a new ECDSA or ECKA key on the requested curve.
func (b *Backend) GenerateKeyPair(_ context.Context, req *hsmv1.GenerateKeyPairRequest) (*hsmv1.GenerateKeyPairResponse, error) {
	if req == nil {
		return nil, broker.ErrInvalidArgument
	}
	id := newID()
	e := &entry{
		handle: hsmv1.KeyHandle{
			ID:    id,
			Label: req.Label,
			Kind:  req.Kind,
			Curve: req.Curve,
		},
	}
	var pub []byte
	switch req.Kind {
	case hsmv1.KeyKindECDSA:
		curve, err := mapECDSACurve(req.Curve)
		if err != nil {
			return nil, err
		}
		k, err := cryptoecdsa.GenerateKey(curve, rand.Reader)
		if err != nil {
			return nil, fmt.Errorf("memory: generate ECDSA: %w", err)
		}
		e.ecdsa = k
		pub = ecdsaPubBytes(k)
	case hsmv1.KeyKindECKA:
		ec, err := mapECKACurve(req.Curve)
		if err != nil {
			return nil, err
		}
		kp, err := cryptoecka.Generate(ec)
		if err != nil {
			return nil, fmt.Errorf("memory: generate ECKA: %w", err)
		}
		e.ecka = kp
		pub = kp.PublicBytes()
	case hsmv1.KeyKindUnspecified:
		return nil, fmt.Errorf("%w: key kind not specified", broker.ErrUnsupportedKind)
	default:
		return nil, broker.ErrUnsupportedKind
	}

	b.mu.Lock()
	b.keys[id] = e
	b.mu.Unlock()

	return &hsmv1.GenerateKeyPairResponse{
		Handle:    e.handle,
		PublicKey: pub,
	}, nil
}

// Sign performs ECDSA over a pre-computed digest.
func (b *Backend) Sign(_ context.Context, req *hsmv1.SignRequest) (*hsmv1.SignResponse, error) {
	if req == nil {
		return nil, broker.ErrInvalidArgument
	}
	b.mu.RLock()
	e, ok := b.keys[req.KeyID]
	b.mu.RUnlock()
	if !ok || e.ecdsa == nil {
		return nil, broker.ErrKeyNotFound
	}
	if req.DigestAlg != hsmv1.HashSHA256 {
		// We only ship SHA-256 today; adding SHA-384/512 is mechanical.
		return nil, fmt.Errorf("%w: digest alg %q", broker.ErrInvalidArgument, req.DigestAlg)
	}
	sig, err := signDigest(e.ecdsa, req.Digest)
	if err != nil {
		return nil, err
	}
	return &hsmv1.SignResponse{SignatureDER: sig}, nil
}

// Decrypt is intentionally not implemented for the memory backend.
// SGP.22 §5.5 / §H scenarios that need it route through real HSM
// backends; the memory backend is for unit tests of the broker shape.
func (b *Backend) Decrypt(_ context.Context, _ *hsmv1.DecryptRequest) (*hsmv1.DecryptResponse, error) {
	return nil, fmt.Errorf("memory: Decrypt not supported in memory backend")
}

// DeriveKey performs ECKA + X9.63-SHA-256 (SGP.22 §2.6.4) and stores
// the derived bytes under a fresh handle. Bytes never leave the broker.
func (b *Backend) DeriveKey(_ context.Context, req *hsmv1.DeriveKeyRequest) (*hsmv1.DeriveKeyResponse, error) {
	if req == nil {
		return nil, broker.ErrInvalidArgument
	}
	b.mu.RLock()
	local, ok := b.keys[req.KeyID]
	b.mu.RUnlock()
	if !ok || local.ecka == nil {
		return nil, broker.ErrKeyNotFound
	}
	derived, err := local.ecka.DeriveBytes(req.PeerPublic, req.SharedInfo, int(req.KeyLen))
	if err != nil {
		return nil, fmt.Errorf("memory: derive: %w", err)
	}
	id := newID()
	b.mu.Lock()
	b.keys[id] = &entry{
		handle: hsmv1.KeyHandle{
			ID:    id,
			Label: "session",
			Kind:  hsmv1.KeyKindECKA,
			Curve: local.handle.Curve,
		},
		sessionBytes: derived,
	}
	b.mu.Unlock()
	return &hsmv1.DeriveKeyResponse{SessionKeyID: id}, nil
}

// ListKeys enumerates available handles. Metadata only.
func (b *Backend) ListKeys(_ context.Context, req *hsmv1.ListKeysRequest) (*hsmv1.ListKeysResponse, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := make([]hsmv1.KeyHandle, 0, len(b.keys))
	for _, e := range b.keys {
		if req != nil && req.LabelPrefix != "" && !strings.HasPrefix(e.handle.Label, req.LabelPrefix) {
			continue
		}
		out = append(out, e.handle)
	}
	return &hsmv1.ListKeysResponse{Keys: out}, nil
}

// Close is a no-op for the memory backend.
func (b *Backend) Close() error { return nil }

// SessionBytes exposes the raw derived bytes for a session key handle.
// This is used internally by the smdp-plus service when colocated with
// the broker; it is NOT exposed over the network.
//
// Returning an error rather than panic for an unknown handle keeps
// the surface uniform with the rest of the broker contract.
func (b *Backend) SessionBytes(id string) ([]byte, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	e, ok := b.keys[id]
	if !ok {
		return nil, broker.ErrKeyNotFound
	}
	if e.sessionBytes == nil {
		return nil, fmt.Errorf("memory: handle %s is not a session key", id)
	}
	out := make([]byte, len(e.sessionBytes))
	copy(out, e.sessionBytes)
	return out, nil
}
