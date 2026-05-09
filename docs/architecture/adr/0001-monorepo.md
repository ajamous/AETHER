# 0001. Monorepo for all services

- Status: Accepted
- Date: 2026-05-09
- Authors: Ameed Jamous <a.jamous@telecomsxchange.com>

## Context

Aether is a multi-service system: SM-DP+, SM-DS, eIM, profile-builder,
certmgr, hsm-broker, audit, gateway, plus a Next.js admin UI, shared
Go libraries, ASN.1 schemas, deployment manifests, and documentation.

The two structural choices were:

- A single repository (monorepo) holding all of it
- A repository per service, with shared libraries split out further

## Decision

Aether is a monorepo. All services, shared libraries, the admin UI,
deployment manifests, documentation, and tooling live under one
top-level repo (`github.com/ajamous/aether`).

Go services use a `go.work` workspace so they can develop against
unreleased versions of shared packages. The UI lives under `ui/admin`
with its own `package.json`. Deployment manifests live under
`deployments/`. Docs live under `docs/`.

## Alternatives considered

**Multi-repo with versioned shared libraries.** Each service in its own
repo, depending on tagged versions of shared libraries. This is the
"microservices with semver dependencies" approach. We rejected it
because:

- It massively raises the cost of cross-cutting changes (e.g., adding
  a field to an ASN.1 type that several services use)
- It makes the project harder to read and to navigate as a learner
  ("which repo defines BPP?")
- It creates a permanent compatibility matrix problem
- The services are co-released and co-tested anyway; splitting them
  doesn't reflect their actual coupling

**Hybrid: monorepo for services, separate repos for `pkg/` libs.** Same
problems at smaller scale.

## Consequences

Positive:

- One `git clone` and you have the whole project
- Cross-cutting refactors land in single PRs and are atomic
- Standards traceability is centralized; the SGP.22 coverage matrix
  in `docs/standards-compliance/` references files everywhere with
  no version-skew anxiety
- New contributors find their bearings faster
- CI is simpler; one workflow tests the whole thing

Negative:

- Repo gets larger over time; we will eventually need shallow-clone
  guidance for low-bandwidth contributors
- Build tooling has to handle multi-language polyglot (Go, TS, Python)
- Code ownership has to be enforced by `MAINTAINERS.md` conventions
  and `CODEOWNERS`, not by repo boundaries

Neutral:

- Forks need to vendor or fork the whole thing rather than cherry-pick
  a single service. This is acceptable; people who want to fork a
  single service can extract what they need under Apache 2.0.

## References

- [The benefits and challenges of monorepos](https://research.google/pubs/why-google-stores-billions-of-lines-of-code-in-a-single-repository/)
- Kubernetes, Terraform, and Linux all run as monorepos. The
  arguments against monorepos are mostly about scale we will not hit
  for years, if ever.
