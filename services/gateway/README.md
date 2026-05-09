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
| OIDC auth                                        | Not started  |
| mTLS for ES2+                                    | Not started  |
| Rate limiting / RBAC                             | Not started  |
| OpenAPI 3 spec generation                        | Not started  |

## Wire format

ES2+ in production is SOAP-over-HTTPS per SGP.22. We expose the
SGP.22 message shapes as JSON today for development; the SOAP envelope
adapter is a focused follow-up that lives behind the same handlers.
