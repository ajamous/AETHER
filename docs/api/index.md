# API

Aether exposes three categories of API:

- **ES2+** — the GSMA-defined interface that BSS systems use to drive
  the SM-DP+. This is a SOAP-flavored XML protocol per SGP.22.
- **REST** — Aether-native, used by the admin UI and any operator
  integrations. OpenAPI 3 spec generated from code.
- **gRPC** — internal service-to-service. Proto definitions under
  `pkg/api/proto/` once they exist.

## Status

| API     | Status     | Reference                          |
| ------- | ---------- | ---------------------------------- |
| ES2+    | Planned    | SGP.22 §5.6 (DownloadOrder, etc.)  |
| REST    | Planned    | OpenAPI 3 spec, served at `/openapi.json` |
| GraphQL | Planned    | For the UI's flexible queries      |
| gRPC    | Planned    | `pkg/api/proto/`                   |

## Authentication

- **ES2+**: mTLS. Carrier BSS presents a client cert; the gateway
  validates against the configured trust store.
- **REST/GraphQL (UI)**: OIDC. In lab mode a built-in dev provider is
  available; in production, plug in your IdP.
- **gRPC (internal)**: mTLS with a private CA managed by the
  deployment, plus per-service identity.

## Error handling

All HTTP-based APIs return RFC 7807 `application/problem+json`
responses on error. ES2+ follows the SGP.22-defined fault codes.

## Versioning

External APIs follow semantic versioning. Backward-incompatible
changes are gated by a major version bump and announced one minor
release in advance.
