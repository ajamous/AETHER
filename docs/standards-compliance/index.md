# Standards compliance

Aether implements GSMA Remote SIM Provisioning specifications. This
section is the running ledger of what we cover and where it lives in
the codebase.

## Specifications

| Spec     | Target version              | Coverage                               |
| -------- | --------------------------- | -------------------------------------- |
| SGP.21   | v3.x (Consumer Architecture) | Architectural alignment               |
| SGP.22   | v3.x (Consumer Technical)    | Full RSP protocol stack                |
| SGP.23   | latest (Conformance)         | Test harness integration               |
| SGP.24   | latest (Compliance Process)  | Audit-trail and evidence templates     |
| SGP.26   | latest (Test Certificates)   | Default lab mode                       |
| SGP.31   | latest (IoT Architecture)    | Architectural alignment                |
| SGP.32   | v1.x (IoT Technical)         | eIM, IPA-server, SM-DP+ IoT extensions |
| TCA SAIP | v2.x                         | Profile package generation             |
| ETSI TS 102.221, 3GPP TS 31.102 | current               | UICC/USIM file system in profiles      |

## Coverage matrices

- [SGP.22 coverage matrix](sgp22-matrix.md) — section by section,
  what is implemented, where, and what tests cover it
- SGP.32 coverage matrix — landing in Phase 4

## How traceability works

Every spec-implementing function carries a doc comment with the
relevant section reference, e.g.:

```go
// AuthenticateClient implements SGP.22 §5.7.5.
//
// The eUICC has signed the SM-DP+ challenge, returning a
// signed authentication token plus the eUICC's certificate
// chain. We verify the chain against our trusted CI roots,
// validate the signature, and return the matching profile
// metadata so the LPA can prompt the user.
func (s *Server) AuthenticateClient(...) (...) { ... }
```

A linter (planned, Phase 1) checks that public functions in the
service packages carry such a reference. The coverage matrix is
updated alongside any PR that touches protocol code.

This is a free RSP textbook for contributors. Read the codebase, learn
the spec.
