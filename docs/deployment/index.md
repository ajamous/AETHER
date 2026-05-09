# Deployment

How to run Aether in different environments. Each topology will have
its own page once the corresponding manifests land.

## Topologies

| Topology              | Status   | Use case                                              |
| --------------------- | -------- | ----------------------------------------------------- |
| Local lab (Docker Compose) | Phase 1 | Laptop, learning, CI integration tests             |
| Single-node Kubernetes | Phase 5 | Small private 5G or research deployment              |
| HA Kubernetes (Helm)  | Phase 7  | MVNO production, multi-AZ                            |
| AWS GSMA-certified region | Phase 7 | US-West / US-East / Paris regions for SAS-SM        |
| GCP SAS-SM-eligible region | Phase 7 | europe-west / us-central                            |
| On-prem with Thales / Utimaco HSMs | Phase 7 | Carriers preferring on-prem key custody     |

## Common decisions

Whatever topology you pick, you will need to decide:

- **Cert mode**: lab (SGP.26) or production (CI). See
  [ADR 0004](../architecture/adr/0004-lab-vs-prod-cert-mode.md).
- **HSM backend**: SoftHSM (lab), cloud HSM, or on-prem HSM.
- **PostgreSQL hosting**: managed (RDS, Cloud SQL) or self-hosted.
- **Audit log retention**: default is 3 years immutable; longer if
  your jurisdiction requires it.
- **Observability stack**: Aether emits OTLP. Plug into whatever you
  already run (Prometheus + Grafana + Loki, Datadog, New Relic, etc.).

## What ships

When a topology is `Implemented`, expect:

- A working manifest (compose file, Helm chart, Terraform module)
- A deployment runbook with concrete steps and example output
- A teardown procedure tested in CI
- An upgrade procedure with rollback steps
- Default observability dashboards (Grafana JSON)
- A list of known limitations

If any of those are missing, the topology is not `Implemented` yet.
See [philosophy principle 4](https://github.com/ajamous/aether/blob/main/GOVERNANCE.md): no feature ships
without operations documentation.
