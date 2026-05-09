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
	TransactionID    string `json:"transaction_id"`
	ServerSigned1    []byte `json:"server_signed1"`
	ServerSignature1 []byte `json:"server_signature1"`
	EuiccCiPKIDToBeUsed []byte `json:"euicc_ci_pkid_to_be_used"`
	ServerCertificate []byte `json:"server_certificate"`
}

// AuthenticateClientRequest — SGP.22 §5.6.3.
type AuthenticateClientRequest struct {
	TransactionID      string `json:"transaction_id"`
	AuthenticateServerResponse []byte `json:"authenticate_server_response"`
}

// AuthenticateClientResponse — SGP.22 §5.6.3.
type AuthenticateClientResponse struct {
	TransactionID  string `json:"transaction_id"`
	ProfileMetadata []byte `json:"profile_metadata"`
	SMDPSigned2    []byte `json:"smdp_signed2"`
	SMDPSignature2 []byte `json:"smdp_signature2"`
	SMDPCertificate []byte `json:"smdp_certificate"`
}

// GetBoundProfilePackageRequest — SGP.22 §5.6.4.
type GetBoundProfilePackageRequest struct {
	TransactionID                  string `json:"transaction_id"`
	PrepareDownloadResponse        []byte `json:"prepare_download_response"`
}

// GetBoundProfilePackageResponse — SGP.22 §5.6.4.
type GetBoundProfilePackageResponse struct {
	TransactionID         string `json:"transaction_id"`
	BoundProfilePackage   []byte `json:"bound_profile_package"`
}

// HandleNotificationRequest — SGP.22 §5.6.5.
type HandleNotificationRequest struct {
	PendingNotification []byte `json:"pending_notification"`
}

// HandleNotificationResponse — SGP.22 §5.6.5.
type HandleNotificationResponse struct{}
