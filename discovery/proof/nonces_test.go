// SPDX-License-Identifier: AGPL-3.0-or-later

package proof_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/discovery/proof"
)

const nonceDiscovery = "https://api.orpheum-app.com/core/v1"

// signedOver is a core's signature over nonce for purpose, at nonceDiscovery as core k7xq2.
func signedOver(t *testing.T, purpose, nonce string) proof.SignedNonce {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return proof.SignedNonce{
		PublicKey: public, Purpose: purpose, BaseURL: nonceDiscovery, Core: "k7xq2", Nonce: nonce,
		Signature: ed25519.Sign(private, proof.Message(purpose, nonceDiscovery, "k7xq2", nonce)),
	}
}

// A nonce works once, for the purpose its signature was made for, until it expires; one another
// issuer's key made, or one whose bytes were changed, never works (S13, D343).
func TestNonces_ANonceWorksOnceForItsPurposeUntilItExpires(t *testing.T) {
	t.Parallel()
	nonces, err := proof.NewNonces(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.October, 26, 12, 0, 0, 0, time.UTC)
	nonce, err := nonces.Issue(rand.Reader, now.Add(5*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	signed := signedOver(t, "session", nonce)
	wrongPurpose := signed
	wrongPurpose.Purpose = "enrol"
	if nonces.Spend(now, wrongPurpose) {
		t.Error("a signature for a session was spent for enrolling")
	}
	if !nonces.Spend(now, signed) {
		t.Fatal("a fresh nonce's valid signature was not taken")
	}
	if nonces.Spend(now, signed) {
		t.Error("a nonce was spent twice")
	}
	later, err := nonces.Issue(rand.Reader, now.Add(5*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if nonces.Spend(now.Add(5*time.Minute), signedOver(t, "session", later)) {
		t.Error("a nonce was spent at its expiry")
	}
	other, err := proof.NewNonces(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if other.Spend(now, signedOver(t, "session", later)) {
		t.Error("a nonce another issuer made was spent")
	}
	raw, _ := base64.RawURLEncoding.DecodeString(later) //nolint:errcheck // issued above
	raw[0] ^= 1
	if nonces.Spend(now, signedOver(t, "session", base64.RawURLEncoding.EncodeToString(raw))) {
		t.Error("a nonce whose expiry was changed was spent")
	}
}

// Issuing keeps nothing, so no count of strangers' challenges keeps a core from its nonce (S13).
func TestNonces_IssuingKeepsNothing(t *testing.T) {
	t.Parallel()
	nonces, err := proof.NewNonces(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.October, 26, 12, 0, 0, 0, time.UTC)
	for range 10_000 {
		if _, err = nonces.Issue(rand.Reader, now.Add(time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	nonce, err := nonces.Issue(rand.Reader, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if !nonces.Spend(now, signedOver(t, "enrol", nonce)) {
		t.Error("after many challenges, a fresh nonce's signature was not taken")
	}
	if _, err = proof.NewNonces(strings.NewReader("short")); err == nil {
		t.Error("nonces were made with a key of five bytes")
	}
}
