# Aether observability bundle

Prometheus alert rules and Prometheus Operator manifests for an
Aether deployment. Two flavours, pick the one that matches your
cluster:

- **`prometheus/prometheus-rules.yaml`** — vanilla Prometheus
  rules file. Mount into your Prometheus pod via
  `rule_files:` in `prometheus.yaml`. No Prometheus Operator
  required.
- **`prometheus-operator/`** — `PrometheusRule` and
  `ServiceMonitor` CRDs for adopters running
  kube-prometheus-stack or the upstream Prometheus Operator.
- **`grafana/dashboards/`** — three dashboards (overview, HSM
  detail, gateway ES2+) backed by the same metrics the alerts
  use. See `grafana/README.md` for import instructions.

Pick one alert form (vanilla or Operator). The Grafana
dashboards are independent and load alongside either.

## Status

| Alert                           | Status        | Source metric                          |
| ------------------------------- | ------------- | -------------------------------------- |
| AetherAuditChainBroken          | Implemented   | `aether_audit_chain_ok` (services/audit `/metrics`) |
| AetherAuditMetricsScrapeFailing | Implemented   | `up{aether_component="audit"}`         |
| AetherCertExpiringSoon (<30d)   | Implemented   | `aether_cert_expiry_days` (services/certmgr `/metrics`) |
| AetherCertExpiringUrgent (<7d)  | Implemented   |  same                                  |
| AetherCertExpired               | Implemented   |  same                                  |
| AetherServiceDown               | Implemented   | `kube_deployment_status_replicas_unavailable` (kube-state-metrics) |
| AetherServiceCrashLooping       | Implemented   | `kube_pod_container_status_restarts_total` (kube-state-metrics) |
| AetherHSMBrokerUnhealthy        | Implemented   | `aether_hsm_broker_ready` (services/hsm-broker `/metrics`) |
| AetherPostgresConnectionsExhausted | Implemented | `pg_stat_activity_count` (postgres-exporter) |
| AetherHSMSignLatencyP99 (>250ms) | Implemented | `aether_hsm_sign_duration_seconds_bucket` (services/hsm-broker `/metrics`) |
| AetherES2PlusUnauthorizedSpike  | Implemented (per-reason: no_tls / no_client_cert / chain_invalid) | `aether_gateway_es2plus_unauthorized_total` (services/gateway `/metrics`) |

All 11 alerts are wired to live metrics emitted by Aether services
or by standard exporters (kube-state-metrics, postgres-exporter).
This satisfies the must-have alerts called out by
`docs/sas-sm/reference-aws.md` and `docs/sas-sm/incident-response.md`.

## Validation

```
promtool check rules prometheus/prometheus-rules.yaml
```

CI runs this on every PR (see `.github/workflows/ci.yml` →
`prometheus-rules-lint`).

## Helm wiring

The Aether Helm chart can render the Prometheus Operator manifests
in-cluster when adopters set:

```yaml
observability:
  prometheusOperator:
    enabled: true
```

When unset (default), neither manifest is created. The chart never
installs the Operator itself — it expects the Operator to be
running already.

## What this bundle does NOT include

- **kube-state-metrics or postgres-exporter setup.** These are
  prerequisites for `AetherServiceDown` and
  `AetherPostgresConnectionsExhausted` respectively. Standard
  installs ship them by default; verify yours does.
- **Alertmanager routing.** Where alerts go (PagerDuty, Slack,
  email) is operator policy. The runbook URLs in each alert's
  annotations point at
  `docs/sas-sm/incident-response.md` so whoever pages can find
  the procedure quickly.

## Cross-references

- [Reference AWS deployment](../../docs/sas-sm/reference-aws.md) §"Observability"
- [Incident response runbook](../../docs/sas-sm/incident-response.md)
- [Audit log retention](../../docs/sas-sm/audit-retention.md)
