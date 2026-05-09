# services/certmgr

The Aether certificate manager. Loads, verifies, and monitors the SGP.22
PKI material the SM-DP+ needs to do its job: the SM-DP+ identity certs
(DPtls, DPauth, DPpb) and the trust store of GSMA Certificate Issuer
(CI) roots used to verify peer eUICCs.

See [ADR 0003](../../docs/architecture/adr/0003-pkcs11-abstraction.md)
on key custody and [ADR 0004](../../docs/architecture/adr/0004-lab-vs-prod-cert-mode.md)
on lab vs production cert modes.

## Status

| Piece                                             | Status        |
| ------------------------------------------------- | ------------- |
| Cert chain loading from PEM                        | Implemented   |
| Chain verification (DPpb/DPauth/DPtls → EUM → CI)  | Implemented   |
| Lab/production mode enforcement                    | Implemented   |
| Expiry metrics (Prometheus)                        | Implemented   |
| HTTP API (list / get / health)                     | Implemented   |
| Cert rotation API                                  | Skeleton      |
| OCSP responder integration                         | Not started   |
| HSM-resident private key handling                  | Pending hsm-broker SoftHSM integration |
| SGP.26 test root vendoring                         | Pending — see `testdata/` |

## Modes

Configured via `--mode=lab` or `--mode=production`.

**Lab.** Loads SGP.26 test CI roots from a vendored bundle. Identity
certs are SGP.26-issued test certs read from the configured paths.
Banner on startup makes it unmistakable. Production-rooted eUICCs are
rejected.

**Production.** Loads CI roots from the configured trust store path.
Identity certs are loaded from the HSM by PKCS#11 URI (handled by
hsm-broker). SGP.26-rooted eUICCs are rejected.

The two modes never mix. The service refuses to start if the certs
present don't match the configured mode.

## Endpoints

- `GET /v1/health` — readiness, including "any cert expiring within N days"
- `GET /v1/certs` — list loaded identity certs with metadata
- `GET /v1/certs/{name}` — fetch one (PEM)
- `GET /v1/trust-store` — list CI roots
- `GET /metrics` — Prometheus

Metrics:
- `aether_cert_expiry_days{name=...}` — gauge, days until notAfter
- `aether_cert_loaded{name=...,issuer=...,subject=...}` — gauge, value 1
- `aether_cert_chain_verified{name=...}` — gauge, 1 if chain verifies
- `aether_certmgr_mode{mode=...}` — gauge, 1 for the active mode

## Running

```
go run ./cmd/certmgr \
    --mode=lab \
    --trust-store=./testdata/lab/ci-roots.pem \
    --dp-tls-cert=./testdata/lab/DPtls.pem \
    --dp-auth-cert=./testdata/lab/DPauth.pem \
    --dp-pb-cert=./testdata/lab/DPpb.pem \
    --listen=:8444
```

The lab Docker Compose generates a self-signed SGP.26-style chain at
startup and points the certmgr at it; this is enough to demonstrate
the loading, verification, and expiry pipelines end-to-end without
shipping anyone else's CA material in the repo.
