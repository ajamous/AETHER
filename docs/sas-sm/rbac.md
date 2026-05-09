# RBAC and segregation of duties

A SAS-SM auditor will look for documented roles, mapped to humans,
with privileges proportionate to each role's job — and crucially,
no one human holding all the keys. This document defines four
roles for an Aether deployment and gives you ready-to-apply
Kubernetes RBAC manifests.

## The four roles

| Role | What they can do | What they cannot do |
| --- | --- | --- |
| `aether-operator` | Operate the platform: read all resources, restart pods, scale, view logs and metrics, run upgrades | Modify HSM config, modify audit log, modify cluster RBAC |
| `aether-key-custodian` | Hold one half of the HSM PIN; participate in key ceremonies; rotate keys per the ceremony procedure | Operate the platform day-to-day, read profile data, modify audit log |
| `aether-auditor` | Read everything — code, configs, audit log, RBAC bindings | Modify *anything* |
| `aether-incident-responder` | Same as operator, plus emergency cluster-admin escalation paths gated by break-glass approval | Default state: no elevated privileges |

The auditor will expect:

- No human in `aether-operator` is also in `aether-key-custodian`.
- The `aether-auditor` role is explicitly read-only.
- A break-glass procedure exists for incident response, with
  approval logging.
- All role assignments are reviewed quarterly.

## How to map humans

Maintain `docs/sas-sm/role-assignments.md` (operator-supplied; not
in this repo) with the current humans for each role. Review and
re-sign quarterly.

Example (cleartext is fine — names are not secret):

```
aether-operator:
  - alice@your-mvno.com    (since 2025-03-01)
  - bob@your-mvno.com      (since 2025-04-15)

aether-key-custodian:
  - carol@your-mvno.com    (since 2025-03-01)
  - dan@your-mvno.com      (since 2025-03-01)

aether-auditor:
  - elaine@your-mvno.com   (since 2025-06-01, internal compliance)

aether-incident-responder:
  - alice@your-mvno.com    (24/7 rota)
  - bob@your-mvno.com      (24/7 rota)
```

**Constraint check**: Alice and Bob cannot also appear in
`aether-key-custodian`. Carol and Dan cannot also appear in
`aether-operator`. Elaine cannot appear in any other role.

## Kubernetes RBAC manifests

Apply these to the namespace your Aether release runs in. Replace
`aether` with your release namespace if different.

```yaml
# Operator: read-write within the namespace, no cluster-admin.
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: aether-operator
  namespace: aether
rules:
  - apiGroups: [""]
    resources: [pods, pods/log, pods/exec, services, configmaps, secrets, persistentvolumeclaims, events]
    verbs: [get, list, watch, create, update, patch, delete]
  - apiGroups: [apps]
    resources: [deployments, statefulsets, replicasets]
    verbs: [get, list, watch, create, update, patch, delete]
  - apiGroups: [batch]
    resources: [jobs, cronjobs]
    verbs: [get, list, watch, create, update, patch, delete]
  - apiGroups: [networking.k8s.io]
    resources: [ingresses, networkpolicies]
    verbs: [get, list, watch]
  # Operators do NOT modify Roles or RoleBindings.
  - apiGroups: [rbac.authorization.k8s.io]
    resources: [roles, rolebindings]
    verbs: [get, list, watch]
---
# Auditor: read-only across the namespace.
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: aether-auditor
  namespace: aether
rules:
  - apiGroups: ["", apps, batch, networking.k8s.io, rbac.authorization.k8s.io]
    resources: ["*"]
    verbs: [get, list, watch]
  # Auditor can read Pod logs but cannot exec into them.
  - apiGroups: [""]
    resources: [pods/log]
    verbs: [get]
---
# Key custodian: read-only on the namespace, plus the right to
# update the hsm-broker pinSecret. The actual HSM PIN never lives
# in cluster secrets in production — these manifests set the
# Kubernetes-side guardrail; the HSM-side authentication is
# separate (PIN held by humans, not stored).
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: aether-key-custodian
  namespace: aether
rules:
  - apiGroups: [""]
    resources: ["*"]
    verbs: [get, list, watch]
  # Custodians can rotate the HSM PIN secret following a ceremony,
  # but ONLY that one secret name.
  - apiGroups: [""]
    resources: [secrets]
    resourceNames: [aether-hsm-pin]
    verbs: [update]
---
# Incident responder: default has no extra rights. The break-glass
# binding below escalates ONLY when activated.
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: aether-incident-responder
  namespace: aether
rules: []
---
# Break-glass: cluster-admin within the aether namespace.
# Bind a human to this only via a short-lived approval workflow
# (e.g. a 4-hour TTL token approved by both an operator AND an
# auditor). Activations land in the audit log.
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: aether-break-glass
  namespace: aether
rules:
  - apiGroups: ["*"]
    resources: ["*"]
    verbs: ["*"]
```

## Postgres GRANTs

The audit log's append-only contract is enforced by the application
today. SAS-SM auditors will want the database itself to refuse
UPDATE and DELETE on `audit_entries` for the application role.
Apply:

```sql
-- Run as the database superuser, after services/audit has applied
-- its schema.

REVOKE UPDATE, DELETE, TRUNCATE ON audit_entries FROM aether;

-- Auditors get read-only access.
CREATE ROLE aether_auditor WITH LOGIN PASSWORD '...';
GRANT CONNECT ON DATABASE aether TO aether_auditor;
GRANT USAGE ON SCHEMA public TO aether_auditor;
GRANT SELECT ON audit_entries TO aether_auditor;
```

The auditor can then run `/v1/verify` (via a service account that
proxies to the read-only role) and confirm chain integrity without
having any write capability.

## Quarterly review

Every quarter:

1. Pull the current `RoleBinding` and `ClusterRoleBinding`
   manifests from Git.
2. Diff against `docs/sas-sm/role-assignments.md`.
3. Confirm no role is held by a human who has changed jobs or
   left.
4. Confirm the operator/custodian split still holds.
5. The reviewer signs the `role-assignments.md` update.
6. The signed update goes into the audit pack.

## Evidence checklist

For the audit pack:

- [ ] The four `Role` manifests above, applied
- [ ] `RoleBinding`s mapping humans to the roles
- [ ] `role-assignments.md` showing current mappings, signed
- [ ] Postgres GRANTs as per the SQL above
- [ ] Break-glass approval workflow documentation + last
      activation log (or "no activations" attestation)
- [ ] Last quarterly review record

## What this template does NOT solve

- It does not configure your IdP. Map the K8s roles to OIDC
  groups via your IdP's group claims and a tool like Dex or
  Keycloak. That config is operator-supplied.
- It does not enforce the operator/custodian separation
  technically — that is a policy constraint. A human who is in
  both groups in your IdP can become both. The auditor will
  look for evidence that this doesn't happen.
- It does not cover BSS-side auth. Your BSS uses ES2+ over
  mTLS; that's a separate trust store on the gateway.
