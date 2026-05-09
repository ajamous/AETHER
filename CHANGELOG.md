# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

Foundation:
- Repository bootstrap: Apache 2.0 license, README with honest status
  table, CONTRIBUTING, CODE_OF_CONDUCT, SECURITY, GOVERNANCE,
  MAINTAINERS, ROADMAP.
- Makefile with build/test/lint/lab-up/lab-down/lab-test/gen targets,
  per-module iteration, graceful tool-missing diagnostics.
- Go workspace (`go.work`).
- Linter and formatter configs.
- GitHub Actions: ci, release, codeql.
- Issue and pull request templates.
- Documentation skeleton (MkDocs Material), ADRs 0001-0005.
- ASN.1 toolchain scaffolding under `pkg/asn1/sgp22/` with starter
  PEHeader round-trip test.

Crypto primitives (`pkg/crypto`):
- ECDSA P-256 sign/verify with SHA-256 (DER signature shape).
- ECKA over P-256 with X9.63-SHA-256 KDF.
- X9.63 KDF tested against a NIST CAVP vector.
- AES-128-GCM BSP wrappers.
- Brainpool P-256 r1 stubbed pending dependency-vetted curve.

Services (`services/`):
- `hsm-broker`: PKCS#11 façade. Memory and SoftHSM backends both
  implement Sign / GenerateKeyPair / DeriveKey / ListKeys. SoftHSM
  backend exercises real PKCS#11 calls (`CKM_EC_KEY_PAIR_GEN`,
  `CKM_ECDSA`, `CKM_ECDH1_DERIVE` with `CKD_NULL`) and is verified by
  an integration test in CI that installs SoftHSM v2 and round-trips
  a signature against a Go-side ECDSA verify. The X9.63-SHA-256 KDF
  runs in-process on SoftHSM (which doesn't expose `CKD_SHA256_KDF`)
  and the intermediate shared secret is zeroed and destroyed.
  HTTP+JSON server with RFC 7807 errors. Proto contract in
  `api/v1/hsm.proto`.
- `certmgr`: cert chain loading and verification, lab and production
  modes (per ADR 0004), expiry Prometheus metrics, lab-chain
  generator for tests and the local lab.
- `smdp-plus`: ES9+ endpoint skeletons at the SGP.22 paths, session
  state machine, in-memory session store. BPP generation honestly
  returns 501 until the SAIP codec lands.
- `profile-builder`: YAML profile template loader with validation,
  UPP envelope (replaced by SAIP-encoded UPP when the codec lands).
- `gateway`: single entry point. ES2+ skeletons at the SGP.22 paths,
  REST proxies to profile-builder and certmgr.
- `audit`: hash-chained append-only ledger. Append, list, get, verify,
  with tamper detection. In-memory and Postgres-backed stores both
  ship; the Postgres path uses serializable transactions with
  retry on 40001 so concurrent appends keep the chain intact.
  `payload` is stored as `BYTEA` (not `JSONB`) because the hash is
  over exact bytes — JSONB normalisation breaks the chain on
  round-trip. Verified with an 8-writer × 25-append-each concurrent
  test.
- `smds`: Subscription Manager — Discovery Service (Phase 3).
  ES12 RegisterEvent / DeleteEvent for SM-DP+, ES11 AuthenticateClient
  / GetEvents for the LPA. In-memory and Postgres-backed event stores
  with idempotent registration via `(eid, event_id)` primary key and
  `ON CONFLICT DO UPDATE`. Full discovery handshake exercised
  end-to-end in tests. Gateway proxies the admin /v1/events surface
  for the UI.

Persistence (Phase 1 follow-up):
- `services/smdp-plus` gains a Postgres-backed session store with
  background TTL eviction. The HTTP server now accepts the `Store`
  interface; the in-memory store stays the test default.
- All three persistence backends (audit ledger, SM-DS events,
  SM-DP+ sessions) share the same opt-in pattern: empty `--pg-url`
  uses in-memory; setting it (or `AETHER_PG_URL`) flips to Postgres
  with the schema applied at startup.
- `deployments/docker-compose/lab.yml` wires the three services to
  the existing Postgres container so the lab now runs against real
  persistence by default.
- New `postgres-integration` job in `.github/workflows/ci.yml`
  brings up a Postgres service container and runs the integration
  tests for all three backends on every PR.

Helm chart (`deployments/helm/aether/`):
- Single chart deploys all backend services + admin UI + an optional
  bundled Postgres StatefulSet with PVC.
- Both install paths render and lint clean: lab defaults
  (`helm install aether ./aether`) and production override
  (external Postgres, SoftHSM/external PKCS#11, production cert
  trust store, ingress with TLS).
- Sensible production-grade defaults: non-root pod security context,
  read-only root filesystem, capabilities dropped, dedicated
  ServiceAccount, resource requests/limits per service, Prometheus
  scrape annotations, readiness and liveness probes against
  `/v1/health`.
- Ingress template routes `/gsma/...` and `/v1/...` to the gateway
  and `/` to the admin UI.
- README documents the production override values, the lab
  cert-chain manual step (until the cert-init Job lands), and the
  explicit list of things the chart deliberately does NOT do
  (generate production keys, configure ingress controller, do
  Postgres backups, configure operator RBAC).
- New `helm` job in `.github/workflows/ci.yml` runs `helm lint`
  and renders both lab and production templates on every PR.

Admin UI (`ui/admin/`):
- Next.js 15 (App Router), React 18, Tailwind CSS, TypeScript strict.
- Read-only operator console: dashboard, profile templates,
  certificates, audit log, about page.
- Server-side data fetching only — the browser never talks to
  backend services directly. Helpers in `lib/api.ts`.
- Standalone-mode build, multi-stage Dockerfile.
- No authentication today; lab use only.

Lab and tests:
- Docker Compose at `deployments/docker-compose/lab.yml` brings up
  Postgres, Redis, NATS, certgen, all six Aether services, and the
  admin UI with health-gated startup ordering.
- Lab smoke tests at `test/e2e/`, gated by `-tags=lab`, exercise the
  gateway, certmgr metrics, audit chain, and ES2+ download-order
  round trip.

[Unreleased]: https://github.com/ajamous/aether/compare/HEAD...HEAD
