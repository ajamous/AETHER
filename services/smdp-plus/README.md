# services/smdp-plus

Aether's SM-DP+ service. Implements the server side of GSMA SGP.22's
Consumer Remote SIM Provisioning protocol: ES9+ on the wire to the
LPA, ES8+ application-protocol payloads carried inside, ES2+
exposed by the gateway for upstream BSS.

## Status

| Piece                                          | Status        |
| ---------------------------------------------- | ------------- |
| HTTPS server, mTLS-ready                       | Implemented   |
| ES9+ `initiateAuthentication` (§5.6.1)         | Partial — payload built and signed end to end; eUICC challenge length enforced |
| ServerSigned1 ASN.1 + ECDSA-SHA-256 signing    | Implemented (§5.7.13 + §H.5) |
| ES9+ `authenticateClient` (§5.6.2)             | Skeleton — eUICC signature verification pending |
| ES9+ `getBoundProfilePackage` (§5.6.3)         | NotImplemented (501) — pending SAIP codec |
| ES9+ `handleNotification` (§5.6.4)             | Skeleton      |
| In-memory session store                        | Implemented   |
| Postgres-backed session store                  | Implemented (with TTL eviction) |
| Redis session store                            | Not started   |
| BPP generation (UPP → PPP → BPP)               | Not started — depends on SAIP codec + real ASN.1 modules |
| State machine (Annex A)                        | Partial       |
| Hookup to certmgr / hsm-broker                 | Wired — DPauth identity provisioned via hsm-broker on startup |

initiateAuthentication is now end-to-end signed. On startup smdp-plus
asks the hsm-broker to generate (or reference) a DPauth keypair, then
self-signs an X.509 wrapper around the public point. Each
initiateAuthentication call builds `ServerSigned1` per §5.7.13,
asks the broker to ECDSA-SHA-256 sign the digest per §H.5, and
returns the DER payload + signature + cert. A test harness can verify
the signature with stdlib ECDSA against the returned cert; this is
done in both the server unit tests and `test/e2e`.

What remains honestly NOT done:
- A real LPA verifying against GSMA CI roots will reject the
  self-signed lab cert. That is the deliberate signal "you are in
  lab mode."
- `authenticateClient` does not yet verify the eUICC signature.
- `getBoundProfilePackage` still returns 501. BPP generation needs
  the SAIP codec + spec-faithful SGP.22 ASN.1 modules; it is the
  next major Phase 1 work item.

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
