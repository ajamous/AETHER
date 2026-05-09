# 0002. Go for service code

- Status: Accepted
- Date: 2026-05-09
- Authors: Ameed Jamous <a.jamous@telecomsxchange.com>

## Context

Aether's services do mTLS, gRPC, HTTP, ASN.1, cryptography, and
PostgreSQL. They run as long-lived daemons in container orchestration.
They need to be operable by SREs at small carriers who may not have
specialized language expertise.

Candidate languages were: Go, Rust, Java/Kotlin, Python.

## Decision

Service code is written in Go (1.22+). Tooling and operator scripts
may be Python (3.11+). The admin UI is TypeScript on Next.js. ASN.1
codec generation produces C via asn1c, with a Go shim where needed.

## Alternatives considered

**Rust.** Memory-safe, excellent crypto ecosystem, fast. We rejected
Rust as the service-tier language for two reasons. First, the telecom
infrastructure pool of engineers familiar with Rust is small relative
to Go; we want the codebase to be approachable to SREs at small
carriers. Second, Rust's compile-time complexity slows iteration on a
project still finding its footing. We are not ruling out individual
performance-critical components in Rust later (e.g., a hot ASN.1 path),
but the default is Go.

**Java/Kotlin.** Strong in enterprise telecom, but carries baggage
(JVM, build complexity, framework sprawl) that conflicts with the
"single binary, easy to deploy" goal. Most importantly, the
operator-runs-this-on-a-laptop story is harder.

**Python.** Excellent for scripting and tooling. Not a serious
candidate for high-throughput servers handling cryptographic state.
We use Python for `tools/` and `pysim-bridge`, where its strengths
shine.

## Decision details

- Go 1.22+, with a workspace (`go.work`) at the repo root
- Module path convention: `github.com/ajamous/aether/<area>` where
  `<area>` is `services/<name>`, `pkg/<name>`, or `tools/<name>`
- Linting: `golangci-lint` with the strict config in `.golangci.yml`
- gRPC for internal RPC, REST + OpenAPI 3 for external APIs
- Postgres via `pgx`; Redis via `redis/go-redis`; NATS via `nats.go`
- HSM via `github.com/miekg/pkcs11`
- Telemetry via OpenTelemetry SDK, exporting OTLP

## Consequences

Positive:

- Single static binary per service. Container images are small. Easy
  for an SRE to run anywhere.
- Deterministic builds. Fast CI.
- Concurrency model is a natural fit for protocol state machines.
- Large pool of contributors familiar with the language.

Negative:

- Go's `encoding/asn1` does not support every constraint used in
  SGP.22 ASN.1 modules; we depend on `asn1c` for primary codec
  generation. This is a known cost; see ADR 0003 area for ASN.1
  decisions when written.
- Generics are still relatively new; we will be conservative about
  using them in places that affect readability.

Neutral:

- We have to maintain Go conventions (error handling, context plumbing,
  package naming) to a high standard. The linter helps.

## References

- [Effective Go](https://go.dev/doc/effective_go)
- [Go workspaces](https://go.dev/ref/mod#workspaces)
