// Package hsmv1 holds the Go types for the HSM broker v1 API.
//
// These types mirror api/v1/hsm.proto 1:1. When the gRPC build is
// wired in, generated Go types replace this file; until then, callers
// import these for both the HTTP+JSON server and clients.
package hsmv1

type Curve string

const (
	CurveUnspecified     Curve = ""
	CurveP256            Curve = "P256"
	CurveBrainpoolP256r1 Curve = "BRAINPOOL_P256_R1"
)

type HashAlg string

const (
	HashUnspecified HashAlg = ""
	HashSHA256      HashAlg = "SHA256"
	HashSHA384      HashAlg = "SHA384"
	HashSHA512      HashAlg = "SHA512"
)

type KeyKind string

const (
	KeyKindUnspecified KeyKind = ""
	KeyKindECDSA       KeyKind = "ECDSA"
	KeyKindECKA        KeyKind = "ECKA"
)

type KeyHandle struct {
	ID    string  `json:"id"`
	Label string  `json:"label"`
	Kind  KeyKind `json:"kind"`
	Curve Curve   `json:"curve"`
}

type SignRequest struct {
	KeyID     string  `json:"key_id"`
	Digest    []byte  `json:"digest"`
	DigestAlg HashAlg `json:"digest_alg"`
}

type SignResponse struct {
	SignatureDER []byte `json:"signature_der"`
}

type DecryptRequest struct {
	KeyID      string `json:"key_id"`
	Ciphertext []byte `json:"ciphertext"`
}

type DecryptResponse struct {
	Plaintext []byte `json:"plaintext"`
}

type DeriveKeyRequest struct {
	KeyID      string `json:"key_id"`
	PeerPublic []byte `json:"peer_public"`
	SharedInfo []byte `json:"shared_info"`
	KeyLen     uint32 `json:"key_len"`
}

type DeriveKeyResponse struct {
	SessionKeyID string `json:"session_key_id"`
}

type GenerateKeyPairRequest struct {
	Label string  `json:"label"`
	Kind  KeyKind `json:"kind"`
	Curve Curve   `json:"curve"`
}

type GenerateKeyPairResponse struct {
	Handle    KeyHandle `json:"handle"`
	PublicKey []byte    `json:"public_key"`
}

type ListKeysRequest struct {
	LabelPrefix string `json:"label_prefix"`
}

type ListKeysResponse struct {
	Keys []KeyHandle `json:"keys"`
}

type HealthRequest struct{}

type HealthResponse struct {
	Ready           bool   `json:"ready"`
	Backend         string `json:"backend"`
	ActiveSessions  uint32 `json:"active_sessions"`
}
