// SPDX-License-Identifier: AGPL-3.0-or-later

package secrets

import (
	"encoding/binary"
	"errors"
	"fmt"

	"golang.org/x/crypto/chacha20poly1305"

	"github.com/kstenerud/orpheum/internal/platform/randbytes"
)

// A bound value is one sealed for a single row of another table rather than kept in the secret
// table: a person's own link, kept so their page can show it again (discovery-core.md K7; D224 §9's
// "a feature that needs one brings it back"). It is sealed under secret.key as every secret is, and
// bound to its row by the identity its owner names, so it opens nowhere else.

// boundKeyIDSize is the bytes of the key's id at the head of a bound value.
const boundKeyIDSize = 8

// ErrBoundUnopened is a bound value that does not open: sealed under another key, as after a
// restore without secret.key, or not the row's.
var ErrBoundUnopened = errors.New("the sealed value does not open under this key for this row")

// SealBound seals text for the row whose identity is given: the key's id, a nonce and the
// ciphertext, in one value its row keeps.
func (secrets *Store) SealBound(identity, text string) ([]byte, error) {
	nonce, err := randbytes.Read(secrets.random, chacha20poly1305.NonceSizeX)
	if err != nil {
		return nil, fmt.Errorf("seal a bound value: make a nonce: %w", err)
	}
	cipher, err := chacha20poly1305.NewX(secrets.key.material[:])
	if err != nil {
		return nil, fmt.Errorf("seal a bound value: %w", err)
	}
	sealed := binary.BigEndian.AppendUint64(nil, uint64(secrets.key.ID())) //nolint:gosec // the id's bits, kept whole
	sealed = append(sealed, nonce...)
	return cipher.Seal(sealed, nonce, []byte(text), boundAssociatedData(identity)), nil
}

// OpenBound opens a value SealBound sealed for the same identity; ErrBoundUnopened otherwise.
func (secrets *Store) OpenBound(identity string, sealed []byte) (string, error) {
	head := boundKeyIDSize + chacha20poly1305.NonceSizeX
	if len(sealed) < head {
		return "", ErrBoundUnopened
	}
	if int64(binary.BigEndian.Uint64(sealed)) != secrets.key.ID() { //nolint:gosec // the id's bits, kept whole
		return "", ErrBoundUnopened
	}
	cipher, err := chacha20poly1305.NewX(secrets.key.material[:])
	if err != nil {
		return "", fmt.Errorf("open a bound value: %w", err)
	}
	plain, err := cipher.Open(nil, sealed[boundKeyIDSize:head], sealed[head:], boundAssociatedData(identity))
	if err != nil {
		return "", ErrBoundUnopened
	}
	return string(plain), nil
}

// boundAssociatedData is a bound value's associated data: its row's identity, in a namespace of its
// own, so neither a server secret's ciphertext nor another row's opens as it.
func boundAssociatedData(identity string) []byte {
	return []byte("bound\x00" + identity)
}
