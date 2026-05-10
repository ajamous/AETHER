package main

import "time"

// Case describes one entry in the conformance catalogue: an
// SGP.23 family, a human-readable title, and the existing
// per-module test that covers it. Add a row here when you add
// a test that exercises a documented SGP.23 case.
//
// The Module field is a path relative to the repo root. The
// Package and RunPattern are passed verbatim to `go test` —
// Package is the package selector (e.g. ./internal/server/...)
// and RunPattern is a `-run` regex.
type Case struct {
	Family     string
	Title      string
	Module     string
	Package    string
	RunPattern string
}

// Result is what run() produces for each case.
type Result struct {
	Case     Case
	OK       bool
	Output   string
	Duration time.Duration
}

// catalogue is the SGP.23 → existing-test mapping. Mirrors
// tools/conformance/coverage/sgp23.md. Keep the two in sync.
var catalogue = []Case{
	// -- ES2+ ----------------------------------------------------
	{
		Family: "ES2+",
		Title:  "DownloadOrder happy path",
		Module: "services/gateway", Package: "./internal/server/...",
		RunPattern: "^TestGateway_DownloadOrder_HappyPath$",
	},
	{
		Family: "ES2+",
		Title:  "DownloadOrder rejects empty payload",
		Module: "services/gateway", Package: "./internal/server/...",
		RunPattern: "^TestGateway_DownloadOrder_RejectsEmpty$",
	},
	{
		Family: "ES2+",
		Title:  "Gateway proxy round-trip to profile-builder",
		Module: "services/gateway", Package: "./internal/server/...",
		RunPattern: "^TestGateway_ProxyToProfileBuilder$",
	},
	{
		Family: "ES2+",
		Title:  "mTLS rejects request without client cert (path-scoped)",
		Module: "services/gateway", Package: "./internal/server/...",
		RunPattern: "^TestGateway_MTLS_ES2PlusRejectsNoClientCert$",
	},
	{
		Family: "ES2+",
		Title:  "mTLS accepts trusted client cert",
		Module: "services/gateway", Package: "./internal/server/...",
		RunPattern: "^TestGateway_MTLS_ES2PlusAcceptsTrustedClientCert$",
	},
	{
		Family: "ES2+",
		Title:  "mTLS rejects untrusted client cert",
		Module: "services/gateway", Package: "./internal/server/...",
		RunPattern: "^TestGateway_MTLS_ES2PlusRejectsUntrustedClientCert$",
	},
	{
		Family: "ES2+",
		Title:  "Admin paths bypass mTLS gate",
		Module: "services/gateway", Package: "./internal/server/...",
		RunPattern: "^TestGateway_MTLS_AdminPathsDoNotRequireClientCert$",
	},
	{
		Family: "ES2+",
		Title:  "401 counter advances on rejected ES2+ requests",
		Module: "services/gateway", Package: "./internal/server/...",
		RunPattern: "^TestGateway_MTLS_401CounterAdvances$",
	},
	{
		Family: "ES2+",
		Title:  "Rate limiter rejects after burst; admin paths bypass; counter advances",
		Module: "services/gateway", Package: "./internal/server/...",
		RunPattern: "^TestGateway_RateLimit_RejectsAfterBurst$",
	},
	{
		Family: "Admin",
		Title:  "OIDC rejects /v1/* without Bearer; /v1/health bypasses; counter advances",
		Module: "services/gateway", Package: "./internal/server/...",
		RunPattern: "^TestGateway_OIDC_RejectsAdminWithoutBearer$",
	},
	{
		Family: "Admin",
		Title:  "OIDC accepts a valid Bearer token (RS256) and proxies to upstream",
		Module: "services/gateway", Package: "./internal/server/...",
		RunPattern: "^TestGateway_OIDC_AcceptsValidBearer$",
	},
	{
		Family: "Admin",
		Title:  "OIDC verifier accepts RS256 happy path",
		Module: "services/gateway", Package: "./internal/oidc/...",
		RunPattern: "^TestVerify_RS256_HappyPath$",
	},
	{
		Family: "Admin",
		Title:  "OIDC verifier accepts ES256 happy path",
		Module: "services/gateway", Package: "./internal/oidc/...",
		RunPattern: "^TestVerify_ES256_HappyPath$",
	},
	{
		Family: "Admin",
		Title:  "OIDC verifier rejects HS256 (symmetric) tokens",
		Module: "services/gateway", Package: "./internal/oidc/...",
		RunPattern: "^TestVerify_RejectsHS256$",
	},
	{
		Family: "Admin",
		Title:  "OIDC verifier rejects expired tokens",
		Module: "services/gateway", Package: "./internal/oidc/...",
		RunPattern: "^TestVerify_RejectsExpired$",
	},
	{
		Family: "Admin",
		Title:  "OIDC verifier rejects wrong issuer",
		Module: "services/gateway", Package: "./internal/oidc/...",
		RunPattern: "^TestVerify_RejectsWrongIssuer$",
	},
	{
		Family: "Admin",
		Title:  "OIDC verifier rejects wrong audience",
		Module: "services/gateway", Package: "./internal/oidc/...",
		RunPattern: "^TestVerify_RejectsWrongAudience$",
	},
	{
		Family: "Admin",
		Title:  "OIDC verifier rejects tampered signature",
		Module: "services/gateway", Package: "./internal/oidc/...",
		RunPattern: "^TestVerify_RejectsTamperedSignature$",
	},
	{
		Family: "Admin",
		Title:  "OpenAPI spec embedded + served at /v1/openapi.yaml",
		Module: "services/gateway", Package: "./internal/server/...",
		RunPattern: "^TestGateway_OpenAPISpec$",
	},
	{
		Family: "Admin",
		Title:  "OpenAPI endpoint bypasses OIDC for client discovery",
		Module: "services/gateway", Package: "./internal/server/...",
		RunPattern: "^TestGateway_OpenAPI_BypassesOIDC$",
	},

	// -- ES9+ ----------------------------------------------------
	{
		Family: "ES9+",
		Title:  "InitiateAuthentication happy path",
		Module: "services/smdp-plus", Package: "./internal/server/...",
		RunPattern: "^TestInitiateAuthentication_HappyPath$",
	},
	{
		Family: "ES9+",
		Title:  "InitiateAuthentication rejects empty challenge",
		Module: "services/smdp-plus", Package: "./internal/server/...",
		RunPattern: "^TestInitiateAuthentication_RejectsEmptyChallenge$",
	},
	{
		Family: "ES9+",
		Title:  "ServerSigned1 ASN.1 round-trip",
		Module: "services/smdp-plus", Package: "./internal/signing/...",
		RunPattern: "^TestServerSigned1_RoundTrip$",
	},
	{
		Family: "ES9+",
		Title:  "ServerSigned1 validation rejects bad inputs",
		Module: "services/smdp-plus", Package: "./internal/signing/...",
		RunPattern: "^TestServerSigned1_ValidationCatches$",
	},
	{
		Family: "ES9+",
		Title:  "InitiateAuthentication signature verifies end-to-end",
		Module: "services/smdp-plus", Package: "./internal/server/...",
		RunPattern: "^TestInitiateAuthentication_SignatureVerifies$",
	},
	{
		Family: "ES9+",
		Title:  "AuthenticateClient state progression",
		Module: "services/smdp-plus", Package: "./internal/server/...",
		RunPattern: "^TestAuthenticateClient_StateProgression$",
	},
	{
		Family: "ES9+",
		Title:  "AuthenticateClient verifies a good eUICC response",
		Module: "services/smdp-plus", Package: "./internal/server/...",
		RunPattern: "^TestAuthenticateClient_VerifiesGoodResponse$",
	},
	{
		Family: "ES9+",
		Title:  "AuthenticateClient rejects wrong serverChallenge (replay defense)",
		Module: "services/smdp-plus", Package: "./internal/server/...",
		RunPattern: "^TestAuthenticateClient_RejectsWrongServerChallenge$",
	},
	{
		Family: "ES9+",
		Title:  "AuthenticateClient rejects eUICC from unknown CI",
		Module: "services/smdp-plus", Package: "./internal/server/...",
		RunPattern: "^TestAuthenticateClient_RejectsEUICCFromUnknownCI$",
	},
	{
		Family: "ES9+",
		Title:  "AuthenticateClient rejects tampered signature",
		Module: "services/smdp-plus", Package: "./internal/server/...",
		RunPattern: "^TestAuthenticateClient_RejectsTamperedSignature$",
	},
	{
		Family: "ES9+",
		Title:  "GetBoundProfilePackage honestly returns 501 (BPP pending SAIP codec)",
		Module: "services/smdp-plus", Package: "./internal/server/...",
		RunPattern: "^TestGetBoundProfilePackage_ReturnsNotImplemented$",
	},
	{
		Family: "ES9+",
		Title:  "HandleNotification happy path",
		Module: "services/smdp-plus", Package: "./internal/server/...",
		RunPattern: "^TestHandleNotification_HappyPath$",
	},

	// -- ES8+ / Crypto primitives --------------------------------
	{
		Family: "ES8+",
		Title:  "BSP AES-128-GCM encrypt/decrypt round-trip",
		Module: "pkg/crypto", Package: "./bsp/...",
		RunPattern: "^TestEncryptDecrypt_RoundTrip$",
	},
	{
		Family: "ES8+",
		Title:  "BSP rejects tampered ciphertext",
		Module: "pkg/crypto", Package: "./bsp/...",
		RunPattern: "^TestDecrypt_RejectsTamperedCiphertext$",
	},
	{
		Family: "ES8+",
		Title:  "BSP rejects tampered associated data",
		Module: "pkg/crypto", Package: "./bsp/...",
		RunPattern: "^TestDecrypt_RejectsTamperedAssociatedData$",
	},
	{
		Family: "ES8+",
		Title:  "ECKA over P-256 (both sides agree)",
		Module: "pkg/crypto", Package: "./ecka/...",
		RunPattern: "^TestECKA_AgreeP256$",
	},
	{
		Family: "ES8+",
		Title:  "X9.63-SHA-256 KDF matches NIST CAVP vector",
		Module: "pkg/crypto", Package: "./kdf/...",
		RunPattern: "^TestX963SHA256_NIST_CAVP$",
	},
	{
		Family: "ES8+",
		Title:  "ECDSA P-256 sign + verify round-trip",
		Module: "pkg/crypto", Package: "./ecdsa/...",
		RunPattern: "^TestSignVerifyP256_RoundTrip$",
	},

	// -- SAIP (SGP.22 §B Profile Package codec) ------------------
	{
		Family: "SAIP",
		Title:  "ProfilePackage build + decode round-trip (header + PEEnd)",
		Module: "pkg/saip", Package: ".",
		RunPattern: "^TestBuild_Roundtrip$",
	},
	{
		Family: "SAIP",
		Title:  "ProfileHeader validation rejects every malformed shape",
		Module: "pkg/saip", Package: ".",
		RunPattern: "^TestBuild_Validation$",
	},
	{
		Family: "SAIP",
		Title:  "Decode rejects trailing bytes / non-SEQUENCE / truncated input",
		Module: "pkg/saip", Package: ".",
		RunPattern: "^TestDecode_Rejects",
	},
	{
		Family: "SAIP",
		Title:  "AppendRaw inserts spare ProfileElement before PEEnd",
		Module: "pkg/saip", Package: ".",
		RunPattern: "^TestAppendRaw_InsertsBeforeEnd$",
	},
	{
		Family: "SAIP",
		Title:  "DER encoding is byte-stable across invocations",
		Module: "pkg/saip", Package: ".",
		RunPattern: "^TestMarshalDER_StableAcrossInvocations$",
	},
	{
		Family: "SAIP",
		Title:  "profile-builder UPP emits valid SAIP DER (header decodes, fields match)",
		Module: "services/profile-builder", Package: "./internal/template/...",
		RunPattern: "^TestBuildUPP_EmitsValidSAIP$",
	},
	{
		Family: "SAIP",
		Title:  "ICCID nibble-swap matches SGP.22 §B.1",
		Module: "services/profile-builder", Package: "./internal/template/...",
		RunPattern: "^TestEncodeICCIDNibbleSwapped$",
	},

	// -- ES11 / ES12 — SM-DS -------------------------------------
	{
		Family: "ES11/ES12",
		Title:  "Full discovery flow: register → auth → poll → delete",
		Module: "services/smds", Package: "./internal/server/...",
		RunPattern: "^TestSMDS_FullDiscoveryFlow$",
	},
	{
		Family: "ES11/ES12",
		Title:  "Register rejects missing fields",
		Module: "services/smds", Package: "./internal/server/...",
		RunPattern: "^TestSMDS_RegisterRejectsMissingFields$",
	},
	{
		Family: "ES11/ES12",
		Title:  "Delete of unknown returns 404",
		Module: "services/smds", Package: "./internal/server/...",
		RunPattern: "^TestSMDS_DeleteUnknownReturns404$",
	},
	{
		Family: "ES11/ES12",
		Title:  "Register is idempotent on (eid, event_id)",
		Module: "services/smds", Package: "./internal/events/...",
		RunPattern: "^TestMemoryStore_Idempotent$",
	},

	// -- SGP.32 — eIM --------------------------------------------
	{
		Family: "SGP.32",
		Title:  "Fleet lifecycle: register → enqueue → poll → ack",
		Module: "services/eim", Package: "./internal/server/...",
		RunPattern: "^TestEIM_FleetLifecycle$",
	},
	{
		Family: "SGP.32",
		Title:  "Register rejects duplicate EID",
		Module: "services/eim", Package: "./internal/server/...",
		RunPattern: "^TestEIM_RegisterRejectsDuplicate$",
	},
	{
		Family: "SGP.32",
		Title:  "Enqueue rejects bad command kind",
		Module: "services/eim", Package: "./internal/server/...",
		RunPattern: "^TestEIM_EnqueueRejectsBadKind$",
	},

	// -- Audit / persistence -------------------------------------
	{
		Family: "Audit",
		Title:  "In-memory hash chain append + verify",
		Module: "services/audit", Package: "./internal/chain/...",
		RunPattern: "^TestLedger_AppendAndVerify$",
	},
	{
		Family: "Audit",
		Title:  "In-memory hash chain detects tampering",
		Module: "services/audit", Package: "./internal/chain/...",
		RunPattern: "^TestLedger_TamperDetected$",
	},
	{
		Family: "Audit",
		Title:  "Anchor DER round-trip preserves all fields",
		Module: "services/audit", Package: "./internal/anchor/...",
		RunPattern: "^TestAnchor_RoundTrip$",
	},
	{
		Family: "Audit",
		Title:  "Anchor signing end-to-end verifies against broker public key",
		Module: "services/audit", Package: "./internal/anchor/...",
		RunPattern: "^TestSign_VerifiesEndToEnd$",
	},
	{
		Family: "Audit",
		Title:  "/v1/anchor returns unsigned anchor in lab mode",
		Module: "services/audit", Package: "./internal/server/...",
		RunPattern: "^TestAudit_Anchor_LabUnsigned$",
	},
	{
		Family: "Audit",
		Title:  "/v1/anchor signed end-to-end (DER + ECDSA-SHA-256)",
		Module: "services/audit", Package: "./internal/server/...",
		RunPattern: "^TestAudit_Anchor_SignedEndToEnd$",
	},
	{
		Family: "Audit",
		Title:  "Auditor CLI verifies a freshly-signed anchor against the published pubkey",
		Module: "tools/aether-verify-anchor", Package: ".",
		RunPattern: "^TestRun_HappyPath$",
	},
	{
		Family: "Audit",
		Title:  "Auditor CLI rejects tampered signed_payload, tampered signature, wrong pubkey",
		Module: "tools/aether-verify-anchor", Package: ".",
		RunPattern: "^TestRun_(TamperedSignedPayload|TamperedSignature|WrongPublicKey)$",
	},
	{
		Family: "Audit",
		Title:  "Auditor CLI replay cross-checks: match + length-mismatch + tail-hash-mismatch",
		Module: "tools/aether-verify-anchor", Package: ".",
		RunPattern: "^TestRun_Replay(Match|LengthMismatch|TailHashMismatch)$",
	},

	// -- Cert handling -------------------------------------------
	{
		Family: "Certs",
		Title:  "Lab chain loads + verifies (CI → EUM → leaf)",
		Module: "services/certmgr", Package: "./internal/store/...",
		RunPattern: "^TestStore_LoadsAndVerifies$",
	},
	{
		Family: "Certs",
		Title:  "VerifyChain accepts lab identities",
		Module: "services/certmgr", Package: "./internal/store/...",
		RunPattern: "^TestStore_VerifyChainAcceptsLabIdentities$",
	},
	{
		Family: "Certs",
		Title:  "Trust-store + intermediates PEM endpoints reachable",
		Module: "services/certmgr", Package: "./internal/server/...",
		RunPattern: "^TestServer_TrustStorePEM_AndIntermediatesPEM$",
	},

	// -- HSM (memory backend; SoftHSM integration runs separately)
	{
		Family: "HSM",
		Title:  "Memory backend Sign produces verifiable signature",
		Module: "services/hsm-broker", Package: "./internal/backend/memory/...",
		RunPattern: "^TestMemory_GenerateAndSignECDSA$",
	},
	{
		Family: "HSM",
		Title:  "Memory backend ECKA derive (both sides agree)",
		Module: "services/hsm-broker", Package: "./internal/backend/memory/...",
		RunPattern: "^TestMemory_DeriveKey_ECKA$",
	},
}
