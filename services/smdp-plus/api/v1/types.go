// Package smdpv1 holds the JSON wire types Aether's SM-DP+ exposes
// over ES9+ to the LPA and the internal types of its session state.
//
// SGP.22 Annex B defines the canonical ASN.1 types. These Go types
// mirror the field shapes for the HTTP+JSON surface the LPA reaches.
// The full ASN.1-faithful encoding lives under pkg/asn1/sgp22 once
// the spec modules are vendored; the JSON shapes here are the
// transport for endpoints that currently exchange them outside the
// signed envelopes.
package smdpv1

// SessionID is the opaque token that binds an ES9+ session across calls.
type SessionID string

// InitiateAuthenticationRequest is sent by the LPA to begin a download.
// SGP.22 §5.6.1.
type InitiateAuthenticationRequest struct {
	EUICCChallenge []byte `json:"euicc_challenge"`
	EUICCInfo1     []byte `json:"euicc_info1"` // raw ASN.1 bytes from the eUICC
	SMDPAddress    string `json:"smdp_address"`
}

// InitiateAuthenticationResponse — SGP.22 §5.6.1.
type InitiateAuthenticationResponse struct {
	TransactionID       string `json:"transaction_id"`
	ServerSigned1       []byte `json:"server_signed1"`
	ServerSignature1    []byte `json:"server_signature1"`
	EuiccCiPKIDToBeUsed []byte `json:"euicc_ci_pkid_to_be_used"`
	ServerCertificate   []byte `json:"server_certificate"`
}

// AuthenticateClientRequest — SGP.22 §5.6.3.
//
// In a fully spec-faithful deployment, the LPA forwards a single
// `AuthenticateServerResponse` SEQUENCE (SGP.22 §5.7.5) carrying the
// eUICC's four signed pieces. Until the SGP.22 Annex B ASN.1 modules
// are vendored and we can parse that SEQUENCE without ambiguity, the
// HTTP+JSON shape carries the four pieces individually. The signing
// pipeline (DER + ECDSA) over each piece is fully spec-correct;
// only the outer envelope shape is the lab convenience.
type AuthenticateClientRequest struct {
	TransactionID string `json:"transaction_id"`

	// EuiccSigned1DER is the DER-encoded EuiccSigned1 SEQUENCE (§5.7.13).
	EuiccSigned1DER []byte `json:"euicc_signed1"`
	// EuiccSignature1 is DER SEQUENCE { r, s } per §H.5.
	EuiccSignature1 []byte `json:"euicc_signature1"`
	// EuiccCertDER is the eUICC's leaf certificate (DER X.509).
	EuiccCertDER []byte `json:"euicc_certificate"`
	// EumCertDER is the EUM intermediate that issued the leaf.
	EumCertDER []byte `json:"eum_certificate"`

	// AuthenticateServerResponse is the legacy single-blob field.
	// Reserved for the spec-faithful outer SEQUENCE; ignored when
	// the four explicit fields above are populated.
	AuthenticateServerResponse []byte `json:"authenticate_server_response,omitempty"`
}

// AuthenticateClientResponse — SGP.22 §5.6.3.
type AuthenticateClientResponse struct {
	TransactionID   string `json:"transaction_id"`
	ProfileMetadata []byte `json:"profile_metadata"`
	SMDPSigned2     []byte `json:"smdp_signed2"`
	SMDPSignature2  []byte `json:"smdp_signature2"`
	SMDPCertificate []byte `json:"smdp_certificate"`
}

// GetBoundProfilePackageRequest — SGP.22 §5.6.4.
type GetBoundProfilePackageRequest struct {
	TransactionID           string `json:"transaction_id"`
	PrepareDownloadResponse []byte `json:"prepare_download_response"`

	// EUICCOtpk is the eUICC's ephemeral public key for ECKA,
	// uncompressed X9.63 point form (0x04 || X || Y, 65 bytes for
	// P-256). In SGP.22's full flow this is parsed out of the
	// signed PrepareDownloadResponse blob and the parsed bytes
	// are signature-verified against the eUICC cert from
	// AuthenticateClient; until that parser+verifier lands,
	// callers (and the in-tree test harness) supply EUICCOtpk
	// directly. The handler validates length + first-byte before
	// using it for ECKA.
	EUICCOtpk []byte `json:"euicc_otpk,omitempty"`

	// ICCID resolves which prepared profile to seal into the BPP.
	// In SGP.22's full flow the matchingId from the activation code
	// resolves the prepared profile server-side; until that lands,
	// the in-tree/test path supplies the ICCID directly. When empty
	// (or unknown to the prepared-profile store) the handler falls
	// back to a header-only placeholder UPP.
	ICCID string `json:"iccid,omitempty"`
}

// PrepareProfileRequest is the in-tree stand-in for ES2+
// DownloadOrder + ConfirmOrder: it asks the SM-DP+ to build a profile
// from a profile-builder template + subscriber data and hold it,
// keyed by ICCID, for the eUICC that later downloads it. Served at
// POST /v1/profiles/prepare (admin surface, not ES9+).
type PrepareProfileRequest struct {
	// Template is the profile-builder template name. Empty uses the
	// SM-DP+'s configured default template.
	Template   string            `json:"template,omitempty"`
	Subscriber PrepareSubscriber `json:"subscriber"`
}

// PrepareSubscriber is the per-activation data merged with the
// template. Mirrors profile-builder's SubscriberData.
type PrepareSubscriber struct {
	IMSI   string `json:"imsi"`
	ICCID  string `json:"iccid"`
	MSISDN string `json:"msisdn"`
	Ki     []byte `json:"ki"`
	OPc    []byte `json:"opc"`
}

// PrepareProfileResponse echoes the ICCID the prepared profile is
// keyed by; the eUICC's later getBoundProfilePackage resolves it.
type PrepareProfileResponse struct {
	ICCID string `json:"iccid"`
	Note  string `json:"_note,omitempty"`
}

// GetBoundProfilePackageResponse — SGP.22 §5.6.4.
type GetBoundProfilePackageResponse struct {
	TransactionID       string `json:"transaction_id"`
	BoundProfilePackage []byte `json:"bound_profile_package"`
}

// HandleNotificationRequest — SGP.22 §5.6.5.
type HandleNotificationRequest struct {
	PendingNotification []byte `json:"pending_notification"`
}

// HandleNotificationResponse — SGP.22 §5.6.5.
type HandleNotificationResponse struct{}
