// SPDX-License-Identifier: AGPL-3.0-or-later

package discoveryproto

import "errors"

// A device code through the paid service (D293 §2; the protocol's §3.1): a device that knows no
// core asks the service for a code with a P-256 public key of its own and a commitment to a nonce of
// its own (proof.DeviceCommitment); a person signed in enters the code on their core, which claims it
// from the service with a nonce of its own, learning the key and the commitment; the device, told the
// core's nonce, reveals its own; both show the check code proof.DeviceCheckCode makes of the key and
// the two nonces; and once the person says they match, the core seals what the device needs to the
// key (proof.SealDeviceOutcome), which the service passes on unread with a bootstrap token for the
// core under proof.DeviceCodePerson of the code (D321).
//
// A person who scans the device's QR code instead has its key's hash (proof.DeviceKeyHash) from the
// device itself, past the service: the core claims the code with no nonce, activates only the key
// that hash names, and no check code is shown (D321).

// The client API's device-code paths, under <base>/client/v1.
const (
	// PathDeviceCodes is posted a DeviceCodeRequest for a new code (DeviceCodeResponse).
	PathDeviceCodes = "/device-codes"
	// PathDeviceCodeState is posted a DeviceCodeStateRequest for how the code stands
	// (DeviceCodeStateResponse).
	PathDeviceCodeState = "/device-codes/state"
	// PathDeviceCodeReveal is posted a DeviceCodeRevealRequest by the device once its code stands
	// entered with the core's nonce: its own nonce, which it committed to when it asked.
	PathDeviceCodeReveal = "/device-codes/reveal"
)

// The core API's device-code paths, under <base>/core/v1.
const (
	// PathDeviceCodeClaim is posted a DeviceCodeClaimRequest by the core a person entered the code on
	// (DeviceCodeClaimResponse).
	PathDeviceCodeClaim = "/device-codes/claim"
	// PathDeviceCodeOutcome is posted a DeviceCodeOutcomeRequest by the core that claimed the code.
	PathDeviceCodeOutcome = "/device-codes/outcome"
)

// The refusals of the device-code paths, each with 429, beside RefusalNotFound's 404: too many codes
// asked for from one place, too many claims by one core (failed, or claimed and unfinished), and a
// device asking how its code stands faster than it may, which asks again later.
const (
	RefusalTooManyCodes  = "too_many_codes"
	RefusalTooManyClaims = "too_many_claims"
	RefusalSlowDown      = "slow_down"
)

// ErrTooManyClaims is the discovery refusing a core's claim with RefusalTooManyClaims: the person
// entering the code is told to wait, as after too many wrong codes.
var ErrTooManyClaims = errors.New("too many device codes claimed that did not work; wait and try again")

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
	// Commitment is proof.DeviceCommitment of the key and the device's nonce, base64url (D321).
	Commitment string `json:"commitment"`
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
// finds the core, as a realm sign-in's does. Once entered, the service names the core that claimed
// the code and that core's realm, as the service knows them, not as the core says: what the device
// shows its person to confirm before going on, and checks the sealed outcome against (the device
// code audit's M1, 2026-10-20). A core with no realm is named by its id alone. A code entered by its
// typed code carries the core's nonce, after which the device reveals its own; one entered by its QR
// code carries none, and the device shows no check code (D321).
type DeviceCodeStateResponse struct {
	State          string `json:"state"`
	Sealed         string `json:"sealed,omitempty"`
	BootstrapToken string `json:"bootstrapToken,omitempty"`
	CoreID         string `json:"coreId,omitempty"`
	Realm          string `json:"realm,omitempty"`
	CoreNonce      string `json:"coreNonce,omitempty"`
}

// DeviceCodeRevealRequest is a device revealing the nonce it committed to, by its code's secret, once
// the code stands entered with the core's nonce (D321). A nonce that does not open the commitment is
// refused, as is one revealed before the core's nonce is given.
type DeviceCodeRevealRequest struct {
	Secret string `json:"secret"`
	Nonce  string `json:"nonce"`
}

// DeviceCodeClaimRequest is a core claiming the code a person entered on it, as safe32code.Read
// reads it, with its nonce for the check code, base64url; none when the person scanned the device's QR
// code, which needs no check code (D321). The core claims again, with the same nonce, to learn the
// device's once revealed; the first claim's nonce is the one kept.
type DeviceCodeClaimRequest struct {
	Code      string `json:"code"`
	CoreNonce string `json:"coreNonce,omitempty"`
}

// DeviceCodeClaimResponse is the device's public key and platform, as it asked with them, and how
// many seconds ago it asked, which the person entering the code is shown: a code asked for long
// before is more likely one someone sent them (the device code audit's M3). A code no device waits
// with, or one another core claimed, is not_found.
//
// It carries the device's commitment, and once the device revealed it, its nonce (D321).
type DeviceCodeClaimResponse struct {
	PublicKey       string `json:"publicKey"`
	Platform        string `json:"platform,omitempty"`
	AskedSecondsAgo int    `json:"askedSecondsAgo"`
	Commitment      string `json:"commitment"`
	DeviceNonce     string `json:"deviceNonce,omitempty"`
}

// DeviceCodeOutcomeRequest is the core that claimed a code saying how it ended: the outcome sealed to
// the device's key, base64url, or none when the person said the check codes do not match, which ends
// the code.
type DeviceCodeOutcomeRequest struct {
	Code   string `json:"code"`
	Sealed string `json:"sealed,omitempty"`
}
