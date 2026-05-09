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

Both forms encode the same alerts. Pick one.

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
| AetherHSMSignLatencyP99 (>250ms) | Pending instrumentation | `aether_hsm_sign_duration_seconds_bucket` — not yet emitted |
| AetherES2PlusUnauthorizedSpike  | Pending instrumentation | `aether_gateway_es2plus_unauthorized_total` — not yet emitted |

The 9 implemented rules satisfy the must-have alerts called out
by `docs/sas-sm/reference-aws.md` and
`docs/sas-sm/incident-response.md`. Two more rules are listed
in those docs but depend on metrics the services don't yet emit;
they're called out as pending so adopters know what's coming.

## Pending instrumentation

Two pieces of follow-up work to fully close the alerting gap:

1. **HSM Sign latency histogram** in `services/hsm-broker`.
   Adds `aether_hsm_sign_duration_seconds` (bucketed) to track
   p99 latency. Enables `AetherHSMSignLatencyP99` from the
   spec list.
2. **Gateway 401 counter** in `services/gateway`. Increments
   on every 401 returned from `/gsma/rsp2/es2plus/*` (the path
   the mTLS gate already enforces). Enables
   `AetherES2PlusUnauthorizedSpike`.

Both are < 30-line patches. They're deferred from this bundle
so the rules file ships only with metrics that actually exist.

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

- **Grafana dashboards.** Dashboard JSON would be unverifiable
  without a Grafana instance. The Operations team's bar is "alerts
  must fire correctly"; visual dashboards are a follow-up.
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
