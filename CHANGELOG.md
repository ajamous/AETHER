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

Conformance harness (`tools/conformance/`):
- New SGP.23 conformance suite scaffolding. The runner aggregates
  the per-module unit tests Aether already ships into a single
  invocation, classified by SGP.23 family (ES2+, ES8+, ES9+,
  ES11/ES12, SGP.32, Audit, Certs, HSM). 40 cases total, all
  green at landing.
- `tools/conformance/runner` is a small Go CLI that walks the
  catalogue, shells out to `go test -count=1 -run <pattern>` per
  case, classifies results by family, and reports pass/fail with
  per-family counts. `--list` enumerates the catalogue without
  running anything; `--family ES9+` filters.
- `tools/conformance/coverage/sgp23.md` is the section-by-section
  coverage matrix mapping every SGP.23 test family to the Aether
  test that covers it, the hardware tests that need a real
  sysmoEUICC bench, and the explicitly out-of-scope items
  (LPA conformance, eUICC firmware, HSM physical security).
- New `make conformance` target. New `conformance` job in
  `.github/workflows/ci.yml` runs the suite on every PR.
- README Status table: "Conformance harness (SGP.23)" moves
  from Not started to Implemented with the explicit caveat
  that hardware-in-the-loop tests are honestly out of scope
  pending a CI hardware bench (Phase 6+ investment).

SAS-SM section: GCP and on-prem reference deployments
- `docs/sas-sm/reference-gcp.md` — full topology mirror of
  reference-aws.md but on Google Cloud: GKE (Autopilot
  recommended) + Cloud SQL Postgres regional HA + Cloud HSM
  (Marvell LiquidSecurity, FIPS 140-2 L3) + GCS bucket with
  Bucket Lock Compliance retention. Sample production Helm
  values pinning Cloud SQL Auth Proxy sidecar, Marvell PKCS#11
  library path, Secret Manager CSI for the HSM PIN, GCE ALB
  ingress with mTLS via ServerTlsPolicy. Cost ballpark
  (Cloud HSM dominates, ~$2k/mo).
- `docs/sas-sm/reference-onprem.md` — for operators with their
  own HSM hardware or data-residency constraints. Vendor-
  agnostic K8s (Rancher / OpenShift / k3s / kubeadm) +
  Postgres HA via Patroni or pg_auto_failover + Thales Luna
  SA *or* Utimaco SecurityServer + S3-compatible WORM storage
  (MinIO Object Lock Compliance, Cloudian, or Scality). The
  hsm-broker code path is identical to the cloud references —
  ADR 0003's whole point. CapEx vs OpEx analysis: the
  cloud references trade ~$2.5k/mo of HSM-as-a-service for
  $40-60k of one-time HSM CapEx that amortises over 3-5y.
- `docs/sas-sm/index.md` Status table: both reference
  deployments move to Implemented. The section now ships nine
  documents (gap analysis, key ceremony, RBAC, audit retention,
  three reference deployments, DR runbook, incident response,
  common audit findings, recertification checklist) — only
  worked evidence examples (which can only come from adopters
  who've passed audits) remain "Not started."
- `docs/sas-sm/gap-analysis.md` "What still has gaps" list
  updated: GCP / on-prem references dropped, only worked
  evidence examples and multi-region active-active and
  Grafana dashboards remain.
- mkdocs.yml SAS-SM nav extended with both new pages.
- README Status table: SAS-SM evidence templates moves from
  Partial to Implemented.

Helm chart lab cert-init:
- Lab-mode certmgr Deployment now ships with an initContainer
  that runs `certmgr --generate-lab=/certs` on every pod start,
  dropping a fresh SGP.26-style chain (CI root → EUM → DPtls/
  DPauth/DPpb, ECDSA P-256, 24-hour validity) into an emptyDir
  volume. The main container mounts the same emptyDir read-only
  and starts as soon as the chain is in place.
- `helm install aether ./aether` is now a true one-shot for the
  lab — no operator pre-work, no manual ConfigMap to create, no
  side-channel `docker run` to mint certs offline. The README
  section that documented the manual step is replaced with the
  new behaviour.
- The pattern is deliberately emptyDir, not a PVC: lab certs are
  ephemeral by design (ADR 0004). Each pod restart mints a new
  chain; keys never persist to durable storage. Production mode
  skips the initContainer entirely; CI-issued certs come from
  the existing `identitySecret` + trust-store ConfigMap path.
- New value `certmgr.lab.certInit.enabled` (default true). Set
  false to opt out and bring your own pre-populated `/certs`.
- The certmgr Deployment template moves from the shared
  `aether.backendDeploy` helper to inline so it can attach the
  emptyDir volume and the initContainer — the helper handles
  uniform Deployments and shouldn't accumulate special cases.
- Verified by `helm template`: lab default renders the
  initContainer with `--generate-lab=/certs`; production mode
  with `certmgr.mode=production --set certmgr.trustStore=...`
  renders zero `cert-init` references. Both modes lint clean.
- NOTES.txt's lab-mode warning replaced with a calmer NOTE:
  initContainer auto-bootstraps; ephemeral by design.
- Status table: Helm chart Implemented note no longer claims
  cert-init Job is "pending"; chart README's "Lab cert chain"
  section is rewritten to describe the new behaviour and the
  opt-out value.

Observability instrumentation closes the bundle's pending alerts:
- New `services/hsm-broker/internal/metrics` package: small
  dependency-free Prometheus exposition (LatencyHistogram +
  LabeledCounter) with atomic-bucket concurrency and the standard
  HSM-call latency layout (5ms/10ms/25ms/50ms/100ms/250ms/500ms/1s/2.5s).
  3 unit tests including 8-writer × 1000-observation concurrency
  check.
- hsm-broker Sign handler now wraps its broker.Sign call with
  `signLatencySec.Observe(time.Since(start))`. The /metrics
  endpoint emits `aether_hsm_sign_duration_seconds_bucket`,
  `_sum`, and `_count`. Drives the previously-pending
  AetherHSMSignLatencyP99 alert.
- New `services/gateway/internal/metrics` package mirroring the
  hsm-broker shape (LabeledCounter only — no histograms needed
  yet). Counter values pre-registered so the hot path is
  lock-free; unregistered labels silently dropped.
- Gateway tlsconf middleware gains an UnauthorizedReporter
  callback. Each 401 on `/gsma/rsp2/es2plus/*` is tagged with
  reason ∈ {no_tls, no_client_cert, chain_invalid} so the
  on-call can route quickly. Reporter stays nil-safe — when
  no reporter is supplied, the middleware is unchanged.
- Gateway server now registers an
  `aether_gateway_es2plus_unauthorized_total{reason}` counter
  pre-loaded with the three reason values, exposes /metrics
  (admin path; mTLS gate doesn't apply), and passes itself as
  the reporter into the middleware.
- Verified by TestGateway_MTLS_401CounterAdvances: drives 3
  cert-less ES2+ requests, confirms the counter shows
  reason="no_client_cert" 3 and the other two reasons stay 0.
- Bundle README, Status table, reference-aws.md, and CHANGELOG
  all updated: 11 alerts, all wired to live metrics emitted by
  Aether services or standard exporters. The bundle moves from
  Partial to Implemented; only Grafana dashboards remained
  outstanding.

Grafana dashboards (`deployments/observability/grafana/`):
- Three Grafana 10.x dashboards backed by metrics already
  emitted on Aether `/metrics` endpoints:
    aether-overview        — audit chain status, HSM broker
                              ready, identity cert days-to-expiry
                              bargauge, unavailable replicas per
                              service, pod restart rate, Postgres
                              connection-pool utilisation gauge,
                              per-service scrape `up` series.
    aether-hsm             — Sign latency p50/p95/p99 timeseries
                              with the 250ms SLO threshold drawn,
                              throughput, broker readiness over
                              time, full-distribution latency
                              heatmap.
    aether-gateway-es2plus — per-reason 401 rate (5m), cumulative
                              counts, reason mix donut over the
                              dashboard range.
- Every panel queries the same metrics the alert rules already
  use; the panels light up red when the corresponding alert is
  firing, keeping dashboards and alerts in lock-step.
- Datasource is parameterised as `${DS_PROMETHEUS}` so adopters
  pick their Prometheus on import or via provisioning.
- README documents three import paths (Grafana UI, file
  provisioner, kube-prometheus-stack ConfigMap with
  `grafana_dashboard=1` label) and is honest about what's NOT
  here: a service-map dashboard (no traces yet), capacity
  planning (depends on retention), and one-panel-per-alert (the
  rest live in Alertmanager + the runbook).
- New `grafana-dashboards` job in `.github/workflows/ci.yml`
  parses every dashboard JSON with `jq` and asserts each has a
  uid, a title, and at least one panel — schema-level validation
  is intentionally not in CI because Grafana's dashboard schema
  is loose and panel-type-specific.
- Top-level Status row "Observability bundle" tightened: drops
  the "Grafana dashboards still pending" tail.

Observability bundle (`deployments/observability/`):
- 9 Prometheus alert rules covering audit chain integrity (Sev-1
  on aether_audit_chain_ok=0), audit-metrics scrape failure,
  cert expiry at three thresholds (sev-3 < 30d / sev-2 < 7d /
  sev-1 expired), service availability + crash-loop, HSM broker
  unhealthy, and Postgres connection exhaustion.
- Two flavours: vanilla Prometheus rules YAML
  (prometheus/prometheus-rules.yaml) for adopters running plain
  Prometheus, plus PrometheusRule + ServiceMonitor CRDs
  (prometheus-operator/) for kube-prometheus-stack adopters.
- Both forms cross-reference docs/sas-sm/incident-response.md
  via runbook_url annotations on the audit-chain-broken alert.
- Two metrics added to make alerts fire from native data:
    services/hsm-broker /metrics — emits aether_hsm_broker_ready
    services/audit /metrics    — emits aether_audit_chain_ok
                                 and aether_audit_chain_length
  Both are tiny hand-rolled exposition; a fuller instrumentation
  pass (Sign-latency histogram, gateway 401 counter) is the
  named follow-up that closes the last two pending alerts.
- Helm chart: new observability.prometheusOperator.enabled
  value (default false). When true, the chart renders the
  PrometheusRule + ServiceMonitor CRDs in-release, scoped to
  the release label kube-prometheus-stack by default.
- CI: new prometheus-rules job runs `promtool check rules` on
  every PR.
- Status table: new "Observability bundle" row marks Partial.
  SAS-SM gap analysis "Key expiry monitoring and alerting" row
  upgraded to point at the shipped alert rules. reference-aws.md
  Observability section rewritten to enumerate the 9 implemented
  alerts and the 2 pending ones honestly.

SAS-SM section: four operator runbooks land:
- `disaster-recovery.md` — three-tier scenario model (single-AZ
  failure, regional outage, database compromise), starting RTO/RPO
  table per service, recovery procedures keyed to the WORM-bucket
  daily timeline anchor pattern from audit-retention.md, drill
  cadence, evidence checklist. The audit-chain-break path
  (Sev-1, preserve-evidence-first) crossreferences the incident
  runbook below.
- `incident-response.md` — severity matrix (audit chain integrity
  break is always Sev-1; suspected key exposure is always Sev-1),
  named roles with the operator/IC-can't-also-be-engineer rule,
  step-by-step ack/triage/mitigate/recovery flow, mitigation
  table by symptom, audit-chain-break sub-procedure, postmortem
  template that lands under `docs/operations/postmortems/`.
- `common-findings.md` — 18 recurring SAS-SM audit findings
  catalogued by control family (key management, audit logging,
  network and access, personnel, crypto, ops). Each entry pairs
  the platform default that pre-empts the finding with the
  operator gotcha that could still catch you out. This is the
  high organic-marketing surface the philosophy doc identifies.
- `recertification-checklist.md` — 60-days-out / 30-days-out /
  audit-week / after-the-audit checklist, organised in the order
  the auditor will read your evidence pack.

mkdocs.yml extended with the new pages. README Status table
updated to reflect the four new docs landed and what's still
pending (GCP and on-prem reference deployments, plus the
adopter-contributed worked-evidence examples). The SAS-SM
gap analysis Incident Management and Business Continuity rows
moved from "(planned, see Status table)" to direct links at the
new runbooks, and the bottom-of-file gap list updated to no
longer claim DR is unsolved (cold-start to qualifying secondary
region is now documented; multi-region active-active is the
remaining Phase-6 platform follow-up).

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

Terraform (`deployments/terraform/gcp/`):
- GCP reference deployment as IaC, symmetric to the AWS module:
  VPC + private subnet + NAT + 5-second-aggregation Flow Logs,
  GKE **Autopilot** with private cluster + Workload Identity +
  Cloud Logging/Monitoring + deletion protection, Cloud SQL
  Postgres 16 regional HA with CMEK + 35-day backup + PITR +
  IAM authentication + private-only IP, Cloud HSM via Cloud
  KMS at `protection_level = HSM` (FIPS 140-2 Level 3) with a
  90-day rotation period, GCS audit bucket with **Bucket Lock
  Compliance** mode + CMEK + uniform bucket-level access +
  public access prevention + lifecycle to Coldline after 90d.
- GCP service accounts for audit and hsm-broker. Workload
  Identity binding to the chart's per-service Kubernetes
  ServiceAccounts (`<release>-aether`) is a documented manual
  post-deploy step (chart release name isn't known until
  install time; a future iteration will read it from a data
  source and wire automatically).
- Submodule layout under `modules/` (network, iam, gke,
  cloudsql, cloudhsm, storage); canonical wiring under
  `examples/full/` with a `next_steps` output that prints the
  exact `gcloud iam service-accounts add-iam-policy-binding`
  commands for the operator.
- Same posture-as-policy stance as the AWS module: regional
  HA, CMEK, Bucket Lock Compliance, private cluster, Flow
  Logs are not exposed as variables. Weakening any of those
  would break the reference-gcp.md / gap-analysis claims.
- `terraform-validate` CI job now runs as a matrix job over
  `[aws, gcp]` so both modules stay validate-clean on every PR.
- README documents what the module does NOT do: Cloud HSM
  ceremony (manual), Aether deploy itself (Helm chart is
  separate), IdP, ingress, multi-region active-active (Phase
  6), enabling GCP APIs (operator does that ahead of apply),
  and Terraform state backend.
- Top-level Status row "Terraform modules" updated: AWS + GCP
  Implemented; Azure not yet started. reference-gcp.md no
  longer claims the modules are unwritten — points at this
  directory and surfaces the manual Workload Identity binding
  + Cloud HSM ceremony as documented post-deploy work.

Terraform (`deployments/terraform/aws/`):
- AWS reference deployment as IaC: VPC + private/public subnets +
  NAT + Flow Logs (365-day retention), EKS with all five
  control-plane log types and customer-managed KMS envelope
  encryption for Kubernetes Secrets, RDS Postgres Multi-AZ with
  CMEK + 35-day backup + deletion protection + Performance
  Insights + Secrets Manager-managed master password, CloudHSM
  cluster with HSMs spread across AZs, S3 audit-log bucket with
  Object Lock **Compliance** mode + KMS encryption + lifecycle
  to Glacier after 90d, cross-region replica bucket skeleton
  (replication rule wiring requires a caller-supplied
  second-region provider).
- IAM roles for EKS cluster, EKS nodes, audit, hsm-broker —
  IRSA trust-policy attachment is documented as a manual
  post-deploy step (the EKS OIDC issuer ARN isn't known until
  the cluster is up; a future iteration will read it from the
  data source and wire automatically).
- Submodule layout under `modules/` (network, iam, eks, rds,
  cloudhsm, storage); canonical wiring under `examples/full/`.
- Production posture pinned by the module, not by variables:
  Flow Logs on, all EKS control-plane logs on, RDS Multi-AZ +
  CMEK + 35-day retention + deletion protection, S3 Object
  Lock Compliance + KMS-CMEK + public access block + versioning.
  These are SAS-SM policy, not preference, so the module
  deliberately does not expose them as inputs.
- New `terraform-validate` job in `.github/workflows/ci.yml`
  runs `terraform fmt -check` + `terraform init -backend=false`
  + `terraform validate` on both the root module and
  `examples/full` on every PR.
- README documents what the module does NOT do: it does not run
  the CloudHSM activation ceremony (manual two-person procedure
  per `key-ceremony.md`), it does not deploy Aether itself (Helm
  chart is separate), it does not configure your IdP or ingress,
  it does not handle multi-region active-active (Phase 6), and
  it does not back up Terraform state (operator configures a
  remote backend).

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
