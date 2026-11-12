// SPDX-License-Identifier: AGPL-3.0-or-later

package proof

import (
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"

	"github.com/kstenerud/orpheum/discovery/safe32code"
)

// prefix separates a core's signatures from any other use of its key: it names the protocol, as the
// purpose after it names the call.
const prefix = "orpheum-discovery-v1:"

// The purposes of a signature over a WebRTC answer, of a proof over an offer, and of a signature
// over the manifest of a core's client.
const (
	answerPurpose          = "answer"
	offerPurpose           = "offer"
	clientManifestPurpose  = "client-manifest"
	handOffOfferPurpose    = "handoff-offer"
	guestGrantOfferPurpose = "guest-grant-offer"
)

// Message is what a signature to a discovery signs: the purpose ("enrol" or "session",
// discoveryproto's Purpose constants), the discovery's identity, its root chain's id ("od1-…",
// discovery/trust), the core (its id, or before it has one KeyLabel of its public key), and the
// discovery's nonce. Bound so, a discovery cannot pass another's challenge to the core and come away
// with a session there, nor one core's signature serve another (the protocol's §2.1; C8, X12); bound
// to the identity, which lasts the discovery's life, not to a name or URL, its addresses move by a new
// document without the core enrolling again (the root key audit's F14; D393 §4).
func Message(purpose, identity, core, nonce string) []byte {
	return []byte(prefix + purpose + ":" + identity + ":" + core + ":" + nonce)
}

// Verify is whether signature is publicKey's over Message's fields. A key of the wrong size is no
// key: it verifies nothing.
func Verify(publicKey ed25519.PublicKey, purpose, identity, core, nonce string, signature []byte) bool {
	if len(publicKey) != ed25519.PublicKeySize {
		return false
	}
	return ed25519.Verify(publicKey, Message(purpose, identity, core, nonce), signature)
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

// fingerprintTextChars is the fingerprint written for a person: 52 safe32 characters, 13 groups of
// four, which hold all of SHA-256's 256 bits.
const fingerprintTextChars = 52

// FingerprintText is the identity key's fingerprint as a person reads, compares or types it: the same
// SHA-256 as Fingerprint, in safe32 groups, "k7xq-2m4p-…", as the core's other codes are written:
// no letters that look alike, case ignored. Links and QR codes, which machines read, carry
// Fingerprint (the protocol's §2.1; the owner, 2026-10-14).
func FingerprintText(publicKey ed25519.PublicKey) string {
	digest := sha256.Sum256(publicKey)
	// A zero byte after the digest, since 52 characters take 260 bits and the digest has 256.
	text, err := safe32code.WriteBytes(append(digest[:], 0), fingerprintTextChars)
	if err != nil {
		return "" // unreachable: 264 bits are more than 52 characters take
	}
	return text
}

// SessionOfferKey is what an offer made by a signed-in client is proven under: SHA-256 of its
// session's token, the hash the core keeps of the session, which the discovery never sees (the
// protocol's §2.5 item 3; A9, A corrections 5).
func SessionOfferKey(token string) []byte {
	digest := sha256.Sum256([]byte(token))
	return digest[:]
}

// HandOffOfferKey is what a client origin proves its offers under while it holds only the code of a
// sign-in handed off to it, before it has a session of its own (D308 §4): SHA-256 over the code
// under a purpose of its own, so no session's key is ever one. The core takes it for the person the
// sign-in was for, beside their realm key, only while the code is live.
func HandOffOfferKey(code string) []byte {
	digest := sha256.Sum256([]byte(prefix + handOffOfferPurpose + "\n" + code))
	return digest[:]
}

// GuestGrantOfferKey is what a Google Cast device proves its first offers under while it holds only
// a grant for one cast, before it has traded it for a session of its own (D465 §2; google-cast.md
// §2, step 2): SHA-256 over the grant under a purpose of its own, so neither a session's key nor a
// hand-off's is ever one. The core takes it under its sender's locator, beside the sender's own
// session key, only while the grant waits.
func GuestGrantOfferKey(grant string) []byte {
	digest := sha256.Sum256([]byte(prefix + guestGrantOfferPurpose + "\n" + grant))
	return digest[:]
}

// Offer is an offer's proof: an HMAC-SHA256, under key, over the DTLS fingerprint the offer carries
// (its a=fingerprint value, "sha-256 AB:..."), base64url. The key is the client's session's
// (SessionOfferKey) or its link's (link.OfferKey); the discovery holds neither, so the offer it
// passes on is one the client made, for a DTLS certificate the client holds (A9).
func Offer(key []byte, fingerprint string) string {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(prefix + offerPurpose + "\n" + fingerprint)) // a hash's Write never fails
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// VerifyOffer is whether proven is Offer's proof under key over fingerprint, compared in constant time.
func VerifyOffer(key []byte, fingerprint, proven string) bool {
	return hmac.Equal([]byte(Offer(key, fingerprint)), []byte(proven))
}

// ClientManifestMessage is what a core's signature over its client's manifest signs: the manifest's
// exact bytes after the purpose's line. A loader runs a core's client only once the manifest holds
// under the key its link or realm named, and each file's hash in it (D308 §6), so neither the
// discovery serving the loader nor anyone between can put other code in the core's place.
func ClientManifestMessage(manifest []byte) []byte {
	return append([]byte(prefix+clientManifestPurpose+"\n"), manifest...)
}

// VerifyClientManifest is whether signature is publicKey's over manifest.
func VerifyClientManifest(publicKey ed25519.PublicKey, manifest, signature []byte) bool {
	if len(publicKey) != ed25519.PublicKeySize {
		return false
	}
	return ed25519.Verify(publicKey, ClientManifestMessage(manifest), signature)
}
