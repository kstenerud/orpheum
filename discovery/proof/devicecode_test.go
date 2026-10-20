// SPDX-License-Identifier: AGPL-3.0-or-later

package proof

import (
	"crypto/ecdh"
	"crypto/hpke"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"testing"
)

// The outcome web/apps/web/src/device-outcome.test.ts opens too, a P-256 key's scalar and what was
// sealed to it, so the core's seal and the browser's open are held to one blob.
const (
	fixturePrivate = "xeruaKT9s4YsierbGsVdrQk8RzKuhPC2k453oBOIjJo"
	fixturePublic  = "BMbJRSPCf69nfVclsx77C19vrA1JS1lRUkmpU5kSQdr53_JGKknN2W0eAMmPYZsdP_8JtRtLggy7CKKoOkhGKLg"
	fixtureSealed  = "BBfNci1ok_9cjsv6uhX90qCwN1b0RE50DPnYo7SGlux2s80NY108R0TvHVo3KPY5KazNfFHeG4Dn2fkeKvMVP-oMf-wohuEBLdcVzhTfDBoozkA_2d4u2pjXsE4v0fovTLENqE7jS8rhl16UXX561ZmEeQvGVa-qMS2DPXOjJj2gFt2xjAiyjzdaCYcj8sMcectPCpFj7F_Ck0aPQDoQl9EP-KmgHpUkhUZQau8x1DIH97qZ4VJIY8NM3VHZFTleUkqSxfuW2XOQrWF5iw" //nolint:lll // one blob
)

var fixtureOutcome = DeviceOutcome{
	CoreID: "k7xq2", Fingerprint: "Vkdap1RjR0wChd9dvyvKtz2mUTWIOem3dIGy6rEHcIw", Realm: "nas",
	HandOffCode: "a-hand-off-code",
}

// fixtureKey is the fixture's private key, as HPKE opens with it.
func fixtureKey(t *testing.T) hpke.PrivateKey {
	t.Helper()
	scalar, err := base64.RawURLEncoding.DecodeString(fixturePrivate)
	if err != nil {
		t.Fatal(err)
	}
	private, err := ecdh.P256().NewPrivateKey(scalar)
	if err != nil {
		t.Fatal(err)
	}
	key, err := hpke.NewDHKEMPrivateKey(private)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

// newDeviceKey is a device's fresh key pair: its private half, and its public half as the device
// sends it.
func newDeviceKey(t *testing.T) (hpke.PrivateKey, string) {
	t.Helper()
	private, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	key, err := hpke.NewDHKEMPrivateKey(private)
	if err != nil {
		t.Fatal(err)
	}
	return key, base64.RawURLEncoding.EncodeToString(private.PublicKey().Bytes())
}

func TestSealDeviceOutcome_OpensOnlyWithTheDevicesKeyAndAsTheBrowserOpensIt(t *testing.T) {
	t.Parallel()
	private, public := newDeviceKey(t)
	sealed, err := SealDeviceOutcome(public, fixtureOutcome)
	if err != nil {
		t.Fatal(err)
	}
	if opened, err := OpenDeviceOutcome(private, sealed); err != nil || opened != fixtureOutcome {
		t.Errorf("opened %+v, %v; want %+v", opened, err, fixtureOutcome)
	}
	other, _ := newDeviceKey(t)
	if _, err := OpenDeviceOutcome(other, sealed); err == nil {
		t.Error("another key opened the outcome")
	}
	// The blob the browser's test opens is one this seal makes.
	if opened, err := OpenDeviceOutcome(fixtureKey(t), fixtureSealed); err != nil || opened != fixtureOutcome {
		t.Errorf("the fixture opened as %+v, %v; want %+v", opened, err, fixtureOutcome)
	}
	if _, err := SealDeviceOutcome("bm90LWEta2V5", fixtureOutcome); !errors.Is(err, ErrNotADeviceKey) {
		t.Errorf("sealing to a key that is not one: %v; want ErrNotADeviceKey", err)
	}
}

// The fixture's nonces, the device's and the core's, which web/packages/sdk/src/device-code.test.ts
// holds too.
const (
	fixtureDeviceNonce = "AAECAwQFBgcICQoLDA0ODw"
	fixtureCoreNonce   = "EBESExQVFhcYGRobHB0eHw"
)

// The check code is made of the key and both nonces (D321), alike in the browser; another key, or
// another nonce of either side's, makes another, and a key or nonce that is none is refused. Renamed
// from TestDeviceCheckCode_IsTheKeysAndTheBrowsersAlike, when the code was the key's alone.
func TestDeviceCheckCode_IsTheKeysAndBothNoncesAndTheBrowsersAlike(t *testing.T) {
	t.Parallel()
	check, err := DeviceCheckCode(fixturePublic, fixtureDeviceNonce, fixtureCoreNonce)
	// The browser's test makes the same code of the same key and nonces.
	if err != nil || check != "dx9y-jewz" {
		t.Errorf("the fixture's check code = %q, %v; want dx9y-jewz", check, err)
	}
	_, other := newDeviceKey(t)
	if otherCheck, err := DeviceCheckCode(other, fixtureDeviceNonce, fixtureCoreNonce); err != nil ||
		otherCheck == check {
		t.Error("another key made the same check code")
	}
	if otherCheck, err := DeviceCheckCode(fixturePublic, fixtureDeviceNonce, fixtureDeviceNonce); err != nil ||
		otherCheck == check {
		t.Error("another core nonce made the same check code")
	}
	if _, err := DeviceCheckCode("", fixtureDeviceNonce, fixtureCoreNonce); !errors.Is(err, ErrNotADeviceKey) {
		t.Errorf("no key: %v; want ErrNotADeviceKey", err)
	}
	if _, err := DeviceCheckCode(fixturePublic, "short", fixtureCoreNonce); !errors.Is(err, ErrNotADeviceNonce) {
		t.Errorf("a short nonce: %v; want ErrNotADeviceNonce", err)
	}
}

// The device's commitment is its key's and its nonce's, alike in the browser, and the QR code's key
// hash is 128 bits of its key's (D321).
func TestDeviceCommitmentAndKeyHash_AreTheBrowsersAlike(t *testing.T) {
	t.Parallel()
	commitment, err := DeviceCommitment(fixturePublic, fixtureDeviceNonce)
	if err != nil || commitment != "T9zJEIeNk5Oc91YyHh_bg_HeQpq9RZ9huSJhR3SrysU" {
		t.Errorf("the fixture's commitment = %q, %v", commitment, err)
	}
	if err = CheckDeviceCommitment(commitment); err != nil {
		t.Errorf("a commitment refused: %v", err)
	}
	if err = CheckDeviceCommitment("short"); !errors.Is(err, ErrNotACommitment) {
		t.Errorf("a short commitment: %v; want ErrNotACommitment", err)
	}
	hash, err := DeviceKeyHash(fixturePublic)
	if err != nil || hash != "rTBVo-NI60s-RZ3ENfKqkw" {
		t.Errorf("the fixture's key hash = %q, %v", hash, err)
	}
}

func TestDeviceCodePerson_IsTheCodesAloneAndNoRealmPersons(t *testing.T) {
	t.Parallel()
	if DeviceCodePerson("abcd-efgh") != "wsj_BSEPwklSSobhc9kIdKbAoWwh8XnVH6Xn6ifDfa0" {
		t.Errorf("DeviceCodePerson = %q", DeviceCodePerson("abcd-efgh"))
	}
	if DeviceCodePerson("abcd-efgh") == DeviceCodePerson("abcd-efgj") {
		t.Error("two codes name the same person")
	}
	if DeviceCodePerson("abcd-efgh") == RealmPerson("abcd", "efgh") {
		t.Error("a device code's person is a realm person's")
	}
}
