// Package broker defines the HSM broker contract.
//
// All HSM-backed cryptographic operations Aether's services need go
// through this interface. Backends implement it; the HTTP server
// invokes it; tests mock it. There is intentionally no method to
// export private key material.
package broker

import (
	"context"
	"errors"

	hsmv1 "github.com/ajamous/aether/services/hsm-broker/api/v1"
)

// Broker is the contract every backend implements.
type Broker interface {
	// Sign produces a DER-encoded ECDSA signature over a pre-computed
	// digest. PKCS#11-style: the broker never hashes on the caller's
	// behalf. SGP.22 §H.5 defines the signature shape.
	Sign(ctx context.Context, req *hsmv1.SignRequest) (*hsmv1.SignResponse, error)

	// Decrypt decrypts ciphertext using a private key kept inside the
	// HSM. Used for ES8+ payload decryption when the local end is the
	// recipient.
	Decrypt(ctx context.Context, req *hsmv1.DecryptRequest) (*hsmv1.DecryptResponse, error)

	// DeriveKey performs ECKA + X9.63 KDF (SGP.22 §2.6.4) inside the
	// HSM and returns a handle to the derived session key. The caller
	// then references that handle for subsequent symmetric ops; the
	// derived bytes never cross the broker boundary.
	DeriveKey(ctx context.Context, req *hsmv1.DeriveKeyRequest) (*hsmv1.DeriveKeyResponse, error)

	// GenerateKeyPair creates a new keypair on the configured curve.
	// Only the public key is returned to the caller.
	GenerateKeyPair(ctx context.Context, req *hsmv1.GenerateKeyPairRequest) (*hsmv1.GenerateKeyPairResponse, error)

	// ListKeys enumerates available keys (metadata only).
	ListKeys(ctx context.Context, req *hsmv1.ListKeysRequest) (*hsmv1.ListKeysResponse, error)

	// Health reports backend readiness.
	Health(ctx context.Context) (*hsmv1.HealthResponse, error)

	// Close releases any session/connection resources.
	Close() error
}

// Common errors backends should return so the HTTP layer can map them
// to consistent status codes.
var (
	ErrKeyNotFound      = errors.New("hsm: key not found")
	ErrUnsupportedCurve = errors.New("hsm: unsupported curve")
	ErrUnsupportedKind  = errors.New("hsm: unsupported key kind")
	ErrInvalidArgument  = errors.New("hsm: invalid argument")
	ErrBackendUnhealthy = errors.New("hsm: backend not ready")
)
