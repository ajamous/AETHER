// Package smdsv1 defines the wire types for the Aether SM-DS API.
//
// SGP.22 §5.5 specifies the ASN.1-shaped messages that ES11 and ES12
// carry. The Go types here mirror the field shapes for the HTTP+JSON
// surface the LPA and the SM-DP+ reach. The full ASN.1-faithful
// envelopes live alongside the SGP.22 codec under pkg/asn1/sgp22 once
// the spec modules are vendored.
package smdsv1

// EID is a 32-hex-character eUICC ID per SGP.02 §6.5.1.
type EID string

// --- ES12: SM-DP+ → SM-DS ----------------------------------------------------

// RegisterEventRequest — SGP.22 §5.5.1.
//
// The SM-DP+ tells the SM-DS "there is a profile waiting for EID X
// at SMDPAddress". Once the LPA discovers it via ES11 and successfully
// downloads, the SM-DP+ is expected to call DeleteEventRequest.
type RegisterEventRequest struct {
	EID                 EID    `json:"eid"`
	RSPServerAddress    string `json:"rsp_server_address"`
	EventID             string `json:"event_id"` // SM-DP+-allocated; idempotency key
	ForwardingIndicator bool   `json:"forwarding_indicator"`
}

// RegisterEventResponse — SGP.22 §5.5.1.
type RegisterEventResponse struct {
	EventID string `json:"event_id"`
}

// DeleteEventRequest — SGP.22 §5.5.2.
type DeleteEventRequest struct {
	EID     EID    `json:"eid"`
	EventID string `json:"event_id"`
}

// DeleteEventResponse — SGP.22 §5.5.2.
type DeleteEventResponse struct{}

// --- ES11: LPA → SM-DS -------------------------------------------------------

// AuthenticateClientRequest — SGP.22 §5.5.4.
type AuthenticateClientRequest struct {
	EUICCChallenge []byte `json:"euicc_challenge"`
	EUICCInfo1     []byte `json:"euicc_info1"`
	EID            EID    `json:"eid"`
}

// AuthenticateClientResponse — SGP.22 §5.5.4.
type AuthenticateClientResponse struct {
	TransactionID     string `json:"transaction_id"`
	ServerSigned1     []byte `json:"server_signed1"`
	ServerSignature1  []byte `json:"server_signature1"`
	ServerCertificate []byte `json:"server_certificate"`
}

// Event is one pending profile entry the LPA learns about.
type Event struct {
	EventID          string `json:"event_id"`
	RSPServerAddress string `json:"rsp_server_address"`
}

// GetEventsRequest — SGP.22 §5.5.3.
type GetEventsRequest struct {
	TransactionID string `json:"transaction_id"`
}

// GetEventsResponse — SGP.22 §5.5.3.
type GetEventsResponse struct {
	TransactionID string  `json:"transaction_id"`
	Events        []Event `json:"events"`
}
