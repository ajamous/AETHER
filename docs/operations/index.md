# Operations

Boring documentation. The kind that matters at 3am.

## Runbooks

A runbook is a recipe for handling a specific operational situation.
We treat them as code: tested, versioned, kept current with what the
software actually does.

| Runbook                                                       | Status   |
| ------------------------------------------------------------- | -------- |
| Cert rotation (lab)                                           | Planned  |
| Cert rotation (production, with HSM)                          | Planned  |
| HSM failover (cloud HSM)                                      | Planned  |
| HSM failover (on-prem)                                        | Planned  |
| Profile bulk import                                           | Planned  |
| Audit log export                                              | Planned  |
| Upgrade procedure (per minor release)                         | Planned  |
| Backup and recovery                                           | Planned  |
| Disaster recovery                                             | Planned  |
| Incident response                                             | Planned  |
| Rate limit tuning                                             | Planned  |

## Observability

| Signal              | Mechanism                                      |
| ------------------- | ---------------------------------------------- |
| Logs                | Structured JSON, OTLP ingest                   |
| Metrics             | Prometheus (RED + USE + protocol-specific)     |
| Traces              | OpenTelemetry (OTLP), W3C Trace Context        |
| Audit               | `services/audit` hash chain, exportable        |

Default Grafana dashboards ship as JSON next to the Helm chart. One
click import.

## Performance baselines

Once we have running services, we will publish:

- BPP generation rate per CPU core (reference hardware)
- ES9+ session throughput
- HSM call rate ceiling per backend
- Memory and CPU footprint per service at idle and at load

We update these every release. They live alongside the runbooks.

## Postmortems

Public, blameless. Once we have incidents to write about, they go
under `docs/operations/postmortems/`. The point is for the next
adopter to learn what we already paid the price to learn.
