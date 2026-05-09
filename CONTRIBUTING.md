# Contributing to Aether

Thanks for considering a contribution. This document covers the practical
mechanics of contributing. The character of the project — why it exists, how
it makes decisions, what it commits to — lives in
[GOVERNANCE.md](GOVERNANCE.md). Read that too. It is binding.

## Before you start

A short pre-flight checklist that saves time for everyone:

1. **Read the Status table in [README.md](README.md).** If a component is
   marked "Not started," there may not be enough scaffolding yet for the
   contribution you have in mind. Open an issue first to align.
2. **Check open issues and PRs.** Someone may already be working on it.
3. **For non-trivial work, open an issue first.** A short paragraph saying
   what you'd like to do is enough. We will respond quickly with a "yes,
   go for it" or a "let's talk first." This is to save your time, not gate
   it.
4. **Read the relevant spec section.** If you're touching protocol code,
   the SGP.22, SGP.32, or SAIP section reference belongs in your doc
   comment and your commit message. If you don't know which section, ask.

## Developer Certificate of Origin

Aether uses the [Developer Certificate of Origin](https://developercertificate.org/),
not a CLA. Every commit must be signed off:

```
git commit -s -m "your message"
```

This adds a `Signed-off-by: Your Name <you@example.com>` trailer. By signing
off, you are stating that you wrote the patch (or have the right to submit
it under the project's license). That's it. No rights assignment, no
corporate paperwork.

CI will reject PRs whose commits are missing the sign-off.

## Pull request process

1. Fork the repo and create a branch from `main`.
2. Make your change. Keep PRs focused — one logical change per PR. If you
   touch multiple services, prefer separate PRs unless they have to land
   together.
3. Add or update tests. New code without tests will be asked to add tests.
4. Run `make lint test` locally before pushing.
5. Open the PR. Fill in the template. Reference any related issue.
6. CI must be green. One maintainer review is required to merge.
7. We use a "merge when ready" workflow — maintainers will merge once
   review is approved and CI is green. We don't squash by default; clean
   commit history with sensible messages is preferred over a single
   squashed blob.

## What gets a PR merged

- **Tests.** New behavior has tests. Bug fixes have a regression test.
- **Spec references.** Protocol code carries `// Implements SGP.22 §X.Y.Z`
  comments. Auditors and learners both rely on this.
- **Operations docs.** New operator-facing features ship with runbook
  updates. No exceptions. If you're adding a new long-lived service or a
  new failure mode, the on-call doc gets updated in the same PR.
- **No marketing language.** No "world-class," no "industry-leading," no
  "enterprise-grade." The work speaks; we don't speak for the work.
- **Honest status.** If you ship something half-done, the README's Status
  table reflects it. Half-done is fine. Pretending it's done is not.
- **Time-to-first-success unharmed.** If your change makes `make lab-up`
  slower or harder, you need a really good reason. The 60-second lab is
  the most important commitment in the project.

## What does not get a PR merged

- Features held back from the OSS for a future commercial tier. There is
  no commercial tier. There won't be one for at least 24 months. See
  `GOVERNANCE.md`.
- Closed-source plugins or vendor-specific code paths that route around
  the PKCS#11 abstraction.
- Marketing prose in the codebase or docs.
- Code without an upstream spec reference (for protocol code) or a clear
  rationale (for everything else).

## Style and tooling

- **Go**: 1.22+. `gofmt`, `goimports`, `golangci-lint` all enforced in CI.
- **TypeScript**: Node 20+. Prettier and ESLint enforced.
- **Python**: 3.11+. Black, ruff. Reserved for tooling and scripts, not
  service code.
- **Commits**: Conventional Commits style is encouraged but not required.
  Clear, imperative-mood subject line is required. Bodies should explain
  *why*, not *what* — the diff already shows what.
- **Comments**: doc comments on exported symbols, period. No
  multi-paragraph essays inside function bodies. If a comment is needed,
  it's because the *why* is non-obvious.

## Building from source

```
make build       # builds all services
make test        # runs unit tests
make lint        # runs linters
make gen         # regenerates ASN.1 bindings, OpenAPI clients, etc.
make lab-up      # starts the local lab stack
make lab-down    # tears it down
```

See the relevant service README for service-specific instructions.

## Reporting security issues

Do not open a public issue. See [SECURITY.md](SECURITY.md) for the
private disclosure process.

## Recognition

Every release notes file lists every contributor by name, not just
maintainers. The `MAINTAINERS.md` file lists the people responsible for
specific subsystems. If you'd like your contribution to credit a different
name (e.g. you contributed via your employer but want personal credit, or
vice versa), tell us in the PR description.

We genuinely want Aether contributions to look good on your resume.
That's not a slogan; it's how careers in this corner of the industry
get built. Patches you contribute here are public, audited, and tied to
real protocol work — the kind of thing hiring managers actually weigh.

## Questions

Open a GitHub issue with the `question` label, or reach out on whichever
community channel ends up being the one we use (the README will be the
source of truth once it's settled).

Welcome aboard.
