// SPDX-License-Identifier: AGPL-3.0-or-later

package discoveryproto

// A device code through the paid service (D293 §2; the protocol's §3.1): a device that knows no
// core asks the service for a code with a P-256 public key of its own; a person signed in enters the
// code on their core, which claims it from the service, learning the key; both show the check code
// proof.DeviceCheckCode makes of that key; and once the person says they match, the core seals what
// the device needs to the key (proof.SealDeviceOutcome), which the service passes on unread with a
// bootstrap token for the core under proof.DeviceCodePerson of the code.

// The client API's device-code paths, under <base>/client/v1.
const (
	// PathDeviceCodes is posted a DeviceCodeRequest for a new code (DeviceCodeResponse).
	PathDeviceCodes = "/device-codes"
	// PathDeviceCodeState is posted a DeviceCodeStateRequest for how the code stands
	// (DeviceCodeStateResponse).
	PathDeviceCodeState = "/device-codes/state"
)

// The core API's device-code paths, under <base>/core/v1.
const (
	// PathDeviceCodeClaim is posted a DeviceCodeClaimRequest by the core a person entered the code on
	// (DeviceCodeClaimResponse).
	PathDeviceCodeClaim = "/device-codes/claim"
	// PathDeviceCodeOutcome is posted a DeviceCodeOutcomeRequest by the core that claimed the code.
	PathDeviceCodeOutcome = "/device-codes/outcome"
)

// The states of a device code, as the device asking sees them.
const (
	// DeviceCodeWaiting is a code no core has claimed yet.
	DeviceCodeWaiting = "waiting"
	// DeviceCodeEntered is a code a core claimed: the device shows its check code.
	DeviceCodeEntered = "entered"
	// DeviceCodeActivated is a code whose person said the check codes match: the state carries the
	// sealed outcome and a bootstrap token.
	DeviceCodeActivated = "activated"
	// DeviceCodeEnded is a code expired, refused or never known.
	DeviceCodeEnded = "ended"
)

// DeviceCodeRequest is a device asking for a code: its public key, the uncompressed P-256 point in
// base64url (65 bytes), which the outcome is sealed to, and its platform as it names itself, which
// the core lists the device by until the person names it.
type DeviceCodeRequest struct {
	PublicKey string `json:"publicKey"`
	Platform  string `json:"platform,omitempty"`
}

// DeviceCodeResponse is a device's code, "xxxx-xxxx" in safe32, the secret by which it alone asks how
// the code stands, and how many seconds the code works for.
type DeviceCodeResponse struct {
	Code      string `json:"code"`
	Secret    string `json:"secret"`
	ExpiresIn int    `json:"expiresIn"`
}

// DeviceCodeStateRequest is a device asking how its code stands, by the secret its code came with.
type DeviceCodeStateRequest struct {
	Secret string `json:"secret"`
}

// DeviceCodeStateResponse is how a device's code stands: one of the DeviceCode states and, once
// activated, the outcome the core sealed to the device's key, base64url, and a bootstrap token that
// finds the core, as a realm sign-in's does.
type DeviceCodeStateResponse struct {
	State          string `json:"state"`
	Sealed         string `json:"sealed,omitempty"`
	BootstrapToken string `json:"bootstrapToken,omitempty"`
}

// DeviceCodeClaimRequest is a core claiming the code a person entered on it, as safe32code.Read
// reads it.
type DeviceCodeClaimRequest struct {
	Code string `json:"code"`
}

// DeviceCodeClaimResponse is the device's public key and platform, as it asked with them. A code no
// device waits with, or one another core claimed, is not_found.
type DeviceCodeClaimResponse struct {
	PublicKey string `json:"publicKey"`
	Platform  string `json:"platform,omitempty"`
}

// DeviceCodeOutcomeRequest is the core that claimed a code saying how it ended: the outcome sealed to
// the device's key, base64url, or none when the person said the check codes do not match, which ends
// the code.
type DeviceCodeOutcomeRequest struct {
	Code   string `json:"code"`
	Sealed string `json:"sealed,omitempty"`
}
