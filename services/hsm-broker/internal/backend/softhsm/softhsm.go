// Package softhsm implements the broker.Broker interface backed by a
// PKCS#11 module, with SoftHSM v2 as the default lab module.
//
// Status: skeleton. The lifecycle (open module, find slot, login,
// close) is wired; the cryptographic operations return
// ErrNotImplemented. A focused follow-up PR fills in Sign /
// GenerateKeyPair / DeriveKey / ListKeys against a running SoftHSM,
// gated by integration tests that spin up SoftHSM in CI.
//
// The reason this is staged:
//
//   - PKCS#11 attribute templates and mechanism parameters are detail-
//     dense; getting them wrong silently produces results that look
//     right but aren't (e.g. truncated signatures, wrong KDF inputs).
//   - Each op deserves a dedicated test against a real SoftHSM with
//     known test vectors. Bundling that with the lifecycle code makes
//     the PR too big to review well.
//
// Until that PR lands, the memory backend is the working backend for
// development, CI, and demos. Real-HSM customers swap it in via
// `--backend=softhsm` when they're ready.
package softhsm

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/miekg/pkcs11"

	hsmv1 "github.com/ajamous/aether/services/hsm-broker/api/v1"
	"github.com/ajamous/aether/services/hsm-broker/internal/broker"
)

// Config holds the runtime parameters for a SoftHSM (or any PKCS#11)
// backend.
type Config struct {
	// LibraryPath is the absolute path to the PKCS#11 .so / .dll.
	// For SoftHSM v2 on Linux this is typically
	// /usr/lib/softhsm/libsofthsm2.so.
	LibraryPath string

	// Slot is the PKCS#11 slot ID to operate against.
	Slot uint

	// PIN is the user PIN for the slot.
	PIN string
}

// ErrNotImplemented is returned for backend operations that are not
// yet wired against PKCS#11. Tracking issue: see package doc.
var ErrNotImplemented = errors.New("softhsm: operation not yet implemented; see package doc")

// Backend wraps a PKCS#11 session.
type Backend struct {
	ctx     *pkcs11.Ctx
	session pkcs11.SessionHandle

	mu     sync.Mutex
	closed bool
}

// New opens the PKCS#11 module, finds the slot, opens a session, and
// logs in. The returned Backend MUST be Close()d to release the session.
func New(cfg Config) (*Backend, error) {
	if cfg.LibraryPath == "" {
		return nil, errors.New("softhsm: LibraryPath is required")
	}
	c := pkcs11.New(cfg.LibraryPath)
	if c == nil {
		return nil, fmt.Errorf("softhsm: failed to load PKCS#11 module at %s", cfg.LibraryPath)
	}
	if err := c.Initialize(); err != nil {
		return nil, fmt.Errorf("softhsm: Initialize: %w", err)
	}

	session, err := c.OpenSession(cfg.Slot, pkcs11.CKF_SERIAL_SESSION|pkcs11.CKF_RW_SESSION)
	if err != nil {
		_ = c.Finalize()
		c.Destroy()
		return nil, fmt.Errorf("softhsm: OpenSession slot=%d: %w", cfg.Slot, err)
	}
	if err := c.Login(session, pkcs11.CKU_USER, cfg.PIN); err != nil {
		_ = c.CloseSession(session)
		_ = c.Finalize()
		c.Destroy()
		return nil, fmt.Errorf("softhsm: Login: %w", err)
	}

	return &Backend{ctx: c, session: session}, nil
}

func (b *Backend) Health(_ context.Context) (*hsmv1.HealthResponse, error) {
	b.mu.Lock()
	closed := b.closed
	b.mu.Unlock()
	return &hsmv1.HealthResponse{
		Ready:          !closed,
		Backend:        "softhsm",
		ActiveSessions: 1,
	}, nil
}

func (b *Backend) Sign(context.Context, *hsmv1.SignRequest) (*hsmv1.SignResponse, error) {
	return nil, ErrNotImplemented
}

func (b *Backend) Decrypt(context.Context, *hsmv1.DecryptRequest) (*hsmv1.DecryptResponse, error) {
	return nil, ErrNotImplemented
}

func (b *Backend) DeriveKey(context.Context, *hsmv1.DeriveKeyRequest) (*hsmv1.DeriveKeyResponse, error) {
	return nil, ErrNotImplemented
}

func (b *Backend) GenerateKeyPair(context.Context, *hsmv1.GenerateKeyPairRequest) (*hsmv1.GenerateKeyPairResponse, error) {
	return nil, ErrNotImplemented
}

func (b *Backend) ListKeys(context.Context, *hsmv1.ListKeysRequest) (*hsmv1.ListKeysResponse, error) {
	return nil, ErrNotImplemented
}

// Close logs out and finalizes the PKCS#11 module.
func (b *Backend) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil
	}
	b.closed = true
	var firstErr error
	if err := b.ctx.Logout(b.session); err != nil {
		firstErr = err
	}
	if err := b.ctx.CloseSession(b.session); err != nil && firstErr == nil {
		firstErr = err
	}
	if err := b.ctx.Finalize(); err != nil && firstErr == nil {
		firstErr = err
	}
	b.ctx.Destroy()
	return firstErr
}

// Compile-time check that the skeleton matches the broker contract.
var _ broker.Broker = (*Backend)(nil)
