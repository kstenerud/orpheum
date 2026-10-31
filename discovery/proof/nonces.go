// SPDX-License-Identifier: AGPL-3.0-or-later

package proof

import (
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"slices"
	"sync"
	"time"
)

// A nonce carries its own expiry and a MAC under a key only its issuer holds, so issuing one keeps
// nothing: no count of strangers' challenges fills a table (a cap of 256 outstanding nonces let one
// refuse the core's own sessions, the security audit's S13). What is kept is each nonce a signature
// spent, until it expires, so each works once. A nonce is 8 bytes of expiry, nonceRandomBytes
// random, and nonceMACBytes of HMAC-SHA256 over both, in unpadded base64url.
const (
	nonceRandomBytes = 16
	nonceMACBytes    = 16
	nonceKeyBytes    = 32
	nonceBodyBytes   = 8 + nonceRandomBytes
)

// Nonces issues the nonces a core signs to enrol and to open a session, and spends each a valid
// signature uses: the replay guard of every signed core call, which a discovery, Orpheum's own or
// the paid service, holds one of (D343). Its key lives in memory: a restart makes a new one, and
// the nonces issued before it end with it, so a core asks for another.
type Nonces struct {
	key   []byte
	mutex sync.Mutex
	// spent is each nonce a signature used, until it expires; guarded by mutex.
	spent map[string]time.Time
}

// NewNonces is an issuer of nonces with a new key from random.
func NewNonces(random io.Reader) (*Nonces, error) {
	key := make([]byte, nonceKeyBytes)
	if _, err := io.ReadFull(random, key); err != nil {
		return nil, fmt.Errorf("make the nonces' key: %w", err)
	}
	return &Nonces{key: key, spent: map[string]time.Time{}}, nil
}

// mac is the MAC of a nonce's expiry and random bytes under the issuer's key.
func (nonces *Nonces) mac(body []byte) []byte {
	mac := hmac.New(sha256.New, nonces.key)
	_, _ = mac.Write(body) // a hash's Write never fails
	return mac.Sum(nil)[:nonceMACBytes]
}

// Issue is a new nonce expiring at expires, its random bytes from random; it keeps nothing.
func (nonces *Nonces) Issue(random io.Reader, expires time.Time) (string, error) {
	body := binary.BigEndian.AppendUint64(make([]byte, 0, nonceBodyBytes),
		uint64(expires.UnixMilli()))
	body = body[:nonceBodyBytes]
	if _, err := io.ReadFull(random, body[8:]); err != nil {
		return "", fmt.Errorf("make a nonce: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(slices.Concat(body, nonces.mac(body))), nil
}

// SignedNonce is a core's signature over a nonce, and what it must be bound to: its purpose, the
// discovery's issuer and the core (Verify).
type SignedNonce struct {
	PublicKey ed25519.PublicKey
	Purpose   string
	Issuer    string
	Core      string
	Nonce     string
	Signature []byte
}

// Spend is whether signed is its key's signature over a nonce these nonces issued, unexpired at now
// and unspent, for its purpose, discovery and core; a nonce a valid signature used is spent, and kept
// until it expires.
func (nonces *Nonces) Spend(now time.Time, signed SignedNonce) bool {
	raw, err := base64.RawURLEncoding.DecodeString(signed.Nonce)
	if err != nil || len(raw) != nonceBodyBytes+nonceMACBytes ||
		!hmac.Equal(raw[nonceBodyBytes:], nonces.mac(raw[:nonceBodyBytes])) {
		return false
	}
	expires := time.UnixMilli(int64(binary.BigEndian.Uint64(raw))) //nolint:gosec // written from a time above
	if !now.Before(expires) ||
		!Verify(signed.PublicKey, signed.Purpose, signed.Issuer, signed.Core, signed.Nonce, signed.Signature) {
		return false
	}
	nonces.mutex.Lock()
	defer nonces.mutex.Unlock()
	for each, until := range nonces.spent {
		if !now.Before(until) {
			delete(nonces.spent, each)
		}
	}
	if _, isSpent := nonces.spent[signed.Nonce]; isSpent {
		return false
	}
	nonces.spent[signed.Nonce] = expires
	return true
}
