// Package eimv1 defines the wire types for the Aether eIM API.
package eimv1

import "time"

// EID is the 32-hex-character eUICC identifier per SGP.02 §6.5.1.
type EID string

// Device is a registered IoT device.
type Device struct {
	EID          EID            `json:"eid"`
	Label        string         `json:"label,omitempty"`
	Tags         []string       `json:"tags,omitempty"`
	RegisteredAt time.Time      `json:"registered_at"`
	LastSeen     *time.Time     `json:"last_seen,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
}

// CommandKind enumerates what an operator can ask a device to do.
type CommandKind string

const (
	CommandDownloadProfile CommandKind = "download_profile"
	CommandEnableProfile   CommandKind = "enable_profile"
	CommandDisableProfile  CommandKind = "disable_profile"
	CommandDeleteProfile   CommandKind = "delete_profile"
)

// CommandState tracks the lifecycle of a queued command.
type CommandState string

const (
	CommandStatePending   CommandState = "pending"
	CommandStateDelivered CommandState = "delivered"
	CommandStateCompleted CommandState = "completed"
	CommandStateFailed    CommandState = "failed"
)

// Command is a queued operation for a device.
type Command struct {
	ID           string       `json:"id"`
	EID          EID          `json:"eid"`
	Kind         CommandKind  `json:"kind"`
	SMDPAddress  string       `json:"smdp_address,omitempty"`
	MatchingID   string       `json:"matching_id,omitempty"`
	ICCID        string       `json:"iccid,omitempty"`
	State        CommandState `json:"state"`
	CreatedAt    time.Time    `json:"created_at"`
	DeliveredAt  *time.Time   `json:"delivered_at,omitempty"`
	CompletedAt  *time.Time   `json:"completed_at,omitempty"`
	FailureCode  string       `json:"failure_code,omitempty"`
	FailureNote  string       `json:"failure_note,omitempty"`
}

// --- request/response shapes ---------------------------------------------

type RegisterDeviceRequest struct {
	EID      EID            `json:"eid"`
	Label    string         `json:"label,omitempty"`
	Tags     []string       `json:"tags,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type ListDevicesResponse struct {
	Length  int      `json:"length"`
	Devices []Device `json:"devices"`
}

type EnqueueCommandRequest struct {
	Kind        CommandKind `json:"kind"`
	SMDPAddress string      `json:"smdp_address,omitempty"`
	MatchingID  string      `json:"matching_id,omitempty"`
	ICCID       string      `json:"iccid,omitempty"`
}

type ListCommandsResponse struct {
	Length   int       `json:"length"`
	Commands []Command `json:"commands"`
}

type AckCommandRequest struct {
	State       CommandState `json:"state"` // completed | failed
	FailureCode string       `json:"failure_code,omitempty"`
	FailureNote string       `json:"failure_note,omitempty"`
}
