// SPDX-License-Identifier: AGPL-3.0-or-later

package proof

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"
)

// A signature made to enrol does not open a session, nor the reverse, and a key that is not a key
// verifies nothing (D273).
func TestVerify_ASignatureServesOnlyThePurposeItWasMadeFor(t *testing.T) {
	t.Parallel()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signature := ed25519.Sign(private, Message("enrol", "n0nce"))
	if !Verify(public, "enrol", "n0nce", signature) {
		t.Error("a signature for enrolling does not verify for enrolling")
	}
	if Verify(public, "session", "n0nce", signature) {
		t.Error("a signature for enrolling verifies for a session")
	}
	if Verify(public, "enrol", "another", signature) {
		t.Error("a signature over one nonce verifies over another")
	}
	if Verify(public[:8], "enrol", "n0nce", signature) {
		t.Error("a key of the wrong size verifies a signature")
	}
}
