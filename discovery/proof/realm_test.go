// SPDX-License-Identifier: AGPL-3.0-or-later

package proof

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"testing"
)

// The realm's vectors, which the web client's tests hold too (web/packages/sdk/src/discovery.test.ts),
// so the page and the core derive each value alike; the key was checked against Python's hashlib.
const (
	realmKeyVector    = "2mjAagkzCF_kAZaUObGZixfFrcDG-39cUXwxtK2hxj0"
	realmPersonVector = "UpaJxwoFfxYyPW1HZo-g-HyN54gftFFBxpa2o5u28UQ"
	realmAskVector    = "KxsLH9QDpNeduWRb9ufhBI1iEY3jLd8igLHF5TZ6nRk"
	realmTagVector    = "oelueZOrkKIG510vkkAhNZ7K7M10Fj1Le9sO6mIs3J4"
	realmOfferVector  = "3C1NBzuWqwkuFATD3QijN5V_0ujLhubShn9Zh7qVqPM"
)

// vectorPrivate is the Ed25519 key of the seed 0x00…0x1f, as the other vectors use.
func vectorPrivate() ed25519.PrivateKey {
	seed := make([]byte, ed25519.SeedSize)
	for index := range seed {
		seed[index] = byte(index)
	}
	return ed25519.NewKeyFromSeed(seed)
}

// vectorKey is vectorPrivate's public half.
func vectorKey() ed25519.PublicKey {
	public, isEd25519 := vectorPrivate().Public().(ed25519.PublicKey)
	if !isEd25519 {
		return nil
	}
	return public
}

// A realm key is PBKDF2-SHA256 of the password, salted by the realm and the name as typed, in any
// case and with spaces around; the person, the ask and the tag are made by it as the web client
// makes them.
func TestRealmKey_IsTheVectorsWhateverTheCaseAndSpacesTyped(t *testing.T) {
	t.Parallel()
	key := RealmKey(" NAS ", "Karl", "a password")
	if got := base64.RawURLEncoding.EncodeToString(key); got != realmKeyVector {
		t.Fatalf("RealmKey = %s; want %s", got, realmKeyVector)
	}
	if got := RealmPerson("nas", "karl"); got != realmPersonVector {
		t.Errorf("RealmPerson = %s; want %s", got, realmPersonVector)
	}
	if got := RealmAsk(key, "nas", realmPersonVector); got != realmAskVector {
		t.Errorf("RealmAsk = %s; want %s", got, realmAskVector)
	}
	if got := RealmTag(key, "k7xq2", vectorKey()); got != realmTagVector {
		t.Errorf("RealmTag = %s; want %s", got, realmTagVector)
	}
	if got := Offer(key, "sha-256 AB:CD"); got != realmOfferVector {
		t.Errorf("Offer under the realm key = %s; want %s", got, realmOfferVector)
	}
}

// Any one of realm, name and password wrong makes another key, and an ask or a tag holds only for
// what it was made over: all three, or nothing (D297 §2).
func TestRealmKey_AnyPartWrongProvesNothing(t *testing.T) {
	t.Parallel()
	key := RealmKey("nas", "karl", "a password")
	for name, other := range map[string][]byte{
		"another realm":    RealmKey("staging", "karl", "a password"),
		"another name":     RealmKey("nas", "anna", "a password"),
		"another password": RealmKey("nas", "karl", "a Password"),
	} {
		if VerifyRealmAsk(other, "nas", realmPersonVector, RealmAsk(key, "nas", realmPersonVector)) {
			t.Errorf("an ask under %s's key holds", name)
		}
	}
	ask := RealmAsk(key, "nas", realmPersonVector)
	if !VerifyRealmAsk(key, "nas", realmPersonVector, ask) ||
		VerifyRealmAsk(key, "nas", RealmPerson("nas", "anna"), ask) {
		t.Error("an ask holds for the person it was made for, and for no other")
	}
	tag := RealmTag(key, "k7xq2", vectorKey())
	other := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize)).Public()
	otherKey, isEd25519 := other.(ed25519.PublicKey)
	if !isEd25519 {
		t.Fatal("no other key")
	}
	if !VerifyRealmTag(key, "k7xq2", vectorKey(), tag) || VerifyRealmTag(key, "k7xq2", otherKey, tag) ||
		VerifyRealmTag(key, "another", vectorKey(), tag) {
		t.Error("a tag holds for the core and the identity key it was made over, and for no other")
	}
}

// A realm key is salted by the master realm alone: a core's subrealm set or changed later leaves the
// key its people's passwords made as it was, while the person and the ask name the whole realm (D297
// §1, §3).
func TestRealmKey_ASubrealmLeavesTheKeyAsItWas(t *testing.T) {
	t.Parallel()
	key := RealmKey("karlnas", "karl", "a password")
	if !bytes.Equal(RealmKey("karlnas.live", "karl", "a password"), key) {
		t.Error("a subrealm changed the key")
	}
	if MasterRealm("KarlNAS.Live") != "karlnas" {
		t.Errorf("MasterRealm(KarlNAS.Live) = %q; want karlnas", MasterRealm("KarlNAS.Live"))
	}
	if RealmPerson("karlnas.live", "karl") == RealmPerson("karlnas.test", "karl") {
		t.Error("two subrealms name the same person")
	}
}
