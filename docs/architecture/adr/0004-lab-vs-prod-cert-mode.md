# 0004. Lab vs production certificate mode

- Status: Accepted
- Date: 2026-05-09
- Authors: Ameed Jamous <a.jamous@telecomsxchange.com>

## Context

GSMA RSP relies on a hierarchical PKI rooted at GSMA-controlled
Certificate Issuers. SM-DP+ identity certificates (DPtls, DPauth,
DPpb) are issued by CIs against a SAS-SM-accredited deployment. eUICCs
are issued by EUMs and chain to the same CI roots.

For development and testing, GSMA publishes SGP.26: a parallel
"test PKI" with its own roots, EUM certs, and SM-DP+ certs.
SGP.26-rooted eUICCs are physically real (sysmocom sells them) but
explicitly not for production traffic.

The promise we make to adopters is: same Aether binary in lab and
production. Only config and certs change. We need the cert handling
to make that promise concrete.

## Decision

The `certmgr` service has two operating modes, selected by config:

```yaml
certificate_mode: lab        # default
# or
certificate_mode: production
```

In `lab` mode:

- Trust store loads SGP.26 test CI roots from
  `test/fixtures/sgp26/ci-roots.pem` (vendored, public)
- SM-DP+ identity certs are SGP.26-issued test certs, also vendored
- eUICC certs are accepted only if they chain to SGP.26 roots
- The startup banner prints `MODE: LAB (SGP.26 test certs)` in
  unmistakable yellow

In `production` mode:

- Trust store loads CI roots from a path provided by the operator,
  pointing to the GSMA Production CI roots they obtained
- SM-DP+ identity certs are loaded from the HSM by PKCS#11 URI
  (`pkcs11:object=DPtls;type=cert`)
- eUICC certs are accepted only if they chain to a configured set
  of production CI roots
- The startup banner prints `MODE: PRODUCTION` in green

Mixing is forbidden. A production-mode SM-DP+ that sees an SGP.26
eUICC rejects it with an explicit error code, and vice versa. The
certmgr service refuses to start if it detects a mismatch between
the configured mode and the certs actually present in the trust
store.

## Alternatives considered

**Single mode with operator discipline.** Just trust the operator to
load the right certs. Rejected: too easy to ship test certs to
production by mistake, and the resulting failure modes (a real eUICC
not trusting our cert chain) are confusing. Explicit mode + refuse
to start on mismatch fails fast.

**A third "mixed" mode for lab work with real CI eUICCs.** Rejected:
this is a real edge case (developers who happen to have real eUICCs
on their desk) but enabling it as a first-class mode dilutes the
guarantee. They can run two stacks side by side.

**Per-tenant cert mode.** Useful at scale when one Aether deployment
serves multiple carriers, but premature now. Single-tenant per
deployment is fine for the foreseeable future. We can revisit when
multi-tenant lands.

## Consequences

Positive:

- The lab-to-production promise is enforced by the code, not just
  documented
- Mistakes fail at startup, not in mysterious downstream protocol errors
- Operator confidence: you can read the banner and know which
  PKI you're trusting

Negative:

- Operators upgrading from lab to production must consciously change
  config. We treat this as a feature, not a bug.
- Adding modes later (e.g., a future hybrid mode for mixed test-and-
  real eUICCs in a controlled environment) requires an ADR amendment.

## References

- GSMA SGP.26 (Test Certificate Specification)
- GSMA SGP.22 §4.5 (Certificates and PKI)
- [README cert mode example](../../../README.md)
