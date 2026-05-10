# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

smdp-plus BPP outer-SEQUENCE assembly
(`services/smdp-plus/internal/bpp/bpp.go`):
- Closes the **last codec layer** in the BPP critical path.
  Every ASN.1 piece the SGP.22 §5.7.6 BoundProfilePackage needs
  now lives in tree:
  - `ControlRefTemplate` (§H.4): SCP03t setup parameters
    (keyUsageQualifier=0xC8, keyType=0x88, keyLength=0x10,
    optional hostId), all APPLICATION-tagged per spec.
  - `InitialiseSecureChannelRequest` (§5.7.7): the signed
    preamble — remoteOpId + transactionId +
    controlRefTemplate + smdpOtpk + smdpSign, with the
    APPLICATION-N tags the spec mandates.
  - `AssembleBoundProfilePackage`: hand-rolled TLV builder
    that wraps `[16] InitialiseSecureChannelRequest` + the
    `[APPLICATION 24] sequenceOf88` segment list + the
    mandatory-but-empty `[APPLICATION 26] sequenceOf86` slot
    inside the outer `[APPLICATION 54]` constructed SEQUENCE.
- The hand-rolled TLV path (rather than a flat Go struct) is
  intentional: SGP.22 mixes APPLICATION-class constructed
  forms with mandatory-empty trailing fields that
  `encoding/asn1` doesn't express in a single struct. We keep
  byte-exact control over every tag/length/body.
- New `SignedInputBytes(transactionID, smdpOtpk, euiccOtpk)`
  helper exposes the `transactionId ‖ smdpOtpk ‖ euiccOtpk`
  concatenation that the SM-DP+ MUST sign with its DPpb key
  per §5.7.7. Auditors looking at this file see exactly what
  gets signed without reading codec internals.
- 12 test groups: ISCR DER round-trip, every validate()
  rejection path (empty/oversized tid, otpk wrong length,
  empty signature, remoteOpId out of range),
  SignedInputBytes concatenation correctness, outer-tag
  shape (verifies the DER starts with `[APPLICATION 54]
  constructed high-tag-number form`), assemble rejects
  empty-segments / invalid ISCR, byte-stable assembly across
  invocations, DER length round-trip across short/long-form
  boundary, TLV-builder high-tag-number form, short-tag form,
  and stripTag body preservation.
- 6 conformance harness cases added. 80 → 86 cases.

Status row updates:
- `services/smdp-plus`: caveat tightens. Now lists the outer
  BoundProfilePackage assembly as a current capability. The
  documented critical-path remainder shrinks to **wiring
  only** — capture eUICC otPK + generate SM-DP+ ephemeral
  keypair + sign InitialiseSecureChannelRequest + assemble +
  wire into `getBoundProfilePackage` handler. No new ASN.1 or
  crypto remains.
- Coverage matrix: 6 new ES8+ rows. The
  `GetBoundProfilePackage` row's "remaining work" updated
  with the wiring-only summary. 80 → 86 cases.

smdp-plus BPP segmentation
(`services/smdp-plus/internal/bpp/segment.go`):
- Closes another layer of the BPP critical path. Previous PR
  shipped `Derive` (ECKA + KDF → SENC/SMAC/MCV); this PR ships
  `SealSegments` and `OpenSegments` — AES-128-GCM seal/open of
  arbitrary plaintext into MAC-chained ciphertext segments
  matching SCP03t's shape.
- `SealSegments(keys, plaintext, segmentSize)` chunks plaintext
  into ≤1024-byte segments, AES-128-GCM-seals each under
  `keys.SENC`, and chains the previous segment's GCM tag into
  the next segment's AAD via `keys.InitialMCV`. Per-segment
  nonces are deterministic (counter starts at 1, increments
  per segment) — SCP03t-style; freshness comes from the per-
  session SENC, not from random nonces.
- `OpenSegments` reverses the operation under the same keys.
  Tamper-or-reorder detection is automatic: any failed GCM
  authentication or a broken MCV chain returns an error and
  no plaintext bytes (SCP03t treats partial decrypt as fatal).
- Spec-precise per-segment AAD framing (counter encoding,
  ICV-as-AAD layout from SGP.22 §H.3) is the explicit follow-
  up. Today's segmenter is shape-correct and round-trips
  against itself; cross-vendor interop with a real eUICC waits
  on the hardware bench. The package godoc spells this out.
- 9 test groups: round-trip across multiple segment sizes,
  end-to-end with ECKA-derived keys (both halves of a fresh
  agreement open each other's seals), tamper detection on
  ciphertext and on tag, segment-reorder rejection (the MCV
  chain catches permutation — replay defense), validation of
  every malformed input (nil keys, short SENC, short MCV,
  zero/oversized segment size, empty plaintext), and the
  paranoid nonce-uniqueness check that catches the AES-GCM
  nonce-reuse footgun.
- 5 conformance harness cases added (round-trip, ECKA end to
  end, tamper detection, chain break, nonce uniqueness). 80
  cases total now (was 75).

Status row updates:
- `services/smdp-plus`: caveat refined again. Now lists
  AES-128-GCM segment seal/open with MAC chaining alongside
  the existing capabilities. The documented critical-path
  remainder shrinks to "the `prepareDownload` HTTP handler
  that captures eUICC otPK + outer BoundProfilePackage
  SEQUENCE assembly".
- Coverage matrix: 5 new ES8+ rows. The Full
  ProtectedProfilePackage framing row's "remaining work"
  shrinks to the spec-precise AAD layout (called out as
  hardware-bench-driven). 75 → 80 cases.

smdp-plus BPP session-key derivation
(`services/smdp-plus/internal/bpp/keys.go`):
- Closes another layer of the BPP critical path. The previous
  PR shipped DPpb-signed `SmdpSigned2`; this PR ships the
  cryptographic glue that turns an ECKA exchange into the
  SCP03t-labelled session keys (SENC, SMAC, initialMCV) that
  every BPP segment will be sealed under.
- New `services/smdp-plus/internal/bpp` package with one
  function: `Derive(spDPpriv, euiccPub, sharedInfo) →
  SessionKeys{SENC, SMAC, InitialMCV}`. Wraps `pkg/crypto/ecka`
  (which already does ECDH + X9.63-SHA-256 KDF) and slices the
  output into the three named 16-byte slots SCP03t talks in
  per SGP.22 §H.3. Derives 64 bytes from the KDF (slots 4
  reserved for future SCP03t fields) so the slot layout stays
  stable as those land.
- The package is intentionally a separate import target from
  `internal/server` so unit tests exercise key derivation
  without an HTTP server or session store. Import direction is
  one-way: bpp → pkg/crypto, never reverse.
- 7 unit tests cover the SAS-SM-relevant invariants:
  `TestDerive_BothSidesAgree` confirms the SM-DP+ and eUICC
  derive identical keys from the matching ephemeral keypairs
  (the prerequisite for every BPP segment to GCM-authenticate
  on-card); `TestDerive_DifferentSharedInfoDifferentKeys`
  checks the replay-defense binding; `TestDerive_DistinctSlices`
  catches a slicing-aliasing bug that would silently make
  SENC, SMAC, and MCV identical;
  `TestDerive_StableAcrossInvocations` confirms determinism so
  the MCV chain reconstructs the same way on both sides; nil-
  arg + empty-sharedInfo guards.
- `services/smdp-plus/go.mod` adds `pkg/crypto` as a workspace
  `replace`-dep, mirroring the existing pattern this module
  uses for `pkg/hsmclient` and `pkg/certmgrclient`.
- 4 conformance harness cases added (both-sides agreement,
  slice shape, sharedInfo binding, determinism). 75 cases
  total now (was 71).

Status row updates:
- `services/smdp-plus`: caveat refined again. Now lists BPP
  session-key derivation under `internal/bpp` as a current
  capability. The documented critical-path remainder shrinks
  to "AES-128-GCM segmentation around the SAIP UPP +
  `prepareDownload` HTTP handler that captures the eUICC's
  otPK".
- Coverage matrix: 4 new ES8+ rows for the session-key
  derivation invariants. The Full ProtectedProfilePackage
  framing row's "remaining work" list shortened by one item.
  71 → 75 cases.

smdp-plus: AuthenticateClient now returns DPpb-signed SmdpSigned2:
- Wires the SmdpSigned2 codec from the previous PR into the
  `authenticateClient` HTTP handler. When the new DPpb identity
  is configured, the response carries SmdpSigned2 + ECDSA-DER
  signature + DPpb cert that the eUICC verifies before
  generating its own ephemeral pubkey for the upcoming BPP
  exchange. Lab default (no DPpb): the fields stay empty so
  test harnesses without a trust store keep working.
- New `Config.DPpb *identity.Identity` and
  `dppbSigningEnabled()` helper. The DPpb identity is a
  separate `EnsureLabIdentity` keypair (lab) or CI-issued cert
  (production); separate ceremony lifecycle from DPauth, per
  the SAS-SM key-ceremony procedure.
- New `--dppb-label` flag in `cmd/smdp-plus`. Empty label
  disables; the warning at startup spells out that an eUICC
  will reject the response if SmdpSigned2 is absent in
  production.
- `bppEuiccOtpk` is intentionally omitted in this PR's
  SmdpSigned2 payload. The eUICC has not yet generated its
  ephemeral pubkey at AuthenticateClient time; it does so
  AFTER verifying SmdpSigned2 and returns the otpk inside the
  PrepareDownloadResponse that GetBoundProfilePackage carries.
  Re-signing SmdpSigned2 with the otpk filled in is the BPP
  follow-up's job.

Tests:
- `TestAuthenticateClient_DPpbSigningEndToEnd` drives a full
  initiateAuthentication + authenticateClient flow with both
  DPauth and DPpb identities configured against a fake hsm-
  broker. Decodes the returned SmdpSigned2 to confirm the
  transactionId matches what the SM-DP+ issued and ECDSA-
  verifies the signature against the broker's public key. As
  close to "an eUICC will accept this" as we get without a
  hardware bench.
- `TestAuthenticateClient_NoDPpbLeavesSmdpSigned2Empty`
  confirms the lab path stays unchanged: when DPpb isn't
  configured, the response carries no SmdpSigned2 fields.
- 2 conformance harness cases added; 71 cases total now (was
  69).

Status row updates:
- `services/smdp-plus`: caveat refined again. Now lists
  DPpb-signed SmdpSigned2 on authenticateClient as a current
  capability. Documented critical-path remainder is now just
  "BPP wrapping" (capture eUICC otPK from
  PrepareDownloadResponse + ECKA + KDF + AES-GCM segmentation
  around the SAIP UPP).
- Coverage matrix: 2 new ES9+ rows. 69 → 71 cases.

`SmdpSigned2` codec + DPpb sign helper
(`services/smdp-plus/internal/signing/smdp_signed2.go`):
- Closes the next layer of the BPP critical path. Earlier PRs
  shipped `pkg/saip` (the UPP layer) and the doc-vs-catalogue
  refresh that documented the remaining steps; this PR ships
  the SGP.22 §5.7.14 SmdpSigned2 SEQUENCE that
  `prepareDownload` will sign over.
- New `SmdpSigned2` Go type mirrors the spec exactly:
  `transactionId` (1..16 bytes), `ccRequiredFlag` BOOLEAN,
  `bppEuiccOtpk` `[APPLICATION 73] OCTET STRING OPTIONAL`. The
  APPLICATION-73 tag is the one SGP.22 quirk; the rest uses
  `encoding/asn1` defaults.
- `MarshalDER` enforces every spec invariant before emitting
  bytes (transactionId range, otpk length 33/65 only, otpk
  first-byte 0x02/0x03 for compressed and 0x04 for uncompressed
  P-256 points). `UnmarshalSmdpSigned2` rejects trailing bytes.
- `SignSmdpSigned2` follows the established pattern from
  `ServerSigned1` and the audit-anchor codec: DER → SHA-256 →
  hsm-broker `Sign` against the **DPpb** key (distinct from
  DPauth — separate ceremony lifecycle, separate rotation
  cadence per the SAS-SM key-ceremony procedure).
- 6 unit tests cover the round-trip in three shapes (no otpk,
  compressed otpk, uncompressed otpk), every `validate()`
  rejection (empty / oversized tid, wrong-length otpk,
  bad-first-byte for compressed and uncompressed), trailing-
  byte rejection on Unmarshal, and a fake-broker end-to-end
  signing test that ECDSA-verifies against the broker's public
  key + round-trips the signed payload to confirm fields don't
  drift between sign and verify.
- 3 conformance harness cases added (round-trip, validation,
  end-to-end signing). 69 cases total now (was 66).

Status row updates:
- `services/smdp-plus`: caveat tightened. Now lists
  `SmdpSigned2` codec + DPpb sign helper alongside the existing
  ServerSigned1 / EuiccSigned1 capabilities. The
  `getBoundProfilePackage`-returns-501 caveat is unchanged but
  the documented critical path is now smaller — the remaining
  work is the `prepareDownload` HTTP handler + BPP wrapping
  (ECKA + KDF + AES-GCM segmentation around the SAIP UPP).
- Coverage matrix: ES9+ section gains 3 SmdpSigned2 rows. The
  GetBoundProfilePackage row's "remaining work" list shortened
  by one item. 66 → 69 cases.

Conformance coverage matrix refresh
(`tools/conformance/coverage/sgp23.md`):
- Closes a doc-vs-catalogue drift gap created by the previous
  PR. `pkg/saip` shipped 7 new conformance cases in the
  machine-readable catalogue, but the human-readable coverage
  matrix didn't have a SAIP section yet.
- New "SAIP — SGP.22 §B Profile Package codec" section with all
  9 rows: build/decode round-trip, validation rejections,
  decode-error rejections, AppendRaw insertion + guards,
  byte-stable encoding, profile-builder integration, ICCID
  nibble-swap, plus two explicit "Pending" rows for the richer
  ProfileElement types (PE-USIM / PE-PinCodes / PE-FileSystem /
  PE-AKAParameter) and for the full SGP.22 reference profile
  decode (hardware-bench fixture).
- Existing rows that referenced "SAIP codec" as a blocking
  dependency tightened to reflect the actual layered state:
  - ES8+ "Full ProtectedProfilePackage framing" row: now
    correctly notes that the UPP layer is in tree via
    `pkg/saip`; the remaining work is the smdp-plus BPP
    wrapping (ECKA + KDF + AES-GCM segmentation around the
    SAIP UPP).
  - ES9+ "GetBoundProfilePackage (BPP generation)" row: same
    update, plus the explicit list of what the BPP wrapping
    needs (InitialiseSecureChannelRequest signed with DPpb).
  - SGP.32 "IPAd direct profile fetch" row: tightened to
    reference both the `pkg/saip` ProfileElement catalogue and
    the smdp-plus BPP wrapping as the explicit dependencies.
- "How to read this table" intro updated: the "Pending" bucket
  is now driven by the smdp-plus BPP wrapping or the
  `pkg/saip` element catalogue continuing to grow, with each
  row calling out which.
- New "Updating this matrix" footer notes the machine-readable
  catalogue lives at `tools/conformance/runner/catalogue.go`
  and reports the current count (**66 cases across 10
  families**: ES2+, ES9+, ES8+/Crypto, SAIP, ES11/ES12,
  SGP.32, Audit, Certs, HSM, Admin). When the two drift, the
  `go test`-driven catalogue is authoritative.

`pkg/saip` (SGP.22 §B SAIP profile-package codec, minimum-viable subset):
- Closes the lynchpin Phase 1 dependency identified in the
  remaining-items audit. Three of four open Phase 1 rows
  collapsed into "ship `pkg/saip`"; this PR ships the first
  slice.
- New stdlib-only Go module `pkg/saip` with:
  - `ProfileHeader` SEQUENCE (major/minor version, profileType
    UTF8String, 10-octet nibble-swapped ICCID, mandatory-services
    SEQUENCE OF UTF8String).
  - `PEEnd` empty-SEQUENCE terminator.
  - `Build(header)` constructor that enforces every spec
    invariant and emits a CHOICE-tagged element list.
  - `MarshalDER` for byte-stable DER round-trip; `Decode` walks
    the outer SEQUENCE and returns CHOICE-tagged element bytes
    in document order.
  - `AppendRaw` lets callers splice pre-marshalled spare
    `ProfileElement` bytes (RFM, application, etc.) between the
    header and PEEnd without breaking package validity —
    forward-compat seam for the richer element catalogue that
    lands incrementally.
- 9 unit tests cover round-trip, every `validate()` rejection
  path (major/minor out of range, empty profileType, wrong-length
  ICCID, missing mandatory services), trailing-bytes / non-SEQUENCE
  / truncated decode rejections, AppendRaw insertion + guards,
  byte-stable encoding across invocations, and the short/long-form
  DER length helper.
- Stdlib-only on purpose: same supply-chain rationale as the
  OIDC verifier and the auditor CLI — the SAIP codec is a
  SAS-SM-relevant primitive and should be auditable from a single
  Go file with zero third-party dependencies.

`services/profile-builder` wired to emit real SAIP:
- `BuildUPP` now produces a DER-encoded `ProfilePackage` via
  `pkg/saip` and returns it in the UPP envelope's new `saip_der`
  field alongside the existing JSON-shaped inputs (kept for
  human-readable inspection through the admin UI).
- New `encodeICCIDNibbleSwapped` helper does the SGP.22 §B.1
  BCD nibble-swap (`d1 d2 → 0x[d2][d1]`, F-padding for 19-digit
  ICCIDs). Tested against published-spec example pairs.
- `services/profile-builder/go.mod` adds `pkg/saip` as a
  workspace `replace`-dep, mirroring the existing pattern that
  smdp-plus uses for `pkg/hsmclient` and `pkg/certmgrclient`.
- 3 new template tests: SAIP DER emission round-trips through
  `pkg/saip.Decode`, the header decodes with the expected
  fields, and the ICCID nibble-swap matches spec examples.
- `go.work` gains the new module.

Conformance harness gains 7 cases in a new "SAIP" family
(round-trip, validation, decode rejections, AppendRaw,
byte-stability, profile-builder integration, ICCID
nibble-swap). 66 cases total now (was 59).

Status row updates:
- `pkg/saip`: new row, "Partial — minimum-viable subset…"
- `services/profile-builder`: "Skeleton" → "Partial". Per-piece
  table updated: UPP generation Skeleton → Partial; SAIP codec
  "Not started" → "Partial — minimum-viable subset…"; BPP
  pipeline still "Not started" with the explicit dependency on
  smdp-plus BPP wrapping called out.
- Conformance harness: 59 → 66 cases, 9 → 10 families.
- `services/smdp-plus`'s "BPP returns 501 until SAIP codec
  lands" caveat is unchanged — wiring the smdp-plus BPP path
  (ECKA + X9.63-SHA-256 KDF + AES-128-GCM segmentation around
  the SAIP UPP) is the explicit follow-up.

SECURITY.md refresh:
- Closes a documentation gap created by the recent supply-chain
  PRs. SECURITY.md hadn't been touched since the project
  bootstrap and didn't mention any of the protections that
  landed since: Dependabot, govulncheck, CodeQL coverage, or
  cosign + Sigstore release signing. Researchers and SAS-SM
  auditors look at SECURITY.md first; an honest current view
  there matters.
- New "Supply-chain protections in place" section: a four-row
  table summarising what runs and where (Dependabot in
  `.github/dependabot.yml`, govulncheck in the per-PR CI job,
  CodeQL on every PR + weekly schedule, cosign + Sigstore on
  every release tag), plus a one-line note about the dual-format
  SBOM (SPDX + CycloneDX) attached to releases and a pointer to
  the toolchain-floor bump policy in `go.work`.
- New "Researcher-friendly scanning" section explicitly welcomes
  public security testing of unreleased branches and PRs and
  spells out what to include in a fix PR (GHSA/CVE/GO-YYYY-NNNN
  reference, `make govulncheck` output showing resolution,
  regression test where feasible).
- "Scope" section expanded: the release pipeline + cosign
  identity and the auditor verifier CLI are now explicitly in
  scope. Out-of-scope adds a clarifying note about CVEs in
  third-party container base images (Dependabot surfaces these
  on the next weekly window; the project's reachability story
  for those CVEs is whatever govulncheck reports).
- Cross-references to the dependency-update policy, the
  govulncheck local procedure, the release-verification doc,
  and the common-findings catalogue.

`docs/sas-sm/common-findings.md` F18 ("Unverified container
images in production"):
- Tightened to reflect what's actually shipped: the release
  workflow now produces dual SBOMs and cosign-signs every
  binary + both SBOMs + SHA256SUMS via Sigstore keyless OIDC.
  The "(planned) signs images with cosign" qualifier is
  replaced with an honest acknowledgement that container image
  signing is the next supply-chain follow-up — the release
  pipeline today builds binaries, container images are operator-
  built or built elsewhere, and Kyverno / Connaisseur are
  called out by name as the layered-on admission-time
  verification answer.

govulncheck in CI (`.github/workflows/ci.yml`, `Makefile`):
- Closes the next layer of the supply-chain story. Dependabot
  surfaces "a new version exists"; cosign + Sigstore prove "this
  binary came from this build"; govulncheck closes the gap
  between them by reporting **reachable** known vulnerabilities
  in each Go module's call graph. False-positive rate is low
  compared to a naive lockfile scan because govulncheck only
  flags vulnerabilities in code paths the program actually
  reaches.
- New `govulncheck (per module)` CI job iterates the workspace
  one `go.mod` at a time (the same pattern the build/test job
  uses, since `./...` from the repo root doesn't expand across
  module boundaries). Exit codes: 0 clean, 3 reachable vuln —
  the job fails on 3 and tolerates "no packages matched" for
  modules whose only files are behind a build tag (e.g.
  `test/e2e` under `//go:build lab`).
- Workspace floor in `go.work` bumped from `1.25.0` to
  `1.25.10` so the CI scan exits clean. The version is chosen
  to pull in every reachable stdlib fix the scanner flags:
  GO-2025-4007 (x509 name-constraint quadratic complexity, fixed
  1.25.3), GO-2025-4011 (asn1 memory exhaustion, fixed 1.25.2),
  the 1.25.5 x509 batch, and the 1.25.8 net/url batch. Re-running
  govulncheck against a stdlib older than 1.25.10 immediately
  surfaces the failure.
- New `make govulncheck` target with the same per-module shape
  so adopters can run the exact same scan locally.
- Existing CI workflows already use `setup-go: '1.25'` (no patch
  pin), which resolves to the latest 1.25.x available on the
  GitHub-hosted runner — currently 1.25.10+. The Makefile's
  install-hint strings bumped from `Go 1.22+` to `Go 1.25.10+`
  to match.
- CONTRIBUTING.md gains a "Vulnerability scanning (govulncheck)"
  subsection under "Dependency updates" explaining the local
  invocation, the exit-code semantics, and the Go-floor bump
  policy.

Status row updates:
- Conformance harness: 59 cases unchanged, all still passing on
  the bumped toolchain.

Release pipeline hardening (`.github/workflows/release.yml`,
`docs/sas-sm/release-verification.md`):
- Closes a real gap in the previous release pipeline. SBOM
  generation was guarded with `|| true` so a release could
  ship without one and nobody would notice; cosign was
  installed but never invoked; only SPDX was emitted (CSAF +
  OWASP Dependency-Track stacks expect CycloneDX).
- The release job now produces, for every artifact in `dist/`:
  the binary itself, a detached cosign signature (`.sig`), the
  ephemeral signing-cert chain (`.pem`), and a Sigstore bundle
  (`.cosign.bundle`) that carries the cert + Rekor inclusion
  proof for offline verification. Plus a `SHA256SUMS` (also
  signed) so adopters who don't trust Sigstore can fall back to
  checksum verification.
- SBOMs land in BOTH formats: SPDX JSON (US-government tooling
  default) and CycloneDX JSON (CSAF + OWASP DT default). The
  `|| true` guard is dropped — a missing SBOM now fails the
  release loud, which is the correct posture.
- The auditor verifier CLI built in the previous PR
  (`tools/aether-verify-anchor`) is now built and signed
  alongside the service binaries — adopters need it to verify
  signed audit anchors, so it travels with every release.
- Signing uses Sigstore **keyless OIDC** via the GitHub Actions
  `id-token: write` permission. The signing identity is bound
  to `https://github.com/ajamous/aether/.github/workflows/release.yml@refs/tags/<tag>`
  + the GitHub OIDC issuer; an attacker pushing a tag from a
  fork cannot forge a matching identity.
- Release notes auto-generated by `softprops/action-gh-release`
  now carry a "Verifying this release" preamble that points at
  the new doc.

`docs/sas-sm/release-verification.md`:
- Adopter-facing procedure for verifying a release. Concrete
  cosign commands for each artifact type (binary, SBOM,
  SHA256SUMS), the expected `--certificate-identity` value, the
  air-gapped verification path (the bundle is self-contained;
  no outbound network needed at verify time), and an honest
  "what this pipeline does NOT do" section calling out the
  remaining gaps: container image signing, SLSA Level 3
  attestations, and Helm-chart-time verification (Kyverno or
  Connaisseur are the layered-on answers, called out by name).
- Cross-linked from `common-findings.md` and the MkDocs nav.

Status row updates:
- "SAS-SM evidence templates" row: now lists "release
  verification (cosign + Sigstore)" alongside the other docs.

Dependabot configuration (`.github/dependabot.yml`):
- Closes a supply-chain hygiene gap: every previous dependency
  bump was hand-rolled. Dependabot now surfaces upstream releases
  automatically across four ecosystems and every `go.mod` /
  Dockerfile in the repository.
- 26 update entries: GitHub Actions (one grouped PR weekly), npm
  under `ui/admin/` (grouped by next/auth + types + lint +
  typescript), Docker base images (one PR per service's
  Dockerfile), and Go modules (one PR per `go.mod` — Go
  workspaces don't share a `go.sum`, so Dependabot needs an
  entry per module).
- Cadence per ecosystem spreads the maintainer load across the
  week: GitHub Actions + npm on Monday 09:00 UTC, Docker on
  Tuesday, Go modules on Wednesday. Patch + minor Go bumps are
  grouped per module; major bumps land as their own PRs.
- Security advisories are surfaced same-day regardless of the
  weekly cadence (GitHub default).
- **No auto-merge.** Aether ships SAS-SM-relevant code; every
  Dependabot PR runs the full CI suite (build, test, helm lint,
  terraform validate, openapi lint, conformance, prometheus
  rules, grafana JSON, postgres + softhsm integration) and gets
  maintainer review before merge.
- Commit-message prefixes follow the project's `<scope>(deps): `
  convention so the bot's PRs sort cleanly alongside human PRs
  in the changelog.

CONTRIBUTING.md gains a "Dependency updates" section with the
ecosystem matrix, the no-auto-merge policy, and pointers for
manual dependency PRs to use the `dependencies` label so they
sort alongside the bot's queue.

Grafana dashboard refresh
(`deployments/observability/grafana/dashboards/`):
- Closes a gap created by recent counter-emitting PRs that didn't
  update the dashboards. The `aether_gateway_ratelimit_rejected_total`
  (rate-limit PR) and `aether_gateway_admin_unauthorized_total`
  (OIDC PR) metrics had no panels; the audit chain had no
  dedicated dashboard at all.
- `aether-gateway-es2plus.json` is broadened to cover all three
  gateway auth gates: ES2+ mTLS 401s, rate-limit 429s by class,
  and admin OIDC 401s by reason. Title becomes "Aether — Gateway
  Auth Gates"; file name + uid stay `aether-gateway-es2plus` for
  backward compatibility with existing bookmarks and
  kube-prometheus-stack ConfigMap names. 9 panels (was 3),
  organised under three collapsible rows.
- New `aether-audit.json` dashboard surfaces the audit chain's
  integrity (`aether_audit_chain_ok`), length, append rate, and
  the scrape-up signal — so on-call can tell `silent` apart from
  `green`. Includes a step-after timeseries of chain integrity
  so any drop to 0 is unmissable. 6 panels.
- Both new dashboards' PromQL targets reference only metrics
  that exist in the codebase (verified by grep across
  services/* and pkg/*); same honest-status posture as the
  existing dashboards.
- Observability bundle README updated: 3 → 4 dashboards.
- Top-level Status row "Observability bundle" updated: now
  lists "four Grafana dashboards (overview, HSM, gateway auth
  gates, audit chain)".
- CI's `grafana-dashboards` job (jq-parse + uid/title/panel
  count check) runs unchanged; both new dashboards pass.

Auditor CLI for signed anchors (`tools/aether-verify-anchor/`):
- Closes the loop opened by the previous PR. Signed timeline
  anchors are only useful if auditors can verify them offline;
  audit-retention.md described the procedure but provided no
  tool. This PR ships one.
- Single-file Go CLI (`tools/aether-verify-anchor/main.go`) that:
  1. Loads a PEM-encoded ECDSA P-256 public key (PKIX or
     extracted from a CERTIFICATE).
  2. Loads a JSON anchor from a path or stdin.
  3. Cross-checks that the DER `signed_payload` decodes to fields
     matching the JSON shape (catches bucket-tampering after
     signing).
  4. SHA-256-hashes the `signed_payload` and ECDSA-verifies the
     signature against the public key.
  5. Optionally cross-checks the anchor's `length` and
     `tail_hash` against operator-supplied values (e.g. from a
     fresh Postgres restore).
- Stdlib-only on purpose: an auditor's verifier should be
  reproducible from a single Go file with zero third-party
  dependencies, and re-implementable in Python or any language
  straight from the SGP.22 §H.5 + asn1.Marshal layout.
- Exit codes are stable for monitor scripts: `0` OK, `1` bad
  input, `2` bad signature / DER-vs-JSON mismatch, `3`
  replay cross-check mismatch.
- 9 unit tests cover the happy path, missing args, tampered
  signed_payload, tampered signature, wrong pubkey, replay match,
  replay length mismatch, replay tail-hash mismatch, unsupported
  signature_alg, missing signed_payload, malformed pubkey PEM.
- New `make verify-anchor` target builds
  `bin/aether-verify-anchor`. Workspace gains the new module.
- audit-retention.md updated with a worked example: the operator
  builds the CLI, runs it against the daily anchor + the
  published pubkey, and optionally pipes a `psql ... encode(hash,
  'hex')` into `--against-tail-hash` for the step-4 replay check.
- 3 conformance harness cases added (happy, tampered triplet,
  replay triplet — using regex run patterns to bundle related
  tests into single cases). 59 total now (was 56).

Status row updates:
- `services/audit`: now lists the auditor CLI alongside the
  `/v1/anchor` endpoint.
- Conformance harness: 56 → 59 cases.

Audit signed timeline anchors (`services/audit/internal/anchor/`,
`/v1/anchor`):
- The audit retention runbook (audit-retention.md) calls for a
  daily cron that records `(length, tail_hash)` to the immutable
  offsite bucket as a "timeline anchor". Until now the recording
  was operator-trusted only — anyone with append rights to the
  bucket could fabricate an anchor.
- New `services/audit/internal/anchor/` package: an `Anchor`
  ASN.1 SEQUENCE `{timestamp, length, tail_hash}` with DER
  marshal/unmarshal and a `Sign` helper that hashes the DER
  with SHA-256 and asks hsm-broker to ECDSA-sign with the
  audit-anchor key. Same DER+SHA256+ECDSA pattern used by
  smdp-plus and smds, kept as a separate package because the
  audit anchor key has a distinct role + lifecycle from the
  SM-DP+ identity hierarchy (rotated yearly on its own ceremony
  cadence).
- 4 unit tests cover the DER round-trip, every validate()
  rejection (empty/short tail hash, negative length, zero
  timestamp), and signing end-to-end through a fake hsm-broker
  that returns a real ECDSA-P256 signature.
- New `GET /v1/anchor` handler. Lab default: returns
  `{length, tail_hash, timestamp}` JSON. Production
  (`--hsm-broker` + `--anchor-key` set): same shape plus
  `signed_payload` (DER) + `signature` (DER ECDSA) +
  `signature_alg` ("ECDSA-SHA-256"). Empty chain returns
  `length=0` with an all-zero tail hash — same convention the
  chain itself uses for the first entry's prev_hash.
- 2 server-level integration tests: lab mode returns no
  signature fields; signed mode round-trips through a fake
  hsm-broker, asserts the signed_payload DER-decodes to fields
  matching the JSON, and verifies the ECDSA signature against
  the broker's public key.
- `cmd/audit` gains `--hsm-broker` + `--anchor-key` flags.
  Server warns at startup when anchors are unsigned — same
  explicit-default-off-with-warning pattern as the gateway's
  mTLS, rate-limit, and OIDC gates.
- audit-retention.md updated: the daily cron now fetches
  `/v1/anchor` (instead of computing the anchor manually), and
  the auditor's offline verification procedure is spelled out
  (read signed_payload, SHA-256, ECDSA-Verify against the
  published audit-anchor public key, replay against a fresh
  Postgres restore).
- 4 conformance harness cases added: DER round-trip, signing
  end-to-end, lab unsigned response, signed response. 56 total
  now (was 52).

Status row updates:
- `services/audit`: now lists "signed timeline anchors at
  `/v1/anchor` (ECDSA-SHA-256 over DER-encoded `(timestamp,
  length, tail_hash)` SEQUENCE; opt-in via `--hsm-broker`)".
- audit README's status table gains the row.
- Conformance harness: 52 → 56 cases.

Gateway OpenAPI 3.1 spec (`services/gateway/api/v1/openapi.yaml`):
- Hand-written OpenAPI 3.1 spec covering both gateway surfaces:
  `/gsma/rsp2/es2plus/*` (mTLS-gated, SGP.22 §5.4) and `/v1/*`
  (OIDC-gated admin). Schemas for DownloadOrder, ConfirmOrder,
  the proxied templates/certs/SM-DS/eIM views, RFC 7807 Problem
  responses, and the per-reason 401/429 semantics inherited from
  the existing rate-limit and OIDC middleware. Security schemes
  capture both auth gates so a generated client gets the right
  wire shape out of the box.
- `services/gateway/api/v1/openapi.go` embeds the YAML via
  `go:embed`. The server registers `GET /v1/openapi.yaml` which
  bypasses the OIDC gate (same shape as `/v1/health` and
  `/metrics`) so operators and CLI tooling can discover the API
  without authenticating first. The API surface itself stays
  gated.
- `oidc.shouldEnforce` updated to bypass `/v1/openapi.yaml`
  alongside `/v1/health`. Server-level test drives the spec
  endpoint with the OIDC verifier wired in and asserts a 200
  with no Bearer.
- `services/gateway/api/v1/redocly.yaml` carries lint config
  that turns off two rules that don't fit SAS-SM use cases
  (`operation-4xx-response` on infra probes; `no-server-example.com`
  since `localhost` is the genuine lab listen address). Everything
  else follows Redocly's recommended profile.
- New `openapi-lint` job in `.github/workflows/ci.yml` runs
  `npx @redocly/cli lint --max-problems 0` on every PR and
  asserts the embed compiles via `go build ./api/...`.
- `TestGateway_OpenAPISpec` and
  `TestGateway_OpenAPI_BypassesOIDC` drive the wired endpoint
  end-to-end through the routing layer (content-type, structural
  shape, OIDC bypass). Both added to the conformance harness;
  52 cases now (was 50).
- Gateway README gains an "OpenAPI" section with example
  `oapi-codegen` (Go) and `openapi-typescript` (TypeScript)
  client-generation invocations.

Status row updates:
- Gateway README: "OpenAPI 3 spec generation — Not started" →
  "Implemented".
- Conformance harness: 50 → 52 cases.

HSM vendor configuration documentation (`docs/sas-sm/hsm-vendors.md`):
- Resolves a contradiction in the honest-status posture: the
  README claimed "Cloud HSM backends — Not started" while the
  SoftHSM package godoc said the same code path serves AWS
  CloudHSM, GCP Cloud HSM, Azure Managed HSM, Thales Luna, and
  Utimaco SecurityServer. Both can't be true; the right answer
  is somewhere between.
- New `docs/sas-sm/hsm-vendors.md` documents the per-vendor
  plumbing each cluster needs: `.so` path, slot/token init,
  PIN handling, known quirks. Sections for SoftHSM v2 (lab),
  AWS CloudHSM, GCP Cloud HSM (via Cloud KMS PKCS#11), Azure
  Managed HSM (via the pkcs11-azure shim — Azure ships no
  first-party PKCS#11 module), Thales Luna Network HSM, and
  Utimaco SecurityServer.
- Each vendor section is concrete: lib paths, slot conventions,
  client-install steps, the chart values that wire it in, and
  the known quirks that surface against the Aether broker
  (e.g. SoftHSM not supporting `CKD_SHA256_KDF`; Cloud KMS
  PKCS#11 being read-mostly so key gen happens out of band;
  Azure shim treating AAD tokens as "PIN"; Utimaco serialising
  Sign per logical CPU).
- A "What 'Implemented' honestly means" section spells out the
  caveat: we exercise the PKCS#11 backend end-to-end against
  SoftHSM v2 in CI, but do NOT claim "tested against AWS
  CloudHSM in CI" — running against a real cluster is
  expensive, and the SAS-SM auditor will run their own cluster
  acceptance test as part of accreditation. The
  hardware-in-the-loop bench stays the explicit follow-up.

`hsm-broker` plumbing:
- `--backend=pkcs11` is added as the preferred name; the
  historical `--backend=softhsm` is kept as a backward-compat
  alias. The error message lists both. Flag help and package
  godoc updated to point at `docs/sas-sm/hsm-vendors.md`.
- All existing tests pass; CI's SoftHSM integration job runs
  unchanged (it still uses `softhsm` for compatibility).

Status row updates:
- Top-level "Cloud HSM backends": "Not started" → **"Implemented
  (PKCS#11)"** with the honest caveat that per-vendor hardware
  verification is a follow-up bench.
- `services/hsm-broker` Status row: "AWS CloudHSM / GCP / Azure
  / Thales / Utimaco backends — Not started" → "Implemented via
  the PKCS#11 backend" with the same caveat.
- "SAS-SM evidence templates" row: now lists "HSM vendor
  configuration" alongside the other docs.
- MkDocs nav: new "HSM vendor configuration" entry between
  "Key ceremony" and "RBAC and SoD".
- `common-findings.md` cross-references `hsm-vendors.md`.

UI: forward Bearer tokens to the gateway (`ui/admin/`):
- Auth.js v5 JWT callback now captures the IdP's `id_token` on
  sign-in and threads it through the session callback so it's
  available in server-side fetches. Mirrors the standard
  Auth.js pattern but explicitly chooses `id_token` (the
  IdP-signed proof of who's logged in) over `access_token` —
  the gateway gates on identity, not on a separate API token,
  and aligning to `id_token` keeps every server-to-gateway
  call bound to the originating session.
- `lib/api.ts` gains `gatewayAuthHeaders(url)`: server-side
  helper that reads `auth()` and returns
  `Authorization: Bearer ${session.idToken}` only when the
  destination is the gateway URL. AUDIT and CERTMGR direct
  fetches do NOT receive the Bearer — those services have no
  OIDC gate and forwarding the token to them would leak the
  operator's `id_token` to services that don't need it.
  Wrapped in try/catch so a malformed session doesn't crash
  server rendering.
- `tsc --noEmit` and `next build` clean.
- README documents the wiring: the UI captures `id_token` on
  sign-in, forwards on gateway calls only, audience must match
  the gateway's `--oidc-audience`. Includes the canary advice:
  a sustained `aether_gateway_admin_unauthorized_total{reason="wrong_audience"}`
  typically means the audience is misconfigured on one side.

This closes the operational loop opened by the previous gateway
OIDC PR: enabling `--oidc-issuer` on the gateway no longer breaks
the UI.

Gateway OIDC for `/v1/*` admin paths (`services/gateway/internal/oidc/`):
- New `oidc` package: stdlib-only JWT verifier with discovery,
  JWKS cache, and middleware. The package is intentionally
  stdlib-only — a third-party JWT library would be a non-trivial
  supply-chain surface for the SAS-SM-relevant admin auth gate.
- Verifier discovers `jwks_uri` via the issuer's
  `/.well-known/openid-configuration`. Supported algorithms are
  **RS256** and **ES256**. HS\*, RS384, RS512, ES384, ES512, and
  EdDSA are deliberately rejected — admin tokens must be
  asymmetrically signed by the IdP.
- JWKS is cached for 5 minutes by default; an unknown `kid`
  triggers an immediate refresh regardless of TTL.
- Validation: signature, `iss == configured issuer`, `aud
  contains configured audience`, `exp > now` (no clock skew
  tolerance — IdP and gateway clocks should be NTP-aligned), and
  `nbf <= now` if present. Both string-form and array-form `aud`
  claims are accepted per RFC 7519 §4.1.3.
- Middleware applies only to `/v1/*` paths. `/v1/health` and
  `/metrics` bypass unconditionally so kube-probes and
  Prometheus scrape unauthenticated. Anything outside `/v1/*`
  (notably `/gsma/rsp2/*`) bypasses too — that surface has its
  own auth (mTLS + rate-limit). Verified subject + claims are
  threaded into request context for downstream handlers.
- 14 unit tests cover both happy paths (RS256 + ES256), every
  rejection path (HS256, expired, not-yet-valid, wrong issuer,
  wrong audience, unknown kid, tampered signature, malformed
  in five flavours), array-form audience, JWKS refresh on
  unknown kid, and the alternate `NewWithJWKS` constructor.
  Plus 3 server-level integration tests that drive the wired
  middleware end-to-end through the routing layer.
- New `aether_gateway_admin_unauthorized_total{reason}` counter
  exposed on `/metrics`, pre-loaded with the 10 documented
  rejection reasons so the hot path is lock-free. Mirrors the
  ES2+ 401 counter shape.
- New main flags `--oidc-issuer` and `--oidc-audience`. Both
  must be set to enable; lab default is disabled. Discovery
  runs at startup with a 10-second timeout so a flaky IdP
  doesn't make the gateway hang. Server warns at startup when
  off — same explicit-default-off-with-warning pattern as the
  mTLS and rate-limit gates.
- Helm chart: new `gateway.oidc.issuer` + `gateway.oidc.audience`
  values (and a parallel `gateway.rateLimit.rps` + `.burst`,
  filling in a previously-undocumented gap in the chart). Both
  default to disabled. `helm lint` + `helm template` clean.
- Conformance harness gains 9 cases (one new "Admin" family)
  exercising RS256 happy, ES256 happy, HS256 rejection, expired,
  wrong issuer, wrong audience, tampered signature, plus the
  two server-level integration tests; 50 total now (was 41).

Status row updates:
- `services/gateway`: Partial → **Implemented**. The "OIDC
  pending" caveat is closed; the gateway now has TLS + ES2+
  mTLS + rate-limit + OIDC, all opt-in.
- Conformance harness: 41 → 50 cases across 9 families.

Terraform (`deployments/terraform/azure/`):
- Azure reference deployment as IaC, third in the cloud trifecta
  alongside AWS and GCP. Stands up the topology described in
  the new `docs/sas-sm/reference-azure.md` in one
  `terraform apply`:
  - VNet + delegated AKS subnet + delegated Postgres-flexible
    subnet + private DNS zone for Postgres + NSG with
    default-deny inbound + VNet Flow Logs (365-day retention,
    Traffic Analytics on) shipped to Log Analytics.
  - Two user-assigned managed identities (`audit`, `hsm-broker`)
    bound to chart ServiceAccounts via Workload Identity
    federated credentials (manual post-deploy step — chart
    release name not known at apply time).
  - AKS **private cluster** (no public API endpoint), Workload
    Identity + OIDC issuer enabled, Container Insights via OMS
    agent, Azure Policy add-on, autoscaler, deletion protection
    via Azure's resource-group-prevent-deletion safeguard.
  - **Azure Database for PostgreSQL Flexible Server** version
    16, zone-redundant HA, geo-redundant 35-day backups, AAD
    authentication on, public network access disabled, log_
    connections + log_disconnections on. Master password
    generated by Terraform and stored in a separate Key Vault
    (RBAC authorization, purge protection, soft-delete 90d).
  - **Azure Key Vault Managed HSM** (FIPS 140-3 Level 3) —
    provisioned only. Activation requires the security-domain
    ceremony documented in `docs/sas-sm/key-ceremony.md`. The
    Reader role is wired for the hsm-broker managed identity at
    the control plane; full local-RBAC role assignments
    ("Managed HSM Crypto User") happen at the data plane after
    the ceremony.
  - **Storage account** with **GZRS** replication, immutable
    container with a **Locked** time-based retention policy
    (Compliance-grade — once locked, retention can be extended
    but never shortened, and the policy itself cannot be
    removed). TLS 1.2 minimum, HTTPS-only, shared access keys
    disabled, default OAuth authentication, public network
    access disabled, versioning + change feed + 365-day
    soft-delete on.
- Production posture (zone-redundant HA, immutable storage,
  private AKS, Flow Logs, Managed HSM Level 3, AAD auth) is
  pinned by the module rather than exposed as variables.
- Submodule layout under `modules/` (network, iam, aks,
  postgres, hsm, storage); canonical wiring under `examples/full/`
  with a `next_steps` output that prints the exact
  `az identity federated-credential create` calls and security-
  domain ceremony commands the operator needs.
- CI's `terraform-validate` matrix grows from `[aws, gcp]` to
  `[aws, gcp, azure]` so all three modules stay fmt+validate
  clean on every PR. The Azure module passes `terraform fmt
  -recursive -check` and `terraform validate` clean with no
  deprecation warnings against azurerm 4.x.

SAS-SM section: new `reference-azure.md`:
- Mirrors the AWS and GCP reference docs section-for-section so
  adopters can compare the three shapes side by side. Documents
  the Azure-specific calls: Workload Identity federated
  credentials (vs IRSA on AWS / Workload Identity bindings on
  GCP), Managed HSM security-domain ceremony (vs CloudHSM
  cluster activation / Cloud KMS HSM key ring), GZRS-replicated
  Storage with locked time-based immutability (vs S3 Object
  Lock Compliance / GCS Bucket Lock).
- Cross-linked from `reference-aws.md`, `reference-gcp.md`,
  `reference-onprem.md`, and the MkDocs nav. Status table row
  "SAS-SM evidence templates" updated to mention Azure.
- Top-level Status row "Terraform modules" updated: AWS + GCP
  + Azure all Implemented. Closes the "Azure not yet started"
  caveat carried since the AWS module landed.

Gateway rate limiter (`services/gateway/internal/ratelimit/`):
- New `ratelimit` package: per-source-IP token bucket. Allow()
  is lock-only-on-the-shared-map (no goroutines, no eviction
  janitor — the BSS-facing surface has bounded source
  cardinality and operators with broader threats fork the
  upstream LB rules). Rate + burst configurable; nil limiter
  is the explicit "disabled" form so middleware short-circuits
  cleanly when off.
- Path classifier rate-limits only `/gsma/rsp2/*` (es2plus and
  es9plus); admin paths (`/v1/*`, `/metrics`, `/health`) bypass
  unconditionally — same exemption shape as the ES2+ mTLS gate.
- Source key is `RemoteAddr` (the source as seen by the
  gateway). Defaults that ignore X-Forwarded-For are intentional;
  trusting forwarded headers without a trusted-proxy CIDR list is
  how rate-limit bypasses happen.
- 8 unit tests cover burst exhaustion, token refill (with an
  injected clock), source-independence, nil-limiter passthrough,
  invalid-config rejection, RemoteAddr parsing for IPv4/IPv6/unix,
  path classification, middleware end-to-end (429 + Retry-After +
  reporter callback fires), and admin path bypass.
- Server wired: new `Config.RateLimitRPS` + `Config.RateLimitBurst`,
  middleware ordered BEFORE the mTLS gate so a flood of cert-less
  requests can't burn CPU on chain checks. New `aether_gateway_
  ratelimit_rejected_total{class}` counter exposed on `/metrics`,
  pre-loaded with `class ∈ {es2plus, es9plus}` so the hot path is
  lock-free.
- New main flags `--rate-limit-rps` + `--rate-limit-burst`.
  Disabled by default; gateway warns at startup when off.
- `TestGateway_RateLimit_RejectsAfterBurst` drives the wired
  middleware end-to-end through the routing layer: burst→reject,
  Retry-After header set, admin paths stay 200, counter visible
  on `/metrics`. Added to `tools/conformance/runner/catalogue.go`
  (the harness now runs 41 cases).
- New `AetherGatewayRateLimited` Prometheus alert (sev-3, fires
  on > 1 req/s of 429s on /gsma/rsp2/* sustained for 5 min).
  Counts as the 12th alert in the bundle.
- README rows tightened: gateway moves from "OIDC and rate-limit
  pending" to "OIDC pending"; Observability bundle row goes from
  11 to 12 alerts; conformance harness from 40 to 41 cases.
- Gateway README documents the SAS-SM-relevant default
  ("source = RemoteAddr; trusting X-Forwarded-For without a
  trusted-proxy list is how bypasses happen").

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
