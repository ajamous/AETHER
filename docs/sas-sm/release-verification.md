# Verifying an Aether release

Every Aether GitHub release ships with cosign signatures over
every binary, both SBOMs (SPDX and CycloneDX), and the
`SHA256SUMS` file. Signatures are produced via **Sigstore
keyless OIDC** during the GitHub Actions release job — the
ephemeral signing certificate is bound to the workflow's
`https://github.com/ajamous/aether/.github/workflows/release.yml@refs/tags/<tag>`
identity and recorded in the public Rekor transparency log.

Adopters with SAS-SM accreditation responsibilities are expected
to verify before deploying. The SAS-SM auditor will check that
the deployed binaries match the Aether release artifacts and
that the signing identity is the project's release workflow.

## What you get per release

For each artifact in `dist/` the release attaches:

| File                    | What it is                                         |
| ----------------------- | -------------------------------------------------- |
| `<artifact>`            | The binary or SBOM itself                          |
| `<artifact>.sig`        | Detached cosign signature (raw)                    |
| `<artifact>.pem`        | Ephemeral signing-cert chain (X.509 PEM)           |
| `<artifact>.cosign.bundle` | Sigstore bundle: signature + cert + Rekor entry |

Plus a single `SHA256SUMS` (with its own `.sig`/`.pem`/`.bundle`)
that lets you fall back to checksum verification if you don't
trust Sigstore.

## Verification — the simple path

```bash
# Install cosign once (https://docs.sigstore.dev/cosign/installation/).
TAG=v0.x.y
TAG_ENC=${TAG//\//%2F}
EXPECTED_IDENTITY="https://github.com/ajamous/aether/.github/workflows/release.yml@refs/tags/${TAG}"
EXPECTED_ISSUER="https://token.actions.githubusercontent.com"

# Verify a binary.
cosign verify-blob \
    --bundle aether-gateway.cosign.bundle \
    --certificate-identity "$EXPECTED_IDENTITY" \
    --certificate-oidc-issuer "$EXPECTED_ISSUER" \
    aether-gateway
# → "Verified OK" on success; non-zero exit on failure.

# Verify the SBOM you intend to load into your supply-chain
# tooling.
cosign verify-blob \
    --bundle sbom.cdx.json.cosign.bundle \
    --certificate-identity "$EXPECTED_IDENTITY" \
    --certificate-oidc-issuer "$EXPECTED_ISSUER" \
    sbom.cdx.json

# Verify SHA256SUMS, then cross-check every artifact in one shot.
cosign verify-blob \
    --bundle SHA256SUMS.cosign.bundle \
    --certificate-identity "$EXPECTED_IDENTITY" \
    --certificate-oidc-issuer "$EXPECTED_ISSUER" \
    SHA256SUMS
sha256sum -c SHA256SUMS
```

`cosign verify-blob` returns exit 0 on success and a non-zero
exit on any failure (signature mismatch, identity mismatch,
expired certificate, missing Rekor entry). Wire it into your
release-promotion pipeline.

## What "identity" means and why we pin it

Sigstore keyless certificates encode the GitHub Actions
workflow that produced them, the tag that triggered it, and the
OIDC issuer (GitHub's). By passing
`--certificate-identity` and `--certificate-oidc-issuer` you are
asserting:

1. The signature came from a workflow file at the path
   `.github/workflows/release.yml`, in the `ajamous/aether`
   repository.
2. The workflow ran in response to the `<tag>` tag (NOT a
   branch push, NOT a workflow_dispatch, NOT a fork).
3. The OIDC token was issued by GitHub's identity provider.

If any of those conditions fail (for example, an attacker pushed
a tag from a fork and the build came from a forked workflow
file), `cosign verify-blob` rejects the signature.

The SAS-SM auditor will look at the verification log — the
`--certificate-identity` value is the auditable proof of
"these binaries came from THIS workflow at THIS tag", which is
the supply-chain integrity claim the Standard expects.

## Air-gapped verification

The Sigstore bundle file (`*.cosign.bundle`) carries everything
needed for offline verification: the ephemeral certificate
chain, the signature, and the Rekor inclusion proof. You do
NOT need outbound network access at verification time — the
bundle is self-contained against the Sigstore root keys, which
ship with cosign.

For air-gapped sites, mirror the cosign binary alongside the
release tarball; nothing else is required.

## What the release pipeline does NOT do

- **Container image signing.** The release pipeline today
  builds binaries and SBOMs only; container images are built by
  a separate workflow (or by adopters out of band) and signed
  there. Wiring image signing into the same flow is a focused
  follow-up.
- **In-toto attestations.** Beyond the SBOM and the cosign
  signature, we do not currently produce SLSA provenance
  attestations. The `cosign sign-blob --bundle` form covers the
  artifact-to-build link that the Standard typically asks for;
  full SLSA Level 3 provenance is a focused follow-up.
- **Verification at install time.** The Helm chart does not
  verify cosign signatures on the images it pulls. Adopters who
  need this should layer
  [Kyverno](https://kyverno.io/policies/cosign-image-signing/)
  or [Connaisseur](https://github.com/sse-secure-systems/connaisseur)
  in front of the chart.

## Cross-references

- [SBOM in CycloneDX format](https://cyclonedx.org/) — the
  format OWASP Dependency-Track and most CSAF stacks consume.
- [SBOM in SPDX format](https://spdx.dev/) — what most US
  government tooling expects.
- [Sigstore](https://www.sigstore.dev/) — the keyless signing
  ecosystem.
- [Rekor transparency log](https://search.sigstore.dev/) —
  search for any signature by SHA-256.
- [Aether `.github/workflows/release.yml`](https://github.com/ajamous/aether/blob/main/.github/workflows/release.yml)
  — the source of truth for what the pipeline does.
- [Aether dependency policy](https://github.com/ajamous/aether/blob/main/CONTRIBUTING.md#dependency-updates)
  — how upstream dependencies enter the project in the first
  place.
- [Common audit findings](common-findings.md) — including the
  hardware-in-the-loop bench and the supply-chain follow-ups
  this doc references.
