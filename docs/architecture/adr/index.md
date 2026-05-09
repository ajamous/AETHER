# Architectural Decision Records

We document significant architectural choices as ADRs. Each ADR captures
context, the decision, alternatives considered, and consequences.

The format is a lightly customized version of the
[Michael Nygard ADR style](https://cognitect.com/blog/2011/11/15/documenting-architecture-decisions).

## How to propose an ADR

1. Copy [`template.md`](template.md) to `NNNN-short-slug.md` (next number).
2. Fill it out. Set `Status: Proposed`.
3. Open a PR. Discuss.
4. On merge with consensus, change status to `Accepted` (or `Rejected`,
   in which case the ADR is still preserved as a record of the
   discussion).

## Index

| #     | Title                                                    | Status   |
| ----- | -------------------------------------------------------- | -------- |
| 0001  | [Monorepo for all services](0001-monorepo.md)            | Accepted |
| 0002  | [Go for service code](0002-go-for-services.md)           | Accepted |
| 0003  | [PKCS#11 HSM abstraction](0003-pkcs11-abstraction.md)    | Accepted |
| 0004  | [Lab vs production cert mode](0004-lab-vs-prod-cert-mode.md) | Accepted |
| 0005  | [Apache 2.0 license, no CLA](0005-apache-2-license.md)   | Accepted |
