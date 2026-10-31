// SPDX-License-Identifier: AGPL-3.0-or-later

package proof

import (
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/pbkdf2"
	"crypto/sha256"
	"encoding/base64"
	"strings"
)

// RealmRounds is how many PBKDF2-SHA256 rounds make a person's realm key: OWASP's figure for
// PBKDF2-HMAC-SHA256, so a discovery that holds a proof pays them for each password it guesses
// (D297 §3).
const RealmRounds = 600_000

// realmKeyBytes is a realm key's length: SHA-256's.
const realmKeyBytes = 32

// The purposes of what a realm key makes, apart from an offer's proof, which Offer makes under it.
const (
	realmKeyPurpose    = "realm-key"
	realmPersonPurpose = "realm-person"
	realmAskPurpose    = "realm-ask"
	realmCorePurpose   = "realm-core"
)

// RealmName is a realm or a person's name as a realm key takes it: without the spaces around it,
// in lower case, so "Karl" typed is the account karl (D297 §1).
func RealmName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// MasterRealm is the realm a host's account holds, which a realm names before any subrealm:
// "karlnas" of "karlnas.test" (D297 §1).
func MasterRealm(realm string) string {
	master, _, _ := strings.Cut(RealmName(realm), ".")
	return master
}

// RealmKey is the key a person's realm sign-in is proven by, both ways: PBKDF2-SHA256 of their
// password, RealmRounds rounds, salted by the master realm and their name, so a core's subrealm set
// or changed later leaves every key as it was. The page derives it from what its person typed; the
// core, whenever it sees their password, and keeps it (D297 §3).
func RealmKey(realm, name, password string) []byte {
	salt := []byte(prefix + realmKeyPurpose + "\n" + MasterRealm(realm) + "\n" + RealmName(name))
	key, err := pbkdf2.Key(sha256.New, password, salt, RealmRounds, realmKeyBytes)
	if err != nil {
		return nil // unreachable: SHA-256 and a key this short are always allowed
	}
	return key
}

// RealmPerson is who a realm sign-in is for, as the discovery passes it to the core: the SHA-256 of
// the realm and the name, base64url, which tells the core whose key to check by and tells the
// discovery no name.
func RealmPerson(realm, name string) string {
	digest := sha256.Sum256([]byte(prefix + realmPersonPurpose + "\n" + RealmName(realm) + "\n" + RealmName(name)))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}

// RealmAsk is a page's proof that it holds person's realm key, which it asks the discovery for its
// realm's core with: an HMAC-SHA256 under the key over the realm and the person, base64url. The
// discovery cannot check it, and passes it to the core, which can.
func RealmAsk(key []byte, realm, person string) string {
	return mac(key, realmAskPurpose+"\n"+RealmName(realm)+"\n"+person)
}

// VerifyRealmAsk is whether proven is RealmAsk's proof under key, compared in constant time.
func VerifyRealmAsk(key []byte, realm, person, proven string) bool {
	return hmac.Equal([]byte(RealmAsk(key, realm, person)), []byte(proven))
}

// RealmTag is the core's answer to a realm sign-in: an HMAC-SHA256 under the person's key over the
// core's id and its identity key, base64url. Only the core and the person hold the key, so a page
// that checks it knows the identity key is the core's, not a discovery's posing as it to harvest the
// password the sign-in sends next (D297 §3).
func RealmTag(key []byte, coreID string, publicKey ed25519.PublicKey) string {
	return mac(key, realmCorePurpose+"\n"+coreID+"\n"+KeyLabel(publicKey))
}

// VerifyRealmTag is whether proven is RealmTag's tag under key, compared in constant time.
func VerifyRealmTag(key []byte, coreID string, publicKey ed25519.PublicKey, proven string) bool {
	return hmac.Equal([]byte(RealmTag(key, coreID, publicKey)), []byte(proven))
}

// mac is an HMAC-SHA256 under key over the protocol's prefix and text, base64url.
func mac(key []byte, text string) string {
	hashed := hmac.New(sha256.New, key)
	_, _ = hashed.Write([]byte(prefix + text)) // a hash's Write never fails
	return base64.RawURLEncoding.EncodeToString(hashed.Sum(nil))
}
