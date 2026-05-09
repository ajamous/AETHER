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
| Rate limiting / RBAC                             | Not started  |
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

## Wire format

ES2+ in production is SOAP-over-HTTPS per SGP.22. We expose the
SGP.22 message shapes as JSON today for development; the SOAP envelope
adapter is a focused follow-up that lives behind the same handlers.
