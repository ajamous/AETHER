# SGP.23 coverage matrix

GSMA SGP.23 organises its test cases by the SGP.22 functional
area they exercise: ES2+, ES8+, ES9+, ES10, ES11, ES12, plus the
profile-package handling at §4 and the certificate-handling at
§5. This matrix maps each family to what Aether covers
automatically (existing Go tests), what requires hardware
(real eUICC bench), and what's out of scope (LPA-side, eUICC-
internal).

## How to read this table

- **Automated** — covered by `go test -tags=conformance ./tools/conformance/...`
- **Hardware** — needs a real sysmoEUICC card and an LPA-capable
  device (Android with the test LPA). Procedure documented in
  the §"Hardware tests" section below.
- **Out of scope** — LPA-side or eUICC-internal behaviour that
  the Aether server side never observes. Documented for
  completeness; you certify those separately or rely on your
  device vendor.
- **Pending** — automatable but blocked on a feature Aether
  hasn't shipped (typically: SAIP codec / real BPP).

## ES2+ — BSS to SM-DP+

| Test family                        | Coverage    | Aether test                                    |
| ---------------------------------- | ----------- | ---------------------------------------------- |
| ES2+/DownloadOrder shape           | Automated   | `services/gateway/.../server_test.go::TestGateway_DownloadOrder_HappyPath` |
| ES2+/DownloadOrder bad input       | Automated   | `services/gateway/.../server_test.go::TestGateway_DownloadOrder_RejectsEmpty` |
| ES2+/ConfirmOrder shape            | Automated   | `services/gateway/.../server_test.go` (proxy round-trip) |
| ES2+/CancelOrder                   | Automated   | gateway server suite                            |
| ES2+/ReleaseProfile                | Automated   | gateway server suite                            |
| ES2+/HandleNotification            | Automated   | gateway server suite                            |
| ES2+ mTLS rejection of bad client  | Automated   | `services/gateway/.../server_mtls_test.go::TestGateway_MTLS_ES2PlusRejectsUntrustedClientCert` |
| ES2+ mTLS rejection of no cert     | Automated   | `services/gateway/.../server_mtls_test.go::TestGateway_MTLS_ES2PlusRejectsNoClientCert` |
| ES2+ admin paths bypass mTLS gate  | Automated   | `services/gateway/.../server_mtls_test.go::TestGateway_MTLS_AdminPathsDoNotRequireClientCert` |
| Rate-limiter rejects /gsma/rsp2/* after burst | Automated | `services/gateway/.../server_test.go::TestGateway_RateLimit_RejectsAfterBurst` (admin paths bypass; rejected counter exposed on `/metrics`) |

## ES9+ — LPA to SM-DP+

| Test family                                | Coverage  | Aether test                                       |
| ------------------------------------------ | --------- | ------------------------------------------------- |
| InitiateAuthentication request shape       | Automated | `services/smdp-plus/.../server_test.go::TestInitiateAuthentication_HappyPath` |
| InitiateAuthentication challenge length    | Automated | `services/smdp-plus/.../server_test.go::TestInitiateAuthentication_RejectsEmptyChallenge` (extended for 16-byte enforcement) |
| ServerSigned1 ASN.1 round-trip             | Automated | `services/smdp-plus/internal/signing/signing_test.go::TestServerSigned1_RoundTrip` |
| ServerSigned1 signature verifies (E2E)     | Automated | `services/smdp-plus/.../server_signing_test.go::TestInitiateAuthentication_SignatureVerifies` |
| AuthenticateClient state progression       | Automated | `services/smdp-plus/.../server_test.go::TestAuthenticateClient_StateProgression` |
| AuthenticateClient unknown txid (404)      | Automated | `services/smdp-plus/.../server_test.go::TestAuthenticateClient_UnknownTID` |
| EuiccSigned1 chain validation              | Automated | `services/smdp-plus/internal/signing/euicc_test.go::TestVerify_HappyPath` plus 4 negative cases |
| EuiccSigned1 server-challenge replay defense | Automated | `services/smdp-plus/.../server_authclient_test.go::TestAuthenticateClient_RejectsWrongServerChallenge` |
| EuiccSigned1 unknown CI rejection          | Automated | `services/smdp-plus/.../server_authclient_test.go::TestAuthenticateClient_RejectsEUICCFromUnknownCI` |
| GetBoundProfilePackage (BPP generation)    | Pending   | Returns 501 today; ships with the SAIP codec follow-up |
| HandleNotification                         | Automated | `services/smdp-plus/.../server_test.go::TestHandleNotification_HappyPath` |

## ES8+ — Application protocol payload

| Test family                                | Coverage  | Notes                                              |
| ------------------------------------------ | --------- | -------------------------------------------------- |
| BPP encryption (BSP, AES-128-GCM)          | Automated | `pkg/crypto/bsp/bsp_test.go` round-trip + tamper rejection |
| ECKA over P-256                            | Automated | `pkg/crypto/ecka/ecka_test.go` (both sides agree) |
| X9.63 KDF SHA-256                          | Automated | `pkg/crypto/kdf/x963_test.go` (NIST CAVP vector)  |
| ECDSA over P-256                           | Automated | `pkg/crypto/ecdsa/ecdsa_test.go`                  |
| Brainpool P-256 r1                         | Pending   | Stubbed; tracked for follow-up dependency vetting |
| Full ProtectedProfilePackage framing       | Pending   | Ships with SAIP codec                             |

## ES11 / ES12 — SM-DS

| Test family                       | Coverage  | Aether test                                              |
| --------------------------------- | --------- | -------------------------------------------------------- |
| ES12 RegisterEvent (§5.5.1)       | Automated | `services/smds/.../server_test.go::TestSMDS_FullDiscoveryFlow` |
| ES12 DeleteEvent (§5.5.2)         | Automated | same                                                     |
| ES12 RegisterEvent idempotency    | Automated | `services/smds/internal/events/store_test.go::TestMemoryStore_Idempotent` (and PG equivalent) |
| ES11 AuthenticateClient (§5.5.4)  | Automated | `services/smds/.../server_test.go` — skeleton path; signature verification is the same dependency as smdp-plus |
| ES11 GetEvents (§5.5.3)           | Automated | `services/smds/.../server_test.go::TestSMDS_FullDiscoveryFlow` |
| Delete-of-unknown returns 404     | Automated | `services/smds/.../server_test.go::TestSMDS_DeleteUnknownReturns404` |

## SGP.32 — IoT (eIM)

| Test family                  | Coverage  | Aether test                                       |
| ---------------------------- | --------- | ------------------------------------------------- |
| Device registration          | Automated | `services/eim/.../server_test.go::TestEIM_FleetLifecycle` (register) |
| Command queue lifecycle      | Automated | same (enqueue → poll → ack)                       |
| Idempotent duplicate-register rejection | Automated | `services/eim/.../server_test.go::TestEIM_RegisterRejectsDuplicate` |
| IPA poll → command delivered | Automated | `services/eim/.../server_test.go::TestEIM_FleetLifecycle` |
| IPAd direct profile fetch    | Pending   | Skeleton today; full flow needs the SAIP codec   |
| IPAe (indirect) flow         | Not started | Phase 4 follow-up                                |

## Audit / persistence behaviour

| Test family                                  | Coverage  | Aether test                                         |
| -------------------------------------------- | --------- | --------------------------------------------------- |
| Hash-chain integrity under concurrent writes | Automated | `services/audit/internal/chain/postgres_test.go::TestPGLedger_ConcurrentAppendsKeepChainIntact` |
| Tamper detection                             | Automated | `TestPGLedger_TamperDetected` and `TestLedger_TamperDetected` |
| Append-only (no UPDATE / DELETE in app code) | Automated (compile-time + runtime) | grep on the chain package; no UPDATE/DELETE statements present |

## Cryptographic correctness — interop with stdlib

These tests prove the platform's signatures and keys are
mathematically correct, not just that two halves of Aether agree
with each other. The verifier is Go's stdlib `crypto/ecdsa`,
which is the same primitive any auditor's tooling would use.

| Test                                                  | What it proves                                      |
| ----------------------------------------------------- | --------------------------------------------------- |
| `services/smdp-plus/.../server_signing_test.go::TestInitiateAuthentication_SignatureVerifies` | ServerSigned1 signature verifies against the returned cert's public key |
| `services/smdp-plus/.../server_authclient_test.go::TestAuthenticateClient_VerifiesGoodResponse` | A synthetic eUICC's full chain (CA → EUM → leaf) verifies through the SM-DP+'s gate |
| `services/hsm-broker/internal/backend/softhsm/softhsm_test.go::TestSoftHSM_GenerateAndSignAndVerify` | A signature produced inside SoftHSM verifies against the public key the SoftHSM returned |
| `pkg/crypto/kdf/x963_test.go::TestX963SHA256_NIST_CAVP` | KDF output matches the published NIST CAVP vector |

## Hardware tests

These cannot be automated without a real eUICC card and an
LPA-capable device. The procedure for each lives in the
relevant per-test page under
`tools/conformance/coverage/hardware/` (planned; create when
your operator first runs through them so the next adopter
benefits).

| Test family                                    | Why hardware is required                              |
| ---------------------------------------------- | ----------------------------------------------------- |
| End-to-end profile download to sysmoEUICC      | Requires a physical eUICC and an LPA running on a real device |
| Profile installation, enable, disable, delete  | Same                                                  |
| eUICC-internal file system layout              | Verifiable only by reading the card                   |
| OTA channel behaviour                          | Requires actual mobile network for radio-side test    |
| EID provisioning and binding                   | Requires the real EUM-issued eUICC                    |
| LPAe / LPAd choice and ATR exchange            | LPA-side, runs on the device                          |

The path to making these automated is a CI hardware bench
(sysmoEUICC + a USB SIM-tray device + Robotium-style Android
test driver). That's a Phase 6+ investment; until then this
list is the operator's manual checklist before a conformance
lab visit.

## Out of scope

| Test family                              | Reason                                              |
| ---------------------------------------- | --------------------------------------------------- |
| LPA conformance                          | Aether is the server side; LPA conformance is the device vendor's certification |
| eUICC firmware behaviour                 | The card vendor certifies this                      |
| HSM physical security                    | Vendor certification (FIPS 140-2/3); Aether consumes the result |
| Network operator / HPLMN behaviour       | Out of the SM-DP+ scope                             |
| SGP.21 architecture compliance           | Architectural alignment, not test cases             |

## Updating this matrix

Any PR that adds or changes a protocol-touching test should
update the relevant row here. CI doesn't enforce this yet —
the linter that checks `services/smdp-plus` for spec references
is the analogous control on the SGP.22 side, and a similar
linter for this matrix is a planned follow-up.
