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
| OIDC auth                                        | Not started  |
| Rate limiting (per-source token bucket on `/gsma/rsp2/*`) | Implemented (`--rate-limit-rps` + `--rate-limit-burst`); admin paths bypass; `aether_gateway_ratelimit_rejected_total{class}` on `/metrics` drives the AetherGatewayRateLimited alert |
| RBAC                                             | Not started  |
| OpenAPI 3 spec generation                        | Not started  |

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

## Wire format

ES2+ in production is SOAP-over-HTTPS per SGP.22. We expose the
SGP.22 message shapes as JSON today for development; the SOAP envelope
adapter is a focused follow-up that lives behind the same handlers.
