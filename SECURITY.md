# Security Policy

## Reporting a vulnerability

Aether handles cryptographic key material and protocols that protect
real telecom subscribers. We take vulnerability reports seriously.

**Do not open a public GitHub issue for security vulnerabilities.**

Instead, report privately by one of:

- GitHub Security Advisory: use the "Report a vulnerability" button on
  the repository's Security tab. This is the preferred channel.
- Email: `security@aether-rsp.org` (PGP key to be published once the
  project has a hosted infrastructure footprint)

When you report, please include:

- A description of the issue and the impact you believe it has
- Steps to reproduce, or a proof-of-concept if you have one
- The version, commit hash, or branch you tested against
- Whether you have already disclosed this elsewhere

## What to expect

- Acknowledgement within 3 business days
- An initial assessment and severity rating within 10 business days
- Coordinated disclosure timeline agreed with the reporter
- Public credit in the release notes for the fix, unless you prefer
  to remain anonymous

We follow a 90-day disclosure window by default, shortened or extended
based on the severity and the reporter's preference.

## Supply-chain protections in place

The project ships four layers of supply-chain hygiene that
researchers and SAS-SM auditors should know exist:

| Layer                    | What it does                                                                                  | Where it lives                                            |
| ------------------------ | --------------------------------------------------------------------------------------------- | --------------------------------------------------------- |
| **Dependabot**           | Surfaces upstream dependency releases (and security advisories same-day) across Go modules, npm, Docker base images, and GitHub Actions | `.github/dependabot.yml`                                  |
| **govulncheck**          | Scans every Go module per PR for **reachable** known-vulnerability call paths (call-graph analysis, low false-positive rate) | `.github/workflows/ci.yml` → `govulncheck (per module)`   |
| **CodeQL**               | Static analysis on every PR + weekly schedule; Go-language coverage across the whole workspace | `.github/workflows/codeql.yml`                            |
| **Cosign + Sigstore**    | Every release artifact (binaries, both SBOMs, SHA256SUMS) is keyless-signed via the workflow's GitHub OIDC identity; bundles carry the Rekor inclusion proof | `.github/workflows/release.yml`, [release-verification.md](docs/sas-sm/release-verification.md) |

Plus a dual-format **SBOM** (SPDX + CycloneDX) attached to every
release so adopters' supply-chain stacks (CSAF/OWASP Dependency-Track
on the CycloneDX side; US-government tooling on the SPDX side) can
both consume the manifest natively.

The full release-verification procedure with concrete `cosign
verify-blob` invocations and the expected `--certificate-identity`
value lives at
[`docs/sas-sm/release-verification.md`](docs/sas-sm/release-verification.md).

The toolchain floor is documented in `go.work`; we bump it
whenever a fresh stdlib CVE makes the previous floor
unscannable, and govulncheck will fail the next PR until the
bump lands.

## Researcher-friendly scanning

Public security testing of unreleased branches and PRs is
welcome. The supply-chain stack above is built to be
researcher-friendly: govulncheck, CodeQL, and cosign verification
all run on every PR, so a researcher's findings show up in the
PR's checks tab without anyone having to ask the maintainers to
re-run anything.

When opening a PR with a fix, include:

- A reference to the GHSA, CVE, or GO-YYYY-NNNN identifier if one
  exists.
- The output of `make govulncheck` showing the issue resolved.
- A test case covering the broken behaviour, when feasible.

If the issue is too sensitive to discuss publicly, fall back to
the private-disclosure channel above.

## Supported versions

Once we have tagged releases, the last two minor versions of the most
recent two majors will receive security fixes for 18 months from
release. Until v0.1, only `main` is supported.

## Scope

In scope:

- All code in this repository
- The default Helm chart and Docker Compose configurations
- The reference Terraform modules
- The release pipeline (`.github/workflows/release.yml`) and its
  cosign keyless-OIDC signing identity
- The auditor verifier CLI (`tools/aether-verify-anchor/`)

Out of scope:

- Issues in third-party HSMs, cloud KMS services, or operating systems
  themselves (please report those upstream)
- Issues that require a privileged operator with legitimate access to
  perform actions they are already authorized to perform (those are
  policy concerns, not vulnerabilities)
- Denial-of-service findings against the lab profile, which is not
  hardened for hostile networks
- Findings in third-party container base images that are already
  tracked by the upstream project's own security process — Dependabot
  surfaces these on the next weekly window, and the project's
  reachability story for those CVEs is whatever govulncheck reports

## Hall of fame

Reporters who help us land a fix will be listed in the release notes
and in `docs/security/researchers.md` once that page exists, with
their permission.

## Cross-references

- [Dependency-update policy](CONTRIBUTING.md#dependency-updates) —
  cadence, grouping, no-auto-merge stance
- [Vulnerability scanning (govulncheck)](CONTRIBUTING.md#vulnerability-scanning-govulncheck) —
  local invocation + Go-floor bump policy
- [Release verification](docs/sas-sm/release-verification.md) —
  cosign + Sigstore procedure for adopters
- [Common audit findings](docs/sas-sm/common-findings.md) —
  including the supply-chain follow-ups (container image
  signing, SLSA Level 3, Helm-chart-time admission control via
  Kyverno or Connaisseur)
