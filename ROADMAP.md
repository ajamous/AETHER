# Roadmap

The full plan, including philosophy and detailed task breakdowns, lives
in `docs/architecture/plan.md`. This document summarizes the phases and
links current work to it.

## Where we are

**Phase 0 — Foundation.** Repo bootstrap, license, governance, CI
scaffolding, documentation skeleton, ASN.1 toolchain.

## Phases

### Phase 0 — Foundation (current)

- Repo bootstrap (license, CoC, contributing, security, governance)
- Build tooling (Makefile, Go workspace, lint configs)
- GitHub Actions CI (build, test, lint, SBOM, container scan)
- Documentation skeleton (MkDocs Material, ADR template, ADRs 1-5)
- ASN.1 toolchain (SGP.22 modules vendored, asn1c build step,
  Go bindings generated, round-trip tests)

### Phase 1 — Consumer SM-DP+ MVP

End-to-end profile download to a real Android device with a sysmoEUICC
test card.

- `pkg/crypto`: BSP, ECKA, ECDSA primitives
- `pkg/saip`: SAIP profile package codec
- `services/hsm-broker` with SoftHSM backend
- `services/certmgr` loading SGP.26 chain
- `services/smdp-plus` with ES9+ endpoints and BPP generation
- `services/profile-builder` emitting SAIP from YAML templates
- `services/gateway` with minimal ES2+ endpoints
- Docker Compose lab; `make lab-up`
- E2E test driving an Android device through profile download

Milestone: install a profile on a sysmoEUICC1-C2T from a self-hosted
Aether instance.

### Phase 2 — Admin UI

Operator UI so engineers stop SSH-ing into boxes for routine work.

- Next.js + auth, dashboard, profile inventory, activation flow
- Cert manager, HSM status, audit log viewer
- Real-time updates, Storybook, WCAG 2.1 AA

### Phase 3 — SM-DS

ES11/ES12, Root and Alternative roles, zero-touch activation.

### Phase 4 — IoT (SGP.32)

`services/eim`, IPAe and IPAd flows, fleet management UI, bulk ops.

### Phase 5 — Production crypto backends

AWS CloudHSM, GCP Cloud HSM, Azure Key Vault Managed HSM, Thales Luna,
Utimaco SecurityServer. Key ceremony tooling. Cert rotation playbook.

### Phase 6 — Conformance and hardening

SGP.23 test suite alignment. Property-based fuzzing. Internal then
external pen test. Multi-region active-active reference deployment.
Disaster recovery runbook. Internal mock SAS-SM audit dry-run.

### Phase 7 — Production reference deployments

Terraform modules (AWS GSMA-certified region, GCP, on-prem k8s). HA
Helm chart. Reference SAS-SM accredited topology. Compliance evidence
templates. First three pilot deployments documented.

Milestone: first MVNO using Aether passes SAS-SM audit (target 12+
months out from project start).

### Phase 8 — Ecosystem and governance

TSC formation. Plugin/extension API. LF Networking or CNCF sandbox
application. Conference talks. Yearly major releases with LTS branches.

## Honest disclaimers

- These are intentions, not commitments. We are an OSS project; we
  ship when it's ready.
- Any timeline given in the underlying plan is a target, not a
  promise. Real-world contribution rhythms drive the actual pace.
- We will not call a phase "done" until its milestone has been
  demonstrably reproduced by someone outside the maintainer team.
