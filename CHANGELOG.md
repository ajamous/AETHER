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

Admin UI OIDC sign-in:
- Added Auth.js v5 (`next-auth@5.0.0-beta.31`) to `ui/admin`. Two
  modes:
    - OIDC: when `AUTH_OIDC_ISSUER`, `AUTH_OIDC_CLIENT_ID`,
      `AUTH_OIDC_CLIENT_SECRET`, and `AUTH_SECRET` are set, every
      page is gated behind a valid session; unauthenticated users
      are bounced to the IdP via `/signin`.
    - Lab bypass: when those env vars are missing, the
      `authorized` callback short-circuits to `true` and Shell.tsx
      renders an unmissable yellow "AUTH DISABLED" banner so the
      running state is obvious.
- Middleware (`middleware.ts`) protects every route except
  `/api/auth/*` and `/signin`. The matcher excludes Next.js
  internals so static assets aren't auth-gated.
- Sign-in page (`/signin`) renders a single OIDC button in OIDC
  mode and redirects back to the dashboard in lab mode.
- Sign-out button in the sidebar of every authenticated page,
  rendered alongside the logged-in user's email.
- `error.tsx` no longer wraps its message in `Shell` — Shell is
  an async server component with server-action sign-out forms,
  which can't render from inside a client component (the error
  boundary must be one).
- Helm chart: new `ui.oidc.*` values block. `issuer` is plain
  text; `clientId` and `clientSecret` come from a Secret named
  by `credentialsSecret`; the cookie-signing key comes from a
  Secret named by `authSecretName`. Optional `scopes` and
  `publicUrl` (= `AUTH_URL`) for ingress deployments. Lab default
  leaves all OIDC values empty so no AUTH_* env reaches the pod.
  Verified by `helm template` with and without OIDC values set.
- README and audit-retention.md updated: the "Operator UI sign-in
  / sign-out" row in the audit-event catalogue moves from
  "(when OIDC lands)" to "via Auth.js (OIDC delegated to your
  IdP)". SAS-SM gap analysis Personnel / Privileged-access-review
  row gains a note that admin-UI auth rides the operator's IdP.

Gateway TLS + mTLS for ES2+ (SGP.22 §5.4):
- New `services/gateway/internal/tlsconf` package centralises the
  listener configuration. Three modes: plain HTTP (lab default),
  HTTPS, HTTPS + mTLS for ES2+.
- Per-request middleware enforces "verified client cert required"
  on `/gsma/rsp2/es2plus/*` and lets `/v1/*` admin paths through
  unchanged. The two surfaces share the listener but live in
  different auth realms — BSS-side mTLS, operator-side OIDC
  (planned). `tls.VerifyClientCertIfGiven` at the handshake plus
  the middleware at the request level form a belt-and-suspenders
  defence against future config drift.
- Verified by 7 integration tests using a freshly minted in-process
  PKI: TLS-only without mTLS lets all clients in; mTLS lets `/v1/*`
  through cert-less; mTLS rejects ES2+ with no client cert (401);
  mTLS accepts ES2+ with a trusted client cert (200); mTLS rejects
  ES2+ with a client cert from an unrelated CA. Plus negative
  cases for missing key file and empty CA bundle.
- `cmd/gateway` exposes `--tls-cert`, `--tls-key`,
  `--es2plus-client-ca`. The startup banner prints the active mode
  and warns when ES2+ mTLS is disabled.
- Helm chart: gains `gateway.tls.serverSecret` and
  `gateway.tls.es2plusClientCASecret` values (both empty by
  default). When set, the chart mounts the Secrets at
  `/etc/aether/tls/` and `/etc/aether/es2plus-ca/` and passes the
  matching flags. Health probes adapt to HTTP vs HTTPS scheme.
- Helm chart also gains the missing `eim:` block (the eIM service
  shipped earlier had no chart values, blocking template
  rendering for any release that included it).
- Status table: gateway moves from Skeleton to Partial. SAS-SM
  gap analysis rows for `ES2+ DownloadOrder authenticated` and
  `mTLS for ES2+ inbound` updated to reflect the actual
  enforcement (was: "configured"; now: tested-enforcement).

SAS-SM evidence templates (`docs/sas-sm/`):
- The first walkable preparation pack for an MVNO running through
  SAS-SM accreditation. All free, in-repo, no paid tier — see
  GOVERNANCE.md §"What this project commits to".
- gap-analysis.md: every SAS-SM control family mapped to the
  Aether feature that satisfies it, the operator-supplied
  control needed to close the gap, and the evidence the auditor
  will expect to see. Covers Security Policy, Sensitive Process,
  Key Management, Network and Infrastructure, Audit and Logging,
  Personnel, Incident Management, Business Continuity.
- key-ceremony.md: a concrete, two-person-quorum HSM key-generation
  procedure with time-stamped step list and a tear-out
  chain-of-custody form ready to print. Documents what goes in
  the audit pack and what NEVER does (PINs, private key bytes).
- rbac.md: four documented roles (operator, key-custodian,
  auditor, incident-responder) with ready-to-apply Kubernetes
  Role manifests and the Postgres GRANT script that revokes
  UPDATE/DELETE on audit_entries from the application role.
  Quarterly review checklist.
- audit-retention.md: catalogues what the platform logs by
  default, the 3-year immutable retention default, the WORM
  offsite-copy pattern (S3 Object Lock Compliance mode), the
  hourly /v1/verify monitor, and the daily timeline-anchor
  backup that creates an external integrity proof.
- reference-aws.md: complete reference deployment topology for
  AWS GSMA-certified regions — EKS + RDS Multi-AZ + CloudHSM
  HA pair + S3 WORM bucket. Sample production Helm values,
  observability and alert rules, backup and DR plan, cost
  ballpark with CloudHSM as the dominant line item.
- mkdocs.yml updated with the new pages.

Status table: SAS-SM evidence templates moves from "Not started"
to "Partial" with explicit notes about what landed (gap analysis,
key ceremony, RBAC, audit retention, AWS reference) and what's
still pending (GCP and on-prem reference deployments, common
audit findings, recertification checklist, incident-response
runbook, worked evidence packages from real audits).

This is the section the philosophy doc explicitly identifies as
the project's most powerful organic-marketing surface. The bar
is "70%+ of evidence Aether-emitted, 30% guided by templates."
We are partway up that hill; each subsequent piece tightens the
gap.

eIM service (Phase 4 — SGP.32):
- New `services/eim` exposes the operator's IoT control plane: a
  device registry keyed by EID and a per-device command queue.
- Device API: register / list / fetch / deregister.
- Operator command API: enqueue / list. Four command kinds:
  download_profile, enable_profile, disable_profile, delete_profile.
- IPA-side API: GET /v1/ipa/{eid}/poll (atomically marks pending
  commands as Delivered as they're returned) and POST
  /v1/ipa/{eid}/commands/{id}/ack with state=completed|failed.
  Acks update the device's last_seen.
- Both backends behind a `Store` interface: in-memory (lab default)
  and Postgres-backed with foreign-key cascade deletes from
  devices to commands. Schema applied on startup. Postgres path
  detects 23505 unique-violation as the duplicate-register signal.
- Lifecycle tests cover: register, duplicate-register (409),
  enqueue against unknown device (404), poll → deliver → ack,
  re-poll after ack drops the command from the active set,
  operator view retains completed history.
- Wired into lab compose on :8449 with AETHER_PG_URL.
- Gateway gains `--eim` flag and `/v1/eim/devices` proxy.
- Admin UI gains a sidebar entry and `/eim` page listing
  registered devices with EID, label, tags, registered_at,
  last_seen.

Honest gaps (called out in services/eim/README.md):
- IPAd direct flow uses the command queue today; full SGP.32
  IPAd profile-download integration with smdp-plus is the next
  step.
- IPAe (indirect, eIM-as-relay) flow is not started.
- ES_eIM_Device authenticated transport (mTLS or signed commands
  between eIM and IPA) is the production-readiness item.
- Bulk operations / push notifications are not started.

SM-DP+ eUICC verification on authenticateClient (SGP.22 §5.6.2 / §5.7.5):
- New `pkg/certmgrclient`: Go client for certmgr. `TrustStore` and
  `Intermediates` return parsed *x509.Certificate slices ready for
  cert-pool construction.
- `certmgr` gains `GET /v1/trust-store/pem` and `GET /v1/intermediates/pem`
  returning the PEM bytes — needed by callers that have to actually
  verify peer chains (the existing JSON `/v1/trust-store` was
  metadata only).
- `services/smdp-plus/internal/identity` gains `TrustMaterial` +
  `FetchTrustMaterial` which pulls the trust set from certmgr and
  builds *x509.CertPool for roots and intermediates. Empty trust
  store is rejected as a config error.
- `services/smdp-plus/internal/signing` gains `EuiccSigned1`
  (mirroring §5.7.13: transactionId, serverAddress, serverChallenge,
  euiccInfo2, ctxParams1) and `VerifyEuiccAuthenticate` which
  verifies the eUICC chain (leaf → EUM → CI root), the ECDSA
  signature against the leaf's public key, and that
  `serverAddress` matches the configured SM-DP+ address.
- `authenticateClient` handler now does the full check when
  Config.Trust is set: chain, signature, serverAddress, plus
  replay defense — `euiccSigned1.serverChallenge` must equal the
  challenge the SM-DP+ issued in initiateAuthentication.
  401 Unauthorized on any failure; 400 if required fields missing.
- API type extended: `AuthenticateClientRequest` carries the four
  pieces individually (euicc_signed1, euicc_signature1,
  euicc_certificate, eum_certificate). The legacy single-blob
  `authenticate_server_response` field is reserved for the
  spec-faithful outer SEQUENCE once the Annex B modules are
  vendored.
- Unit tests cover happy path + four rejection cases (tampered
  payload, tampered signature, unknown CI root, wrong server
  address) using a synthetic CI→EUM→eUICC chain minted in process.
- Integration test in `internal/server` drives the full flow:
  initiateAuthentication → eUICC produces signed response →
  authenticateClient verifies. Replay defense, unknown-CI, and
  tampered-signature paths all gated.
- `cmd/smdp-plus`: new `--certmgr` flag enables verification.
  Lab compose already passes both `--hsm-broker` and `--certmgr`,
  so `make lab-up` now exercises the full bidirectional auth
  cryptography end to end.

SM-DP+ signing pipeline:
- New `pkg/hsmclient`: shared Go client for the HSM broker. Mirrors
  the broker's HTTP+JSON surface 1:1 so the eventual gRPC migration
  doesn't change callers.
- `services/smdp-plus/internal/identity`: provisions the SM-DP+'s
  DPauth identity. Lab mode generates a fresh key in the broker on
  startup and self-signs an X.509 wrapper around the public point;
  production mode references the operator's pre-loaded HSM key by
  label and the certmgr-served CI cert.
- `services/smdp-plus/internal/signing`: implements `ServerSigned1`
  per SGP.22 §5.7.13 (transactionId, euiccChallenge, serverAddress,
  serverChallenge) and `SignServerSigned1` which builds the DER,
  hashes with SHA-256, and asks the broker to sign per §H.5.
- `initiateAuthentication` now populates `ServerSigned1`,
  `ServerSignature1`, and `ServerCertificate` when signing is
  enabled (set `--hsm-broker`); returns the previous nil-fields
  shape when it isn't, so we don't fabricate signatures.
- `euicc_challenge` length now enforced to 16 bytes per spec.
- Verified end-to-end: a server-side test extracts the public key
  from the returned cert, recomputes the digest, and verifies the
  signature with stdlib ECDSA. A matching lab smoke test under
  `test/e2e` does the same against a running stack.
- Lab compose already passes `--hsm-broker`, so `make lab-up`
  demos signing automatically.

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
