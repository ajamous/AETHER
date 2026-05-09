// Package crypto holds Aether's cryptographic primitives for SGP.22
// Remote SIM Provisioning.
//
// Subpackages:
//
//   - ecdsa: ECDSA helpers over the curves SGP.22 mandates.
//   - ecka:  ECKA (Elliptic Curve Key Agreement) for SGP.22 §2.6.4 and
//     the X9.63 KDF used to derive session keys.
//   - bsp:   Bound Profile Protection wrappers, the AES-128-GCM
//     framing used for ES8+ payload protection (SGP.22 §2.6, §5.5).
//   - kdf:   The X9.63 KDF-SHA-256 used by ECKA, factored out so it
//     can be tested independently.
//
// Where the GSMA spec mandates a specific algorithm or constraint, the
// relevant Go function carries a comment naming the spec section. This
// is so the codebase reads as a reference implementation, not a
// black box.
//
// What is not in this package: PKCS#11 access (lives in services/hsm-broker),
// X.509 certificate handling beyond the helpers used here (lives in
// services/certmgr), and high-level RSP session state (lives in
// services/smdp-plus).
package crypto
