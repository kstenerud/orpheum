// SPDX-License-Identifier: AGPL-3.0-or-later

package proof

import (
	"crypto/ecdh"
	"crypto/hpke"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"

	"github.com/kstenerud/orpheum/discovery/safe32code"
)

// A device code through the paid service (D293 §2): the device's P-256 key is the one thing the
// service passes between the device and the core that both can check, so the check code is made
// of it, and the outcome is sealed to it.
const (
	deviceCodePersonPurpose  = "device-code-person"
	deviceCheckCodePurpose   = "device-check-code"
	deviceOutcomeInfoPurpose = "device-outcome"
)

// ErrNotADeviceKey is a device's public key that is not an uncompressed P-256 point in base64url.
var ErrNotADeviceKey = errors.New("not a device's P-256 public key")

// DeviceCodePerson is who a device code's hand-off is for, as the bootstrap token the service gives
// the device names it: the SHA-256 of the code under a purpose of its own, base64url, so the core
// takes the hand-off's key for it (HandOffOfferKey) as for a realm sign-in's person, and the service,
// which knows the code, names no person.
func DeviceCodePerson(code string) string {
	digest := sha256.Sum256([]byte(prefix + deviceCodePersonPurpose + "\n" + code))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}

// DeviceCheckCode is the check code both the device and the person's page show (D288 §3) for a
// device asking with publicKey, base64url as it was sent: a device code's 40 bits of the key's
// SHA-256 under a purpose of its own. A service that swapped in a key of its own would show the
// person another code than the device's.
func DeviceCheckCode(publicKey string) (string, error) {
	raw, err := deviceKey(publicKey)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(append([]byte(prefix+deviceCheckCodePurpose+"\n"), raw...))
	return safe32code.Device([safe32code.DeviceBytes]byte(digest[:safe32code.DeviceBytes])), nil
}

// DeviceOutcome is what an activated device needs, sealed to its key so the service passes it on
// unread: the core's id and identity key's fingerprint, which the device checks what it reaches by,
// the realm it signs in by, the code of the session the core keeps for its client origin to take
// (D308 §4), and the name of the person it signs in as, which the device asks its person to confirm
// before going on (the device code audit's M1).
type DeviceOutcome struct {
	CoreID      string `json:"coreId"`
	Fingerprint string `json:"fingerprint"`
	Realm       string `json:"realm"`
	HandOffCode string `json:"handOffCode"`
	Person      string `json:"person,omitempty"`
}

// SealDeviceOutcome seals outcome to the device's publicKey, base64url, with HPKE's base mode
// (RFC 9180) over DHKEM(P-256, HKDF-SHA256), HKDF-SHA256 and AES-128-GCM, whose every part Web
// Crypto has (D293 §2): the encapsulated key then the ciphertext, base64url.
func SealDeviceOutcome(publicKey string, outcome DeviceOutcome) (string, error) {
	raw, err := deviceKey(publicKey)
	if err != nil {
		return "", err
	}
	recipient, err := hpke.DHKEM(ecdh.P256()).NewPublicKey(raw)
	if err != nil {
		return "", errors.Join(ErrNotADeviceKey, err)
	}
	plaintext, err := json.Marshal(outcome)
	if err != nil {
		return "", err
	}
	sealed, err := hpke.Seal(recipient, hpke.HKDFSHA256(), hpke.AES128GCM(), deviceOutcomeInfo(), plaintext)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(sealed), nil
}

// OpenDeviceOutcome opens what SealDeviceOutcome sealed to privateKey's public half: the client's
// own work in the browser, here for its tests and a native client's.
func OpenDeviceOutcome(privateKey hpke.PrivateKey, sealed string) (DeviceOutcome, error) {
	raw, err := base64.RawURLEncoding.DecodeString(sealed)
	if err != nil {
		return DeviceOutcome{}, err
	}
	plaintext, err := hpke.Open(privateKey, hpke.HKDFSHA256(), hpke.AES128GCM(), deviceOutcomeInfo(), raw)
	if err != nil {
		return DeviceOutcome{}, err
	}
	var outcome DeviceOutcome
	if err := json.Unmarshal(plaintext, &outcome); err != nil {
		return DeviceOutcome{}, err
	}
	return outcome, nil
}

// deviceOutcomeInfo is HPKE's info for a device's outcome, which binds a sealed blob to this use.
func deviceOutcomeInfo() []byte {
	return []byte(prefix + deviceOutcomeInfoPurpose)
}

// deviceKey is a device's public key as bytes: an uncompressed P-256 point, 65 bytes starting 0x04.
func deviceKey(publicKey string) ([]byte, error) {
	raw, err := base64.RawURLEncoding.DecodeString(publicKey)
	if err != nil || len(raw) != uncompressedP256Length || raw[0] != uncompressedPointTag {
		return nil, ErrNotADeviceKey
	}
	return raw, nil
}

// An uncompressed P-256 point: the tag, then its two 32-byte coordinates.
const (
	uncompressedP256Length = 65
	uncompressedPointTag   = 0x04
)
