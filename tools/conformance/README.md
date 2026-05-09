# SGP.23 conformance harness

GSMA SGP.23 ("Test Specification") defines a battery of test
cases an SM-DP+, SM-DS, and LPA must pass to claim conformance.
Aether's commitment in the original plan is to align with SGP.23
where automatable, document where it isn't, and never paper over
gaps.

This directory holds:

- [`coverage/sgp23.md`](coverage/sgp23.md) — section-by-section
  coverage matrix mapping SGP.23 test cases to the Aether tests
  that exercise them, the manual test cases that need real
  hardware, and the scope-out items.
- [`runner/`](runner/) — a Go test runner that aggregates the
  existing per-service tests into a single `go test -tags=conformance`
  invocation. CI runs this on every PR; the `make conformance`
  target runs it locally.

## Status

| Piece                                              | Status        |
| -------------------------------------------------- | ------------- |
| SGP.23 coverage matrix                             | Implemented   |
| Conformance suite runner                           | Implemented   |
| `make conformance` target                          | Implemented   |
| CI gate                                            | Implemented   |
| Hardware-in-the-loop (real eUICC) test harness     | Not started — see `coverage/sgp23.md` §"Hardware tests" |
| LPA-side conformance                               | Out of scope — Aether is the server side; LPA conformance is a separate certification program |

## How to use this

Adopters preparing for a conformance audit:

1. Read [`coverage/sgp23.md`](coverage/sgp23.md). It maps every
   SGP.23 test family to which Aether code path covers it, what
   passes today, and what requires a real eUICC bench.
2. Run `make conformance` locally. Green means every automatable
   conformance test passes against the current build.
3. For the hardware-in-the-loop tests (a sysmoEUICC card, an
   Android device with a working LPA), follow the manual
   procedure documented in the matrix. Capture the LPA-side
   logs and the smdp-plus-side audit entries; both go in your
   conformance evidence pack.

## What this is not

- **Not an official conformance certification.** GSMA SGP.23
  certification is a paid programme run by approved test
  laboratories. This harness gives you the engineering
  confidence to walk into one of those labs; it doesn't
  substitute for the lab.
- **Not a substitute for production telemetry.** Conformance
  tests confirm the protocol behaviour at one moment in time.
  The observability bundle ([deployments/observability/](https://github.com/ajamous/aether/tree/main/deployments/observability))
  is what tells you the protocol behaviour stays correct in
  production.
- **Not a complete SGP.23 implementation.** The spec contains
  hundreds of test cases, many of which exercise real-eUICC
  behaviour (CAT commands, OTA, file-system layout, etc.) that
  Aether's server side never sees. The matrix is honest about
  scope.

## Cross-references

- [SGP.22 coverage matrix](https://github.com/ajamous/aether/blob/main/docs/standards-compliance/sgp22-matrix.md)
  — what we implement on the protocol side
- [SAS-SM gap analysis](https://github.com/ajamous/aether/blob/main/docs/sas-sm/gap-analysis.md)
  — security audit story
- [Common audit findings](https://github.com/ajamous/aether/blob/main/docs/sas-sm/common-findings.md)
  — what auditors look for
