# services/gateway

Aether's API gateway. Single entry point for:

- ES2+ over HTTPS, used by upstream BSS systems to drive provisioning
- REST + (planned) GraphQL, used by the admin UI

## Status

| Piece                                            | Status       |
| ------------------------------------------------ | ------------ |
| HTTP server                                      | Implemented  |
| ES2+ DownloadOrder (§5.6.1)                      | Skeleton     |
| ES2+ ConfirmOrder (§5.6.2)                       | Skeleton     |
| ES2+ CancelOrder (§5.6.3)                        | Skeleton     |
| ES2+ ReleaseProfile (§5.6.4)                     | Skeleton     |
| ES2+ HandleNotification                          | Skeleton     |
| REST: list/inspect templates                     | Implemented (proxy) |
| HTTPS listener                                   | Implemented (`--tls-cert` / `--tls-key`) |
| mTLS for ES2+ (path-scoped)                      | Implemented (`--es2plus-client-ca`); `/v1/*` admin paths stay unchallenged |
| OIDC auth on `/v1/*`                             | Implemented (`--oidc-issuer` + `--oidc-audience`); RS256 + ES256, JWKS cache, /v1/health and /metrics bypass; `aether_gateway_admin_unauthorized_total{reason}` on `/metrics` |
| Rate limiting (per-source token bucket on `/gsma/rsp2/*`) | Implemented (`--rate-limit-rps` + `--rate-limit-burst`); admin paths bypass; `aether_gateway_ratelimit_rejected_total{class}` on `/metrics` drives the AetherGatewayRateLimited alert |
| RBAC                                             | Not started  |
| OpenAPI 3 spec                                   | Implemented (`services/gateway/api/v1/openapi.yaml` — embedded via `go:embed`, served at `/v1/openapi.yaml` which bypasses OIDC for client discovery; CI gate via `redocly lint`) |

## TLS / mTLS

The lab default is plain HTTP — no client-cert checks. Production
deployments enable TLS by setting `--tls-cert` and `--tls-key`, and
enable mTLS for ES2+ by additionally setting `--es2plus-client-ca`
to a PEM bundle of CAs whose-issued client certs the gateway will
accept.

The mTLS gate is **path-scoped**: a request to `/gsma/rsp2/es2plus/*`
without a verified client cert chain is rejected with `401`, while
the operator UI and `/v1/*` proxies continue to work over the same
listener without a client cert. This separation matters because the
two surfaces use different auth realms (BSS-side mTLS vs operator
OIDC); they share the listener and certificate.

The Helm chart wires this via two Secret references on the
`gateway` block:

```yaml
gateway:
  tls:
    serverSecret: aether-tls          # tls.crt + tls.key
    es2plusClientCASecret: aether-bss-ca   # ca.crt
```

Both Secrets are operator-supplied; the chart does not generate
TLS material.

## Rate limiting

The public `/gsma/rsp2/*` surface is protected by a per-source-IP
token-bucket limiter, configured via two flags:

```
--rate-limit-rps <float>    steady-state requests/sec per source
--rate-limit-burst <int>    bucket capacity per source
```

Both must be set (and > 0 / >= 1 respectively) to enable. Lab
default is disabled; the gateway warns at startup when off.

The limiter keys on `RemoteAddr` (the source as seen by the
gateway). Behind an L7 LB, that's the LB's IP, so the limit
aggregates all inbound traffic. That is the safe default —
trusting `X-Forwarded-For` without a trusted-proxy CIDR list is
how rate-limit bypasses happen. Operators who want per-real-client
limiting should configure the upstream LB.

Admin paths (`/v1/*`, `/metrics`) bypass the limiter unconditionally,
matching the mTLS gate's exemption shape.

Rejections increment `aether_gateway_ratelimit_rejected_total{class}`
on `/metrics`, with `class` ∈ `{es2plus, es9plus}`. The
`AetherGatewayRateLimited` alert in `deployments/observability/`
fires on sustained high reject rate.

## OIDC

The admin `/v1/*` surface is gated by Bearer-token OIDC, configured
via two flags:

```
--oidc-issuer <url>      OIDC issuer URL (must match `iss` in admin tokens)
--oidc-audience <id>     Required `aud` on admin tokens
```

Both must be set to enable. Lab default is disabled; the gateway
warns at startup when off, just like the mTLS and rate-limit gates.

The verifier:

- Discovers `jwks_uri` via the issuer's
  `/.well-known/openid-configuration`. Tokens signed with **RS256**
  or **ES256** are accepted; HS\*, RS384, RS512, ES384, ES512, EdDSA
  are deliberately rejected — admin tokens must be asymmetrically
  signed.
- Caches the JWKS for 5 minutes. Unknown `kid`s trigger an immediate
  refresh.
- Verifies signature, `iss == configured issuer`, `aud contains
  configured audience`, and `exp/nbf` against the wall clock with
  no skew tolerance (IdP and gateway clocks should be NTP-aligned).

`/v1/health` and `/metrics` bypass unconditionally — they have to,
so kube-probes and Prometheus can scrape unauthenticated. Anything
outside `/v1/*` (notably `/gsma/rsp2/*`) bypasses OIDC too; that
surface has its own auth (mTLS + rate-limit).

Rejections increment `aether_gateway_admin_unauthorized_total{reason}`
on `/metrics`, with `reason` ∈ `{no_token, malformed,
unsupported_alg, unknown_kid, bad_signature, wrong_issuer,
wrong_audience, expired, not_yet_valid, jwks_fetch_failed}`.

The implementation is stdlib-only on purpose: a third-party JWT
library would be a non-trivial supply-chain surface for the
SAS-SM-relevant admin auth gate.

## OpenAPI

The gateway publishes a hand-written OpenAPI 3.1 spec covering the
ES2+ surface (mTLS-gated) and the `/v1/*` admin surface
(OIDC-gated). The spec lives at
[`services/gateway/api/v1/openapi.yaml`](api/v1/openapi.yaml) and
is embedded into the binary via `go:embed`, so a running gateway
serves it at:

```
GET /v1/openapi.yaml
```

This endpoint **bypasses the OIDC gate** (same shape as
`/v1/health` and `/metrics`) so operators and CLI tooling can
discover the API without authenticating first. The API surface
itself stays gated.

CI lints the spec with [Redocly CLI](https://redocly.com/docs/cli/)
on every PR (`.github/workflows/ci.yml` → `openapi-lint`). Lint
config under `services/gateway/api/v1/redocly.yaml` turns off
two rules that don't fit the SAS-SM use case (the
`operation-4xx-response` recommendation on infra probes, and
`no-server-example.com` since `localhost` is the genuine lab
listen address). Everything else uses Redocly's recommended
profile.

Generate a client:

```bash
# Go client
oapi-codegen -package aetherclient services/gateway/api/v1/openapi.yaml \
    > internal/aetherclient/client.go

# TypeScript client
npx openapi-typescript services/gateway/api/v1/openapi.yaml \
    -o ui/admin/lib/aetherclient.ts
```

## Wire format

ES2+ in production is SOAP-over-HTTPS per SGP.22. We expose the
SGP.22 message shapes as JSON today for development; the SOAP envelope
adapter is a focused follow-up that lives behind the same handlers.
