// SPDX-License-Identifier: AGPL-3.0-or-later

package proof

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/discovery/safe32code"
)

const (
	discoveryA = "https://api.orpheum-app.com/core/v1"
	discoveryB = "https://discovery.example/core/v1"
)

// A signature made to enrol does not open a session, nor the reverse, and a key that is not a key
// verifies nothing (D273).
func TestVerify_ASignatureServesOnlyThePurposeItWasMadeFor(t *testing.T) {
	t.Parallel()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signature := ed25519.Sign(private, Message("enrol", discoveryA, "k7xq2", "n0nce"))
	if !Verify(public, "enrol", discoveryA, "k7xq2", "n0nce", signature) {
		t.Error("a signature for enrolling does not verify for enrolling")
	}
	if Verify(public, "session", discoveryA, "k7xq2", "n0nce", signature) {
		t.Error("a signature for enrolling verifies for a session")
	}
	if Verify(public, "enrol", discoveryA, "k7xq2", "another", signature) {
		t.Error("a signature over one nonce verifies over another")
	}
	if Verify(public[:8], "enrol", discoveryA, "k7xq2", "n0nce", signature) {
		t.Error("a key of the wrong size verifies a signature")
	}
}

// A signature is bound to the discovery the core called and to the core: one discovery cannot pass
// another's challenge to the core and use the signature there, nor one core's serve another (C8,
// X12). A trailing slash on the base URL changes nothing.
func TestVerify_ASignatureServesOnlyTheDiscoveryAndCoreItWasMadeFor(t *testing.T) {
	t.Parallel()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signature := ed25519.Sign(private, Message("session", discoveryA+"/", "k7xq2", "n0nce"))
	if !Verify(public, "session", discoveryA, "k7xq2", "n0nce", signature) {
		t.Error("a signature does not verify at the discovery it was made for")
	}
	if Verify(public, "session", discoveryB, "k7xq2", "n0nce", signature) {
		t.Error("a signature made for one discovery verifies at another")
	}
	if Verify(public, "session", discoveryA, "other", "n0nce", signature) {
		t.Error("a signature made for one core verifies for another")
	}
}

// An answer's signature covers its exact SDP, the offer it answers and the fingerprint that offer
// carried; a change to any fails it (the protocol's §2.5 item 4; A7, A9).
func TestVerifyAnswer_CoversTheExactSDPTheOfferAndItsFingerprint(t *testing.T) {
	t.Parallel()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	const fingerprint = "sha-256 AB:CD"
	answer := Answer{CoreID: "k7xq2", OfferID: "7", OfferFingerprint: fingerprint, SDP: "v=0\r\na=x\r\n"}
	signature := ed25519.Sign(private, answer.Message())
	if !VerifyAnswer(public, answer, signature) {
		t.Fatal("the answer's own signature does not verify")
	}
	for name, changed := range map[string]Answer{
		"another SDP":         {CoreID: "k7xq2", OfferID: "7", OfferFingerprint: fingerprint, SDP: "v=0\r\na=y\r\n"},
		"another offer":       {CoreID: "k7xq2", OfferID: "8", OfferFingerprint: fingerprint, SDP: answer.SDP},
		"another fingerprint": {CoreID: "k7xq2", OfferID: "7", OfferFingerprint: "sha-256 EF:01", SDP: answer.SDP},
		"another core":        {CoreID: "other", OfferID: "7", OfferFingerprint: fingerprint, SDP: answer.SDP},
	} {
		if VerifyAnswer(public, changed, signature) {
			t.Errorf("the signature verifies over %s", name)
		}
	}
}

// The fingerprint links carry is the SHA-256 of the public key, base64url without padding (§3.1).
func TestFingerprint_IsTheKeysSHA256InBase64URL(t *testing.T) {
	t.Parallel()
	zero := make(ed25519.PublicKey, ed25519.PublicKeySize)
	// SHA-256 of 32 zero bytes is 66687aad...f925; its base64url, unpadded:
	if got := Fingerprint(zero); got != "Zmh6rfhivXdsj8GLjp-OIAiXFIVu4jOzkCpZHQ1fKSU" {
		t.Errorf("Fingerprint(32 zero bytes) = %q", got)
	}
}

func TestOffer_ProvesOnlyItsFingerprintUnderItsKey(t *testing.T) {
	t.Parallel()
	key := SessionOfferKey("a session's token")
	proven := Offer(key, "sha-256 AB:CD")
	if !VerifyOffer(key, "sha-256 AB:CD", proven) {
		t.Error("an offer's own proof does not verify")
	}
	if VerifyOffer(key, "sha-256 AB:CE", proven) {
		t.Error("the proof verified for another fingerprint: a discovery could swap in its own certificate")
	}
	if VerifyOffer(SessionOfferKey("another session's token"), "sha-256 AB:CD", proven) {
		t.Error("the proof verified under another key")
	}
	if digest := sha256.Sum256([]byte("a session's token")); string(key) != string(digest[:]) {
		t.Error("a session's offer key is not SHA-256 of its token, the hash the core keeps")
	}
}

func TestFingerprintText_IsTheSameDigestInSafe32Groups(t *testing.T) {
	t.Parallel()
	public, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	text := FingerprintText(public)
	if len(text) != 52+12 || strings.Count(text, "-") != 12 {
		t.Fatalf("FingerprintText = %q; want 13 groups of four", text)
	}
	read, err := safe32code.Read(strings.ToUpper(text), 52)
	if err != nil || read != text {
		t.Errorf("the text read back as typed, in capitals, is %q, %v; want it unchanged", read, err)
	}
	digest := sha256.Sum256(public)
	again, err := safe32code.WriteBytes(append(digest[:], 0), 52)
	if err != nil || again != text {
		t.Errorf("FingerprintText = %q; want the key's SHA-256 in safe32, %q", text, again)
	}
}
