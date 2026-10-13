// SPDX-License-Identifier: AGPL-3.0-or-later

package link

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
)

// fingerprintChars is a fingerprint's length: a SHA-256 in base64url, unpadded (proof.Fingerprint).
const fingerprintChars = 43

// The labels that separate each hash of a link's secret from any other.
const (
	locateLabel = "orpheum-locate"
	offerLabel  = "orpheum-offer"
)

// ErrNotALink is a fragment that is not a core's link's.
var ErrNotALink = errors.New("not an Orpheum link: want <coreId>.<secret>.<fingerprint>")

// Fragment is what a link carries after `#`: the core's id, the secret, and the identity key's
// fingerprint.
type Fragment struct {
	CoreID, Secret, Fingerprint string
}

// String is the fragment as a link carries it, without the `#`.
func (fragment Fragment) String() string {
	return fragment.CoreID + "." + fragment.Secret + "." + fragment.Fingerprint
}

// Read reads a link's fragment, with or without its `#`: three parts, each of base64url's characters,
// the last a fingerprint's length.
func Read(text string) (Fragment, error) {
	parts := strings.Split(strings.TrimPrefix(text, "#"), ".")
	if len(parts) != 3 || len(parts[2]) != fingerprintChars {
		return Fragment{}, ErrNotALink
	}
	for _, part := range parts {
		if part == "" || strings.Trim(part, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_") != "" {
			return Fragment{}, ErrNotALink
		}
	}
	return Fragment{CoreID: parts[0], Secret: parts[1], Fingerprint: parts[2]}, nil
}

// LocateHash is the hash a core registers a link's secret by: SHA-256 over "orpheum-locate" and the
// secret, base64url, unpadded. A discovery finds the core by it and never learns the secret.
func LocateHash(secret string) string {
	digest := sha256.Sum256([]byte(locateLabel + secret))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}

// OfferKey is what an offer made by a link's bootstrap is proven under: SHA-256 over
// "orpheum-offer" and the secret. The core keeps it when it makes the link and the client derives it
// from the link, so a discovery, which sees neither the secret nor this key, cannot make an offer
// the core takes (the protocol's §2.5 item 3; A9, T7).
func OfferKey(secret string) []byte {
	digest := sha256.Sum256([]byte(offerLabel + secret))
	return digest[:]
}
