# Roadmap

The full plan, including philosophy and detailed task breakdowns, lives
in `docs/architecture/plan.md`. This document summarizes the phases and
links current work to it.

## Where we are

Work has run ahead of the phase order: Phase 1 is close to its
milestone and parts of Phases 2–7 have landed. No phase is called done
until its milestone is reproduced (see the disclaimers below). The
README status table is the per-component source of truth; this is the
per-phase view.

| Phase | State | What's left |
| ----- | ----- | ----------- |
| 0 — Foundation | Mostly done | SGP.22 ASN.1 modules are not vendored yet (`pkg/asn1/sgp22/modules/` is empty; vendoring GSMA spec text needs a licensing decision). CI builds, tests, lints, and builds every image; SBOMs are produced at release time only, and there is no container image scan |
| 1 — Consumer SM-DP+ MVP | In progress (current focus) | Every BPP layer is built and tested in software. For a real card: further SAIP ProfileElements (PE-PinCodes, PE-FileSystem, TCA §B framing), the per-segment AAD layout checked on hardware, and the device E2E. **Milestone not yet reached** |
| 2 — Admin UI | Partial | Read-only console with OIDC sign-in. No profile activation flow, live updates, or Storybook; no formal WCAG 2.1 AA audit (an automated axe-core pass is clean) |
| 3 — SM-DS | Partial | ES11 + ES12 discovery with signed ServerSigned1. Root / Alternative SM-DS roles and zero-touch activation not started |
| 4 — IoT (SGP.32) | Skeleton | eIM device registry and command queue. IPAe / IPAd flows, fleet UI, bulk ops not started |
| 5 — Production crypto | Partial | One PKCS#11 backend verified against SoftHSM; per-vendor configuration documented. Per-vendor hardware verification and cert rotation tooling pending; key ceremony is documented |
| 6 — Conformance & hardening | Partial | SGP.23 harness (92 cases) and DR runbook in place. No fuzzing yet; pen tests and the mock SAS-SM audit not started |
| 7 — Production references | Partial | Terraform (AWS, GCP, Azure), Helm chart, and SAS-SM evidence templates in place. No pilot deployments yet |
| 8 — Ecosystem | Not started | |

## Phases

### Phase 0 — Foundation

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
