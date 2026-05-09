# Aether Grafana dashboards

Three dashboards backed by metrics that Aether services already
emit on `/metrics` (plus `kube-state-metrics` and
`postgres-exporter` for the cluster-level views).

| Dashboard                                   | UID                       | Description                                           |
| ------------------------------------------- | ------------------------- | ----------------------------------------------------- |
| `aether-overview.json`                      | `aether-overview`         | Top-level health: audit chain, services, HSM, certs   |
| `aether-hsm.json`                           | `aether-hsm`              | HSM broker latency percentiles, throughput, heatmap   |
| `aether-gateway-es2plus.json`               | `aether-gateway-es2plus`  | ES2+ 401 spike breakdown by reason                    |

Every panel queries metrics that the alert rules in
[`../prometheus/prometheus-rules.yaml`](../prometheus/prometheus-rules.yaml)
also use, so the dashboards and the alerts stay in lock-step.
The same panels light up red when their corresponding alert is
firing.

## Datasource

Each panel uses `${DS_PROMETHEUS}` as the datasource UID. On
import:

- Grafana UI: select your Prometheus datasource at the import
  prompt.
- Provisioning (`grafana.ini` / `dashboards.yaml`): set
  `DS_PROMETHEUS` in the dashboard's `__inputs` or use a
  fixed-UID provisioning entry.

## Importing

### Grafana UI

1. **+** → **Import**
2. Upload one of the JSON files
3. Pick your Prometheus datasource

### Provisioning (recommended)

Drop the JSON files into a path your Grafana provisioner watches.
Example `dashboards.yaml`:

```yaml
apiVersion: 1
providers:
  - name: aether
    folder: Aether
    type: file
    options:
      path: /var/lib/grafana/dashboards/aether
```

Then mount the contents of this directory at
`/var/lib/grafana/dashboards/aether`.

### kube-prometheus-stack ConfigMap

If you run kube-prometheus-stack, you can render each dashboard
as a labelled ConfigMap so the sidecar picks it up
automatically:

```bash
kubectl -n monitoring create configmap aether-overview \
  --from-file=aether-overview.json=deployments/observability/grafana/dashboards/aether-overview.json
kubectl -n monitoring label configmap aether-overview \
  grafana_dashboard=1
```

(Repeat for each dashboard.)

## Validation

CI parses every JSON file with `jq` to catch syntax errors. Run
the same check locally:

```bash
for f in deployments/observability/grafana/dashboards/*.json; do
  jq -e . "$f" >/dev/null && echo "ok: $f"
done
```

Schema-level validation against the Grafana dashboard model is
not in CI: the schema is loosely versioned and panel-type-specific,
so a structural check would either be too strict (rejecting
forward-compatible dashboards) or too loose (accepting anything).
The dashboards are expected to load on Grafana 10.x and later.

## What this set does NOT include

- **A "service map" / dependency graph dashboard.** The Aether
  services don't yet emit OpenTelemetry traces, so the data isn't
  there to draw it from.
- **Capacity-planning dashboards.** Grafana counts of capacity
  metrics that span months are dependent on the operator's
  retention policy. Adopters who care can fork these and add
  long-window panels.
- **A copy of every Prometheus alert as a panel.** The three
  dashboards above cover the operationally important signals;
  the rest live in Alertmanager and the runbook.

## Cross-references

- [Prometheus alert rules](../prometheus/prometheus-rules.yaml)
- [Incident response runbook](../../../docs/sas-sm/incident-response.md)
- [Reference AWS deployment](../../../docs/sas-sm/reference-aws.md) §"Observability"
