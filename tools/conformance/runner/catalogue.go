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
