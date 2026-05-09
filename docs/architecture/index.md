# Architecture

This section explains how Aether is structured: what the services do,
how they talk to each other, and why the boundaries are where they are.

## The big picture

```
┌──────────────────────────────────────────────────────────┐
│                  Aether Admin Console                    │
│           Next.js + React + Tailwind + shadcn/ui         │
└────────────────────────┬─────────────────────────────────┘
                         │
                ┌────────┴────────┐
                │   API Gateway   │   ES2+ for BSS, REST/GQL for UI
                └────────┬────────┘
                         │
   ┌────────┬────────────┼────────────┬─────────┐
   ▼        ▼            ▼            ▼         ▼
 SM-DP+   SM-DS         eIM       Profile     Audit
 (SGP.22) (SGP.22)   (SGP.32)     Builder    (immutable
                                  (SAIP)    hash chain)
   │        │            │            │         │
   └────────┴────────────┴────────────┴─────────┘
                         │
                ┌────────┴────────┐
                │   HSM Broker    │   PKCS#11 abstraction
                └────────┬────────┘
                         │
        ┌────────────────┼────────────────┐
        ▼                ▼                ▼
     SoftHSM         AWS / GCP /        On-prem
     (lab)           Azure HSM        Thales / Utimaco

     PostgreSQL          NATS           Prometheus
     (state)             (events)         + Loki
```

## Service boundaries

Each service has a single responsibility and a stable interface.

| Service              | Responsibility                                                |
| -------------------- | ------------------------------------------------------------- |
| `smdp-plus`          | SGP.22 ES8+/ES9+/ES10b application protocol; BPP generation   |
| `smds`               | SGP.22 ES11/ES12; event registration; root and alternative roles |
| `eim`                | SGP.32 IoT manager; IPAe and IPAd flows                       |
| `profile-builder`    | TCA SAIP v2.x profile package generation (UPP → PPP → BPP)    |
| `certmgr`            | Cert chain loading, rotation, expiry monitoring               |
| `hsm-broker`         | PKCS#11 façade for all crypto. Backends are pluggable.        |
| `audit`              | Append-only, hash-chained event log                           |
| `gateway`            | ES2+ ingress, REST/GraphQL for UI, mTLS, OIDC, RBAC           |

Internal communication uses gRPC. External APIs are REST + OpenAPI 3
(plus ES2+ for BSS, which is its own SOAP-flavored thing per spec).

## Data stores

- **PostgreSQL 16** — durable state (profiles, orders, certs, audit)
- **Redis 7** — RSP session state, ephemeral keys, rate limit counters
- **NATS JetStream** — internal event bus, audit log ingest

## The HSM abstraction

All sensitive crypto operations route through `hsm-broker`. It exposes
a small gRPC interface (`Sign`, `Decrypt`, `DeriveKey`,
`GenerateKeyPair`, `ListKeys`) and never returns private key material.

Backends are PKCS#11-conforming HSMs:

- SoftHSM v2 (lab default)
- AWS CloudHSM, GCP Cloud HSM, Azure Key Vault Managed HSM
- Thales Luna SA, Utimaco SecurityServer (on-prem)

This is what makes the same binary work in lab and production. See
[ADR 0003](adr/0003-pkcs11-abstraction.md) for the rationale and
[ADR 0004](adr/0004-lab-vs-prod-cert-mode.md) for how the lab/production
mode switch is implemented.

## Decision records

Every significant architectural choice is captured as an ADR. Read
them in order to understand not just what the system is, but why it
got that way:

- [ADR index](adr/index.md)
