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
| ES9+ `authenticateClient` (§5.6.2)             | Partial — eUICC chain + signature + replay defense; outer SGP.22 SEQUENCE pending Annex B |
| EuiccSigned1 verification (§5.7.13)            | Implemented |
| ES9+ `getBoundProfilePackage` (§5.6.3)         | Implemented when DPpb configured — ECKA → SCP03t → sealed SAIP; honest 501 in lab mode |
| ES9+ `handleNotification` (§5.6.4)             | Skeleton      |
| Profile preparation (`POST /v1/profiles/prepare`) | Implemented — in-tree stand-in for ES2+ DownloadOrder; builds a UPP via profile-builder, keyed by ICCID |
| In-memory session store                        | Implemented   |
| Postgres-backed session store                  | Implemented (with TTL eviction) |
| Redis session store                            | Not started   |
| BPP generation (UPP → PPP → BPP)               | Implemented — bpp.Derive/SealSegments/AssembleBoundProfilePackage; seals a prepared profile-builder UPP, else a header-only placeholder |
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

`getBoundProfilePackage` now seals a real Bound Profile Package when a
DPpb identity is configured: it runs a fresh ECKA P-256 exchange,
derives SCP03t SENC/SMAC/MCV, seals the SAIP UPP into AES-128-GCM
segments, signs the §5.7.7 preamble with DPpb, and assembles the
`[APPLICATION 54]` wrapper. With `--profile-builder` set and a profile
prepared via `POST /v1/profiles/prepare`, the sealed UPP carries the
subscriber's PE-USIM / PE-AKAParameter credentials; otherwise a
header-only placeholder is sealed so the lab path needs no
profile-builder. The server-side unit tests decrypt the BPP with a
test eUICC key and confirm the operator's IMSI/Ki/OPc round-trip.

What remains honestly NOT done:
- A real LPA verifying against GSMA CI roots will reject the
  self-signed lab cert. That is the deliberate signal "you are in
  lab mode."
- The per-segment SCP03t AAD layout matches the spec in shape but is
  not yet cross-vendor verified against a real eUICC (hardware-bench
  follow-up).
- ES2+ DownloadOrder/ConfirmOrder in the gateway does not yet drive
  `/v1/profiles/prepare`; the prepared profile is keyed by ICCID
  directly until that lands.

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
