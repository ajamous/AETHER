// Package bpp holds the SM-DP+ side of the SGP.22 §5.6.4 / §H.3
// Bound Profile Package construction. Today this package ships only
// the session-key derivation step (ECKA + X9.63 KDF, sliced into
// SENC / SMAC / initialMCV per §H.3); the surrounding BPP framing
// (segmentation, AES-128-GCM per segment, the
// InitialiseSecureChannelRequest signed with DPpb) lands in
// follow-up PRs that build on this file.
//
// Why a separate package:
//
//   - Session-key derivation is pure crypto — it depends only on
//     pkg/crypto/ecka. Keeping it out of services/smdp-plus/internal
//     /server lets unit tests exercise the derivation without
//     spinning up an HTTP server or a session store.
//   - The labelling (SENC / SMAC / MCV split) is Aether-specific
//     glue between the spec-mandated KDF output (which is just
//     bytes) and the named slots SCP03t talks in. Putting that
//     labelling beside services/smdp-plus's other BPP machinery
//     keeps the import graph one-way: bpp → pkg/crypto, never the
//     reverse.
//
// SGP.22 §H.3 cross-reference: the KDF runs over the ECDH shared
// secret with a sharedInfo string the spec defines in §H.4. We
// take sharedInfo as caller-supplied bytes so this package is
// agnostic to which SCP variant (SCP03t today; future SCP81 etc.)
// the surrounding flow chose.
package bpp

import (
	"crypto/ecdh"
	"errors"
	"fmt"

	"github.com/ajamous/aether/pkg/crypto/ecka"
)

// SessionKeys carries the three labelled byte slices SCP03t derives
// from one ECKA exchange. All three are 16-byte slices on AES-128.
//
//   - SENC: payload-encryption key for AES-128-GCM. Used to seal
//     each ProtectedProfilePackage segment.
//   - SMAC: payload-authentication key for the per-segment GMAC.
//     SGP.22 §2.6.3 uses GCM with full-length tag, so SMAC is
//     conceptually distinct from SENC even when the same AES
//     primitive consumes both — separate for ceremony hygiene
//     when one or the other gets compromised.
//   - InitialMCV: the seed for the MAC chaining vector that links
//     consecutive segments. Each segment's MAC is computed over
//     (MCV || segment); MCV is updated after each successful seal
//     to the previous segment's MAC.
type SessionKeys struct {
	SENC       []byte
	SMAC       []byte
	InitialMCV []byte
}

// SessionKeyLen is the total bytes of derived material BPP needs.
// 16 for SENC + 16 for SMAC + 16 for initialMCV = 48. We also
// reserve 16 trailing bytes that SCP03t's full §H.3 layout leaves
// for future use; deriving 64 bytes today keeps the slot stable
// when a future PR consumes them.
const (
	sessionKeyMaterialLen = 64
	sencLen               = 16
	smacLen               = 16
	mcvLen                = 16
)

// Derive runs ECKA + X9.63-SHA-256 KDF over (spDPpriv, euiccPub,
// sharedInfo), then slices the output into (SENC, SMAC,
// InitialMCV) per SGP.22 §H.3.
//
// sharedInfo is whatever bytes the caller's SCP variant requires;
// for SCP03t the spec has a particular tagged structure (§H.4). We
// don't construct that here — too tied to surrounding ASN.1 — but
// the caller MUST pass the same bytes the eUICC-side derivation
// uses, otherwise the two halves will not agree and every BPP
// segment will fail GCM authentication on the eUICC.
//
// Both keys MUST be P-256. ecka.Derive enforces this; passing
// Brainpool keys returns an ECKA error today.
func Derive(spDPpriv *ecdh.PrivateKey, euiccPub *ecdh.PublicKey, sharedInfo []byte) (*SessionKeys, error) {
	if spDPpriv == nil {
		return nil, errors.New("bpp: spDPpriv is nil")
	}
	if euiccPub == nil {
		return nil, errors.New("bpp: euiccPub is nil")
	}
	material, err := ecka.Derive(spDPpriv, euiccPub, sharedInfo, sessionKeyMaterialLen)
	if err != nil {
		return nil, fmt.Errorf("bpp: ECKA + KDF: %w", err)
	}
	if len(material) != sessionKeyMaterialLen {
		// Defensive — ecka.Derive's contract is exact-length, but
		// catching a violation here turns a silent BPP-decryption
		// failure on the eUICC into an obvious error in our logs.
		return nil, fmt.Errorf("bpp: derived %d bytes, want %d", len(material), sessionKeyMaterialLen)
	}
	return &SessionKeys{
		SENC:       material[0:sencLen],
		SMAC:       material[sencLen : sencLen+smacLen],
		InitialMCV: material[sencLen+smacLen : sencLen+smacLen+mcvLen],
	}, nil
}
