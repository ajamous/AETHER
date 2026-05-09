# services/smdp-plus

Aether's SM-DP+ service. Implements the server side of GSMA SGP.22's
Consumer Remote SIM Provisioning protocol: ES9+ on the wire to the
LPA, ES8+ application-protocol payloads carried inside, ES2+
exposed by the gateway for upstream BSS.

## Status

| Piece                                      | Status        |
| ------------------------------------------ | ------------- |
| HTTPS server, mTLS-ready                   | Implemented   |
| ES9+ `initiateAuthentication` (§5.6.2)     | Skeleton (correct shape, no eUICC verification yet) |
| ES9+ `authenticateClient` (§5.6.3)         | Skeleton      |
| ES9+ `getBoundProfilePackage` (§5.6.4)     | Skeleton      |
| ES9+ `handleNotification` (§5.6.5)         | Skeleton      |
| In-memory session store                    | Implemented   |
| Redis session store                        | Not started   |
| BPP generation (UPP → PPP → BPP)           | Not started — depends on SAIP codec + real ASN.1 modules |
| State machine (Annex A)                    | Skeleton      |
| Hookup to certmgr / hsm-broker             | Wired (HTTP clients present; protocol use lands with BPP) |

The skeleton endpoints accept and return JSON with the SGP.22
field shapes. The actual cryptographic protocol — eUICC challenge
verification, ECDSA over the SM-DP+ identity chain, BPP encryption
with ECKA-derived session keys — lands in a focused PR alongside
the spec-faithful ASN.1 types and SGP.26 conformance vectors.

This is deliberate. A pretend-ES9+ that returns the right HTTP shape
but skips signature verification would be more dangerous than an
honest skeleton that returns NotImplemented from the protocol
methods. We refuse to ship that pretence.

## Running

```
go run ./cmd/smdp-plus \
    --listen=:8443 \
    --certmgr=http://localhost:8444 \
    --hsm-broker=http://localhost:8445 \
    --tls-cert=./testdata/dp-tls.pem \
    --tls-key=./testdata/dp-tls.key
```

The lab Docker Compose passes the right URLs and certs.
