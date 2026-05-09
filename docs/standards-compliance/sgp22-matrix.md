# SGP.22 coverage matrix

Tracks Aether's implementation of GSMA SGP.22 (Consumer RSP Technical
Specification). This is the truth of what we cover and what we don't.

Status legend:

- **Implemented** — code exists, has tests, is exercised in CI
- **Partial** — some endpoints/branches done, others stubbed
- **Planned** — on the roadmap, not yet started
- **Not in scope** — explicitly out of scope (with reason)

## §3 Procedures

| Section | Title                                          | Status   | Code location |
| ------- | ---------------------------------------------- | -------- | ------------- |
| §3.1    | Profile Download and Installation              | Planned  | `services/smdp-plus/` (Phase 1) |
| §3.2    | Profile enable / disable                       | Planned  | `services/smdp-plus/` (Phase 1) |
| §3.3    | Profile delete                                 | Planned  | `services/smdp-plus/` |
| §3.4    | Notifications                                  | Planned  | `services/smdp-plus/` |
| §3.5    | Discovery service procedures                   | Skeleton | `services/smds/` (in-memory event store; signing pending) |

## §4 Architecture

| Section | Title                                          | Status   | Code location |
| ------- | ---------------------------------------------- | -------- | ------------- |
| §4.1    | Components                                     | Aligned  | `docs/architecture/` |
| §4.5    | Certificates and PKI                           | Planned  | `services/certmgr/` |
| §4.6    | Identifiers                                    | Planned  | `pkg/saip/` |

## §5 Functions

| Section | Function                                       | Status   | Code location |
| ------- | ---------------------------------------------- | -------- | ------------- |
| §5.4.1  | ES2+ DownloadOrder                             | Skeleton | `services/gateway/` |
| §5.4.2  | ES2+ ConfirmOrder                              | Skeleton | `services/gateway/` |
| §5.4.3  | ES2+ CancelOrder                               | Skeleton | `services/gateway/` |
| §5.4.4  | ES2+ ReleaseProfile                            | Skeleton | `services/gateway/` |
| §5.4.5  | ES2+ HandleNotification                        | Skeleton | `services/gateway/` |
| §5.5.1  | ES12 RegisterEvent                             | Implemented (in-memory) | `services/smds/` |
| §5.5.2  | ES12 DeleteEvent                               | Implemented (in-memory) | `services/smds/` |
| §5.5.3  | ES11 GetEvents                                 | Implemented (in-memory) | `services/smds/` |
| §5.5.4  | ES11 AuthenticateClient                        | Skeleton (no signing)   | `services/smds/` |
| §5.6.1  | ES9+ InitiateAuthentication                    | Skeleton | `services/smdp-plus/` |
| §5.6.2  | ES9+ AuthenticateClient                        | Skeleton | `services/smdp-plus/` |
| §5.6.3  | ES9+ GetBoundProfilePackage                    | NotImplemented (501) | `services/smdp-plus/` (BPP pending SAIP codec) |
| §5.6.5  | ES9+ HandleNotification                        | Skeleton | `services/smdp-plus/` |
| §5.7.5  | ES9+ AuthenticateClient                        | Planned  | `services/smdp-plus/` |
| §5.7.6  | ES9+ GetBoundProfilePackage                    | Planned  | `services/smdp-plus/` |
| §5.7.7  | ES9+ HandleNotification                        | Planned  | `services/smdp-plus/` |
| §5.8    | ES8+ application protocol                      | Planned  | `services/smdp-plus/` |

## §6 Cryptographic primitives

| Section | Title                                          | Status   | Code location |
| ------- | ---------------------------------------------- | -------- | ------------- |
| §2.6    | BSP (Bound Profile Protection)                 | Planned  | `pkg/crypto/bsp/` |
| —       | ECKA over P-256                                | Planned  | `pkg/crypto/ecka/` |
| —       | ECKA over Brainpool P-256 r1                   | Planned  | `pkg/crypto/ecka/` |
| —       | ECDSA over P-256 / Brainpool P-256 r1          | Planned  | `pkg/crypto/ecdsa/` |

## Annexes

| Annex   | Title                                          | Status   | Code location |
| ------- | ---------------------------------------------- | -------- | ------------- |
| A       | RSP session state machine                      | Planned  | `services/smdp-plus/state/` |
| B       | ASN.1 data types                               | Planned  | `pkg/asn1/sgp22/` |
| H       | Certificate profiles                           | Planned  | `services/certmgr/` |

## How this matrix gets updated

Any PR that adds or changes coverage updates this file in the same
PR. The PR template prompts for it. CI is on the path to enforcing
that PRs touching `services/` or `pkg/` either update this file or
explain why not.
