# Aether Helm chart

Deploys the full Aether RSP stack: SM-DP+, SM-DS, eIM (planned),
profile-builder, certmgr, hsm-broker, audit, gateway, and the admin UI.
Plus an optional bundled Postgres for state.

## Status

| Concern                                          | Status        |
| ------------------------------------------------ | ------------- |
| Lab install (`helm install aether ./aether`)     | Implemented   |
| Production install (external Postgres + HSM)     | Implemented   |
| Ingress (gateway + UI)                           | Implemented   |
| Lab cert-init initContainer (auto-generate SGP.26 chain) | Implemented — opt-out via `certmgr.lab.certInit.enabled=false` |
| HA defaults (multi-replica)                      | Not started — replicas default to 1 |
| HPA / autoscaling                                | Not started   |
| NetworkPolicies                                  | Not started   |
| Bundled Grafana dashboards                       | Not started   |

The chart is sufficient for an MVNO to deploy what currently exists in
the platform. As the protocol surface fills in (SAIP codec, real BPP,
SGP.32 eIM), no chart changes should be required.

## Quick start

```bash
helm install aether ./deployments/helm/aether
kubectl port-forward svc/aether-aether-ui 3000:3000
open http://localhost:3000
```

## Lab cert chain

In lab mode (`certmgr.mode: lab`, the default), the certmgr pod
runs an `initContainer` that calls `certmgr --generate-lab=/certs`
on every pod start. The chain (CI root → EUM → DPtls/DPauth/DPpb,
ECDSA P-256, 24-hour validity) lands in an `emptyDir` volume that
the main certmgr container then reads at startup. No operator
pre-work; `helm install aether ./aether` is a true one-shot for
the lab.

Lab certs are deliberately ephemeral. Each pod restart mints a
new chain; the keys never persist to durable storage. This
matches what the lab cert mode is for (see ADR 0004): test
material that should not survive.

Opt out by setting `certmgr.lab.certInit.enabled: false` if you
want to pre-populate `/certs` from a different source (e.g. a
pre-built ConfigMap mounted via a values override).

## Production checklist

Override these values for any non-lab deployment:

```yaml
postgres:
  enabled: false              # bring your own
postgresUrl: "postgres://aether:secret@your-pg.internal:5432/aether?sslmode=require"

hsmBroker:
  backend: softhsm            # or external for a real PKCS#11 module
  pkcs11:
    libraryPath: /usr/lib/softhsm/libsofthsm2.so
    slot: "0"
    pinSecret:
      name: aether-hsm-pin    # must contain a 'pin' key

certmgr:
  mode: production
  trustStore: |
    -----BEGIN CERTIFICATE-----
    ...your GSMA CI root...
    -----END CERTIFICATE-----
  intermediates: |
    -----BEGIN CERTIFICATE-----
    ...your EUM intermediate(s)...
    -----END CERTIFICATE-----
  identitySecret:
    name: aether-identity-keys

ingress:
  enabled: true
  className: nginx
  host: rsp.your-mvno.com
  tls:
    enabled: true
    secretName: aether-tls
```

Then:

```bash
helm install aether-prod ./deployments/helm/aether -f values-prod.yaml
```

## What this chart does NOT do

- **It does not generate or store production private keys.** Identity
  keys (DPtls, DPauth, DPpb) come from your HSM. The chart references
  them by Secret name; you must populate those Secrets out-of-band as
  part of your key ceremony. See `docs/sas-sm/` for templates once
  that section lands.
- **It does not configure your TLS certificates or LB.** Bring your
  own ingress controller and cert-manager (or equivalent).
- **It does not bundle Postgres backups.** The PVC is durable but
  backups are an operator concern.
- **It does not configure RBAC for human operators.** That belongs in
  your cluster's auth stack.

## Default port map

| Component       | Port |
| --------------- | ---- |
| hsm-broker      | 8443 |
| certmgr         | 8444 |
| smdp-plus       | 8445 |
| profile-builder | 8446 |
| audit           | 8447 |
| smds            | 8448 |
| gateway         | 8080 |
| admin-ui        | 3000 |

## Testing

```
helm lint ./aether
helm template aether-test ./aether
helm template aether-test ./aether --set ingress.enabled=true --set ingress.host=aether.example
```

CI runs `helm lint` on every PR (see `.github/workflows/ci.yml`).
