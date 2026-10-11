// SPDX-License-Identifier: AGPL-3.0-or-later

package proof

import "crypto/ed25519"

// prefix separates a core's signatures to its discovery from any other use of its key: it names the
// protocol, as the purpose after it names the call.
const prefix = "orpheum-discovery-v1:"

// Message is what a signature for purpose over nonce signs: "enrol" or "session" (discoveryproto's
// Purpose constants).
func Message(purpose, nonce string) []byte {
	return []byte(prefix + purpose + ":" + nonce)
}

// Verify is whether signature is publicKey's over nonce for purpose. A key of the wrong size is no
// key: it verifies nothing.
func Verify(publicKey ed25519.PublicKey, purpose, nonce string, signature []byte) bool {
	if len(publicKey) != ed25519.PublicKeySize {
		return false
	}
	return ed25519.Verify(publicKey, Message(purpose, nonce), signature)
}
