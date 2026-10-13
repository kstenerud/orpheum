// SPDX-License-Identifier: AGPL-3.0-or-later

package proof

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"strings"
)

// prefix separates a core's signatures from any other use of its key: it names the protocol, as the
// purpose after it names the call.
const prefix = "orpheum-discovery-v1:"

// answerPurpose is the purpose of a signature over a WebRTC answer.
const answerPurpose = "answer"

// Message is what a signature to a discovery signs: the purpose ("enrol" or "session",
// discoveryproto's Purpose constants), the discovery's base URL as the core called it, without a
// trailing slash, the core (its id, or before it has one KeyLabel of its public key), and the
// discovery's nonce. Bound so, a discovery cannot pass another's challenge to the core and come away
// with a session there, nor one core's signature serve another (the protocol's §2.1; C8, X12).
func Message(purpose, baseURL, core, nonce string) []byte {
	return []byte(prefix + purpose + ":" + strings.TrimSuffix(baseURL, "/") + ":" + core + ":" + nonce)
}

// Verify is whether signature is publicKey's over Message's fields. A key of the wrong size is no
// key: it verifies nothing.
func Verify(publicKey ed25519.PublicKey, purpose, baseURL, core, nonce string, signature []byte) bool {
	if len(publicKey) != ed25519.PublicKeySize {
		return false
	}
	return ed25519.Verify(publicKey, Message(purpose, baseURL, core, nonce), signature)
}

// Answer is a WebRTC answer as the core signs it: the core's id, the offer's id, the fingerprint the
// client's offer carried (its a=fingerprint value, "sha-256 AB:..."), and the answer's exact SDP. A
// client checks the signature with the key it remembers or its link's fingerprint names, and passes
// that exact SDP on, so a discovery that relays the answer cannot change it, nor answer another
// client's offer with it (the protocol's §2.5 item 4; A7, A9).
type Answer struct {
	CoreID, OfferID, OfferFingerprint, SDP string
}

// Message is what a signature over the answer signs, a line for each field and the SDP last, so no
// field's own colons run into the next.
func (answer Answer) Message() []byte {
	return []byte(prefix + answerPurpose + "\n" + answer.CoreID + "\n" + answer.OfferID + "\n" +
		answer.OfferFingerprint + "\n" + answer.SDP)
}

// VerifyAnswer is whether signature is publicKey's over answer.
func VerifyAnswer(publicKey ed25519.PublicKey, answer Answer, signature []byte) bool {
	if len(publicKey) != ed25519.PublicKeySize {
		return false
	}
	return ed25519.Verify(publicKey, answer.Message(), signature)
}

// KeyLabel is a public key as Message names a core that has no id yet: base64url, unpadded.
func KeyLabel(publicKey ed25519.PublicKey) string {
	return base64.RawURLEncoding.EncodeToString(publicKey)
}

// Fingerprint is the identity key's fingerprint, as every link and QR a core makes carries it: the
// SHA-256 of its public key, base64url, unpadded, 43 characters (the protocol's §3.1; D292 §4, A7).
func Fingerprint(publicKey ed25519.PublicKey) string {
	digest := sha256.Sum256(publicKey)
	return base64.RawURLEncoding.EncodeToString(digest[:])
}
