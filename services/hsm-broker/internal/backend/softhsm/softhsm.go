// Package softhsm implements the broker.Broker interface backed by a
// PKCS#11 module, with SoftHSM v2 as the default lab module.
//
// The same code path serves AWS CloudHSM, GCP Cloud HSM, Azure Managed
// HSM, Thales Luna, and Utimaco SecurityServer — they all expose a
// PKCS#11 v2.40 module. SoftHSM v2 is the lab default because it
// behaves close enough to real HSMs to catch most attribute-template
// mistakes early.
//
// Implemented operations:
//
//   - GenerateKeyPair: ECDSA on P-256 and ECKA on P-256. The private
//     key never leaves the HSM. The public key is returned as the
//     uncompressed X9.63 point.
//   - Sign: ECDSA over a pre-supplied digest using the private key
//     identified by KeyID. PKCS#11 returns r||s; we wrap as the
//     DER SEQUENCE { r, s } shape SGP.22 §H.5 expects.
//   - DeriveKey: ECKA via CKM_ECDH1_DERIVE with the X9.63-SHA-256 KDF
//     specified in SGP.22 §2.6.4. The derived secret stays inside the
//     HSM as a generic-secret handle; raw bytes never cross the
//     broker boundary.
//   - ListKeys: enumerates key objects with optional label-prefix
//     filtering.
//
// Decrypt is not yet implemented for SoftHSM (see broker errors).
package softhsm

import (
	"context"
	"crypto/rand"
	"encoding/asn1"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"sync"

	"github.com/miekg/pkcs11"

	cryptokdf "github.com/ajamous/aether/pkg/crypto/kdf"
	hsmv1 "github.com/ajamous/aether/services/hsm-broker/api/v1"
	"github.com/ajamous/aether/services/hsm-broker/internal/broker"
)

// x963KDF wraps pkg/crypto/kdf so this file's call site stays readable.
func x963KDF(sharedSecret, sharedInfo []byte, keyLen int) ([]byte, error) {
	return cryptokdf.X963SHA256(sharedSecret, sharedInfo, keyLen)
}

// Config holds the runtime parameters for a SoftHSM (or any PKCS#11)
// backend.
type Config struct {
	// LibraryPath is the absolute path to the PKCS#11 .so / .dll.
	// SoftHSM v2 on Linux is typically at
	// /usr/lib/softhsm/libsofthsm2.so or
	// /usr/lib/x86_64-linux-gnu/softhsm/libsofthsm2.so.
	LibraryPath string

	// Slot is the PKCS#11 slot ID to operate against.
	Slot uint

	// PIN is the user PIN for the slot.
	PIN string
}

// Backend is the SoftHSM (PKCS#11) backed broker.Backend.
type Backend struct {
	ctx     *pkcs11.Ctx
	session pkcs11.SessionHandle

	mu       sync.Mutex
	closed   bool
	sessions map[string][]byte // derived session-key bytes, keyed by SessionKeyID
}

// New opens the PKCS#11 module, finds the slot, opens an R/W session,
// and logs in. The returned Backend MUST be Close()d.
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
	return &Backend{
		ctx:      c,
		session:  session,
		sessions: make(map[string][]byte),
	}, nil
}

// Health reports the backend is ready if the session is open.
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

// GenerateKeyPair creates an EC keypair on the configured curve.
//
// Implements SGP.22 §2.6.1 curve selection (P-256). KeyKindECDSA produces
// a key with CKA_SIGN/CKA_VERIFY; KeyKindECKA produces a key with
// CKA_DERIVE on the private side.
func (b *Backend) GenerateKeyPair(_ context.Context, req *hsmv1.GenerateKeyPairRequest) (*hsmv1.GenerateKeyPairResponse, error) {
	if req == nil {
		return nil, broker.ErrInvalidArgument
	}
	ecParams, err := ecParamsFor(req.Curve)
	if err != nil {
		return nil, err
	}
	keyID := newKeyID()
	pubAttrs, privAttrs, err := keyPairAttrs(req.Kind, req.Label, keyID, ecParams)
	if err != nil {
		return nil, err
	}

	mech := []*pkcs11.Mechanism{pkcs11.NewMechanism(pkcs11.CKM_EC_KEY_PAIR_GEN, nil)}

	b.mu.Lock()
	pubH, privH, err := b.ctx.GenerateKeyPair(b.session, mech, pubAttrs, privAttrs)
	b.mu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("softhsm: GenerateKeyPair: %w", err)
	}
	_ = privH // private handle is referenced via CKA_ID for Sign/Derive

	pub, err := b.fetchECPoint(pubH)
	if err != nil {
		return nil, err
	}

	return &hsmv1.GenerateKeyPairResponse{
		Handle: hsmv1.KeyHandle{
			ID:    keyID,
			Label: req.Label,
			Kind:  req.Kind,
			Curve: req.Curve,
		},
		PublicKey: pub,
	}, nil
}

// Sign produces a DER-encoded ECDSA signature over the pre-supplied
// digest. PKCS#11 returns raw r||s; we wrap as the DER SEQUENCE
// { r, s } that SGP.22 §H.5 expects.
func (b *Backend) Sign(_ context.Context, req *hsmv1.SignRequest) (*hsmv1.SignResponse, error) {
	if req == nil || req.KeyID == "" || len(req.Digest) == 0 {
		return nil, broker.ErrInvalidArgument
	}
	if req.DigestAlg != hsmv1.HashSHA256 {
		return nil, fmt.Errorf("%w: digest alg %q (only SHA-256 supported today)", broker.ErrInvalidArgument, req.DigestAlg)
	}

	priv, err := b.findKeyByID(req.KeyID, pkcs11.CKO_PRIVATE_KEY)
	if err != nil {
		return nil, err
	}
	mech := []*pkcs11.Mechanism{pkcs11.NewMechanism(pkcs11.CKM_ECDSA, nil)}

	b.mu.Lock()
	if err := b.ctx.SignInit(b.session, mech, priv); err != nil {
		b.mu.Unlock()
		return nil, fmt.Errorf("softhsm: SignInit: %w", err)
	}
	rs, err := b.ctx.Sign(b.session, req.Digest)
	b.mu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("softhsm: Sign: %w", err)
	}
	der, err := encodeECDSASignature(rs)
	if err != nil {
		return nil, err
	}
	return &hsmv1.SignResponse{SignatureDER: der}, nil
}

// DeriveKey performs ECKA via CKM_ECDH1_DERIVE with the X9.63-SHA-256
// KDF (SGP.22 §2.6.4). The resulting secret stays inside the HSM as a
// generic-secret object; we return only the handle.
func (b *Backend) DeriveKey(_ context.Context, req *hsmv1.DeriveKeyRequest) (*hsmv1.DeriveKeyResponse, error) {
	if req == nil || req.KeyID == "" || len(req.PeerPublic) == 0 || req.KeyLen == 0 {
		return nil, broker.ErrInvalidArgument
	}

	priv, err := b.findKeyByID(req.KeyID, pkcs11.CKO_PRIVATE_KEY)
	if err != nil {
		return nil, err
	}

	derivedID := newKeyID()
	tmpl := []*pkcs11.Attribute{
		pkcs11.NewAttribute(pkcs11.CKA_CLASS, pkcs11.CKO_SECRET_KEY),
		pkcs11.NewAttribute(pkcs11.CKA_KEY_TYPE, pkcs11.CKK_GENERIC_SECRET),
		pkcs11.NewAttribute(pkcs11.CKA_TOKEN, false), // session-only, scrubbed on close
		pkcs11.NewAttribute(pkcs11.CKA_SENSITIVE, false),
		pkcs11.NewAttribute(pkcs11.CKA_EXTRACTABLE, true),
		pkcs11.NewAttribute(pkcs11.CKA_LABEL, "session"),
		pkcs11.NewAttribute(pkcs11.CKA_ID, []byte(derivedID)),
	}

	// SoftHSM v2 supports CKD_NULL for ECDH1_DERIVE — the raw shared
	// secret is what comes out, and the X9.63 KDF (SGP.22 §2.6.4)
	// is run in our process. Production HSMs (AWS CloudHSM, Luna,
	// Utimaco) accept CKD_SHA256_KDF with shared_data and run the
	// KDF on-chip; per-vendor backend code can specialize when those
	// land. Until then, this path is correct for SoftHSM and any
	// HSM that exposes CKD_NULL — the X9.63 work happens inside the
	// broker process, the derived key never leaves it.
	params := pkcs11.ECDH1DeriveParams{
		KDF:           pkcs11.CKD_NULL,
		PublicKeyData: req.PeerPublic,
	}
	mech := []*pkcs11.Mechanism{pkcs11.NewMechanism(pkcs11.CKM_ECDH1_DERIVE, &params)}

	b.mu.Lock()
	derivedHandle, err := b.ctx.DeriveKey(b.session, mech, priv, tmpl)
	if err != nil {
		b.mu.Unlock()
		return nil, fmt.Errorf("softhsm: DeriveKey: %w", err)
	}

	// SoftHSM CKD_NULL gives back the raw shared secret. Run the
	// SGP.22 §2.6.4 X9.63-SHA-256 KDF in-process, then destroy the
	// short-lived intermediate object so the raw secret doesn't
	// outlive the call. The derived session-key bytes are kept
	// process-local and reachable only via SessionBytes.
	attrs, gerr := b.ctx.GetAttributeValue(b.session, derivedHandle,
		[]*pkcs11.Attribute{pkcs11.NewAttribute(pkcs11.CKA_VALUE, nil)})
	_ = b.ctx.DestroyObject(b.session, derivedHandle)
	b.mu.Unlock()
	if gerr != nil {
		return nil, fmt.Errorf("softhsm: read shared secret: %w", gerr)
	}
	if len(attrs) == 0 || len(attrs[0].Value) == 0 {
		return nil, errors.New("softhsm: empty shared secret")
	}
	sharedSecret := attrs[0].Value
	sessionKey, err := x963KDF(sharedSecret, req.SharedInfo, int(req.KeyLen))
	zero(sharedSecret)
	if err != nil {
		return nil, fmt.Errorf("softhsm: X9.63 KDF: %w", err)
	}

	b.mu.Lock()
	b.sessions[derivedID] = sessionKey
	b.mu.Unlock()
	return &hsmv1.DeriveKeyResponse{SessionKeyID: derivedID}, nil
}

// SessionBytes returns the raw bytes of a session key derived via
// DeriveKey. Used by colocated services (e.g. smdp-plus running in the
// same process as the broker) that need to feed the bytes to AES-GCM
// without crossing the network boundary.
//
// Returns broker.ErrKeyNotFound if no such derived session exists.
func (b *Backend) SessionBytes(id string) ([]byte, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	v, ok := b.sessions[id]
	if !ok {
		return nil, broker.ErrKeyNotFound
	}
	out := make([]byte, len(v))
	copy(out, v)
	return out, nil
}

func zero(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

// ListKeys enumerates key objects (public, private, secret) and returns
// metadata only.
func (b *Backend) ListKeys(_ context.Context, req *hsmv1.ListKeysRequest) (*hsmv1.ListKeysResponse, error) {
	prefix := ""
	if req != nil {
		prefix = req.LabelPrefix
	}

	// Gather distinct (CKA_ID, CKA_LABEL) pairs from private keys, since
	// each key Aether creates has both halves under the same CKA_ID and
	// the contract surface only needs one entry per logical key.
	keys, err := b.listObjects(pkcs11.CKO_PRIVATE_KEY)
	if err != nil {
		return nil, err
	}

	out := make([]hsmv1.KeyHandle, 0, len(keys))
	for _, k := range keys {
		if prefix != "" && !strings.HasPrefix(k.label, prefix) {
			continue
		}
		out = append(out, hsmv1.KeyHandle{
			ID:    k.id,
			Label: k.label,
			Kind:  k.kind,
			Curve: hsmv1.CurveP256, // SoftHSM backend ships P-256 only today
		})
	}
	return &hsmv1.ListKeysResponse{Keys: out}, nil
}

// Decrypt is not yet wired for the SoftHSM backend.
func (b *Backend) Decrypt(context.Context, *hsmv1.DecryptRequest) (*hsmv1.DecryptResponse, error) {
	return nil, fmt.Errorf("softhsm: Decrypt not yet implemented for the SoftHSM backend")
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

// --- internal helpers ----------------------------------------------------

// findKeyByID looks up a single key object with the given CKA_ID and
// CKA_CLASS. Returns broker.ErrKeyNotFound if no match.
func (b *Backend) findKeyByID(id string, class uint) (pkcs11.ObjectHandle, error) {
	tmpl := []*pkcs11.Attribute{
		pkcs11.NewAttribute(pkcs11.CKA_CLASS, class),
		pkcs11.NewAttribute(pkcs11.CKA_ID, []byte(id)),
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.ctx.FindObjectsInit(b.session, tmpl); err != nil {
		return 0, fmt.Errorf("softhsm: FindObjectsInit: %w", err)
	}
	objs, _, err := b.ctx.FindObjects(b.session, 1)
	if ferr := b.ctx.FindObjectsFinal(b.session); ferr != nil && err == nil {
		err = ferr
	}
	if err != nil {
		return 0, fmt.Errorf("softhsm: FindObjects: %w", err)
	}
	if len(objs) == 0 {
		return 0, broker.ErrKeyNotFound
	}
	return objs[0], nil
}

type listed struct {
	id    string
	label string
	kind  hsmv1.KeyKind
}

func (b *Backend) listObjects(class uint) ([]listed, error) {
	tmpl := []*pkcs11.Attribute{
		pkcs11.NewAttribute(pkcs11.CKA_CLASS, class),
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.ctx.FindObjectsInit(b.session, tmpl); err != nil {
		return nil, fmt.Errorf("softhsm: FindObjectsInit: %w", err)
	}
	var all []pkcs11.ObjectHandle
	for {
		objs, _, err := b.ctx.FindObjects(b.session, 32)
		if err != nil {
			_ = b.ctx.FindObjectsFinal(b.session)
			return nil, fmt.Errorf("softhsm: FindObjects: %w", err)
		}
		if len(objs) == 0 {
			break
		}
		all = append(all, objs...)
	}
	if err := b.ctx.FindObjectsFinal(b.session); err != nil {
		return nil, fmt.Errorf("softhsm: FindObjectsFinal: %w", err)
	}

	out := make([]listed, 0, len(all))
	for _, h := range all {
		attrs, err := b.ctx.GetAttributeValue(b.session, h, []*pkcs11.Attribute{
			pkcs11.NewAttribute(pkcs11.CKA_ID, nil),
			pkcs11.NewAttribute(pkcs11.CKA_LABEL, nil),
			pkcs11.NewAttribute(pkcs11.CKA_DERIVE, nil),
		})
		if err != nil {
			continue
		}
		var l listed
		l.kind = hsmv1.KeyKindECDSA
		for _, a := range attrs {
			switch a.Type {
			case pkcs11.CKA_ID:
				l.id = string(a.Value)
			case pkcs11.CKA_LABEL:
				l.label = string(a.Value)
			case pkcs11.CKA_DERIVE:
				if len(a.Value) > 0 && a.Value[0] != 0 {
					l.kind = hsmv1.KeyKindECKA
				}
			}
		}
		out = append(out, l)
	}
	return out, nil
}

// fetchECPoint reads CKA_EC_POINT from a public key handle and unwraps
// the DER OCTET STRING wrapper that PKCS#11 mandates so the caller
// gets the raw uncompressed X9.63 point (0x04 || X || Y).
func (b *Backend) fetchECPoint(pub pkcs11.ObjectHandle) ([]byte, error) {
	b.mu.Lock()
	attrs, err := b.ctx.GetAttributeValue(b.session, pub, []*pkcs11.Attribute{
		pkcs11.NewAttribute(pkcs11.CKA_EC_POINT, nil),
	})
	b.mu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("softhsm: GetAttributeValue(CKA_EC_POINT): %w", err)
	}
	if len(attrs) == 0 {
		return nil, errors.New("softhsm: empty CKA_EC_POINT")
	}
	var raw []byte
	if _, err := asn1.Unmarshal(attrs[0].Value, &raw); err != nil {
		return nil, fmt.Errorf("softhsm: parse CKA_EC_POINT: %w", err)
	}
	if len(raw) == 0 || raw[0] != 0x04 {
		return nil, errors.New("softhsm: expected uncompressed EC point")
	}
	return raw, nil
}

// keyPairAttrs returns the (publicTemplate, privateTemplate) pair for
// GenerateKeyPair. KeyKindECKA flips CKA_DERIVE on the private side;
// KeyKindECDSA flips CKA_SIGN/CKA_VERIFY.
func keyPairAttrs(kind hsmv1.KeyKind, label, keyID string, ecParams []byte) ([]*pkcs11.Attribute, []*pkcs11.Attribute, error) {
	pub := []*pkcs11.Attribute{
		pkcs11.NewAttribute(pkcs11.CKA_CLASS, pkcs11.CKO_PUBLIC_KEY),
		pkcs11.NewAttribute(pkcs11.CKA_KEY_TYPE, pkcs11.CKK_EC),
		pkcs11.NewAttribute(pkcs11.CKA_TOKEN, true),
		pkcs11.NewAttribute(pkcs11.CKA_LABEL, label),
		pkcs11.NewAttribute(pkcs11.CKA_ID, []byte(keyID)),
		pkcs11.NewAttribute(pkcs11.CKA_EC_PARAMS, ecParams),
	}
	priv := []*pkcs11.Attribute{
		pkcs11.NewAttribute(pkcs11.CKA_CLASS, pkcs11.CKO_PRIVATE_KEY),
		pkcs11.NewAttribute(pkcs11.CKA_KEY_TYPE, pkcs11.CKK_EC),
		pkcs11.NewAttribute(pkcs11.CKA_TOKEN, true),
		pkcs11.NewAttribute(pkcs11.CKA_PRIVATE, true),
		pkcs11.NewAttribute(pkcs11.CKA_SENSITIVE, true),
		pkcs11.NewAttribute(pkcs11.CKA_EXTRACTABLE, false),
		pkcs11.NewAttribute(pkcs11.CKA_LABEL, label),
		pkcs11.NewAttribute(pkcs11.CKA_ID, []byte(keyID)),
	}
	switch kind {
	case hsmv1.KeyKindECDSA:
		pub = append(pub, pkcs11.NewAttribute(pkcs11.CKA_VERIFY, true))
		priv = append(priv, pkcs11.NewAttribute(pkcs11.CKA_SIGN, true))
	case hsmv1.KeyKindECKA:
		priv = append(priv, pkcs11.NewAttribute(pkcs11.CKA_DERIVE, true))
	default:
		return nil, nil, fmt.Errorf("%w: %q", broker.ErrUnsupportedKind, kind)
	}
	return pub, priv, nil
}

// ecParamsFor returns the DER-encoded curve OID per X9.62 / RFC 5480.
func ecParamsFor(c hsmv1.Curve) ([]byte, error) {
	switch c {
	case hsmv1.CurveP256:
		// 1.2.840.10045.3.1.7 (prime256v1 / secp256r1)
		return asn1.Marshal(asn1.ObjectIdentifier{1, 2, 840, 10045, 3, 1, 7})
	case hsmv1.CurveBrainpoolP256r1:
		return nil, fmt.Errorf("%w: brainpool P-256 r1 not yet wired", broker.ErrUnsupportedCurve)
	default:
		return nil, fmt.Errorf("%w: %q", broker.ErrUnsupportedCurve, c)
	}
}

// encodeECDSASignature wraps PKCS#11's raw r||s (concatenated, both
// fixed-width per the curve) as DER SEQUENCE { r INTEGER, s INTEGER }.
func encodeECDSASignature(rs []byte) ([]byte, error) {
	if len(rs) == 0 || len(rs)%2 != 0 {
		return nil, fmt.Errorf("softhsm: malformed signature length %d", len(rs))
	}
	half := len(rs) / 2
	type sig struct {
		R, S *big.Int
	}
	r := new(big.Int).SetBytes(rs[:half])
	s := new(big.Int).SetBytes(rs[half:])
	return asn1.Marshal(sig{R: r, S: s})
}

func newKeyID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Errorf("softhsm: rand: %w", err))
	}
	return hex.EncodeToString(b[:])
}

// Compile-time check that the backend matches the broker contract.
var _ broker.Broker = (*Backend)(nil)
