# 0003. PKCS#11 HSM abstraction

- Status: Accepted
- Date: 2026-05-09
- Authors: Ameed Jamous <a.jamous@telecomsxchange.com>

## Context

A SAS-SM-accredited SM-DP+ keeps its private keys in a hardware
security module. The HSM market is fragmented: AWS CloudHSM, GCP Cloud
HSM, Azure Key Vault Managed HSM, Thales Luna SA, Utimaco
SecurityServer, and several smaller vendors all serve carriers.

For lab use, no real HSM is available. SoftHSM v2 is the de facto
"HSM emulator" for development.

We need an interface that:

- Works with all of the above
- Never returns private key material to the caller
- Lets us swap backends with a config change, not a code change
- Doesn't lock us into any one vendor's quirks

## Decision

All cryptographic operations involving long-lived private keys go
through a single `services/hsm-broker` daemon that exposes a small
gRPC interface and uses PKCS#11 v2.40 to talk to the underlying HSM.
The broker accepts any PKCS#11-conforming module configured at
startup.

Lab default: SoftHSM v2. Production backends: AWS CloudHSM, GCP Cloud
HSM, Azure Managed HSM, Thales Luna, Utimaco. All of them speak
PKCS#11.

The broker exposes:

- `Sign(keyRef, data) -> signature`
- `Decrypt(keyRef, ciphertext) -> plaintext`
- `DeriveKey(keyRef, peerPub, kdfParams) -> sessionKeyRef`
- `GenerateKeyPair(spec) -> keyRef`
- `ListKeys(filter) -> [keyMetadata]`
- `Health()`

Note: there is no `Export`. Private key material never leaves the
HSM, and never crosses the broker's gRPC boundary. Session keys
derived inside the HSM are referenced by handle; if a service needs
the raw bytes (e.g., for AES-GCM via a Go library), it must perform
that operation inside the broker via `Encrypt`/`Decrypt`/`Mac` calls.

## Alternatives considered

**Direct PKCS#11 from each service.** Every service that needs
crypto links `github.com/miekg/pkcs11` and talks to the HSM directly.
Rejected: connection management, session limits, and key handle
caching become per-service problems. Auditing what reached the HSM
becomes harder. PKCS#11 sessions are a precious resource on real
HSMs; centralizing helps.

**Per-vendor SDK abstraction.** Use AWS KMS API on AWS, Azure Key
Vault SDK on Azure, etc., behind a shared interface. Rejected: KMS
APIs differ in what they can do. ECKA in particular is poorly
supported across cloud KMS APIs. PKCS#11 is the lowest common
denominator that actually does what we need. Cloud HSM products all
ship a PKCS#11 library precisely because the industry needs that.

**KMIP.** A standard but heavyweight key management protocol.
Excellent for enterprise key lifecycle, less common in the HSM
operations path, and no real benefit over PKCS#11 for our use cases.

## Consequences

Positive:

- Any PKCS#11 HSM can be plugged in with a config change
- A single audit point for every crypto operation
- A single place to add metrics, tracing, retries, and connection pools
- Clean separation of concerns: services know nothing about which HSM
  they're talking to

Negative:

- An additional network hop for every crypto operation. Mitigated
  with co-located deployment (broker as a sidecar) and short-lived
  authenticated UDS or in-cluster mTLS connections.
- PKCS#11 is C-shaped; the Go cgo binding (`miekg/pkcs11`) is solid
  but not idiomatic Go. The broker hides this from everyone else.
- Some HSMs implement PKCS#11 attributes differently. We will need a
  per-vendor compatibility matrix in `docs/operations/hsm/` over time.

## References

- OASIS PKCS#11 v2.40 specification
- SoftHSM v2: https://www.opendnssec.org/softhsm/
- `github.com/miekg/pkcs11`
- AWS CloudHSM PKCS#11 library docs
- Thales Luna PKCS#11 library docs
