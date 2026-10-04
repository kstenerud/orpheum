// SPDX-License-Identifier: GPL-3.0-or-later

package secrets

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/chacha20poly1305"

	"github.com/kstenerud/orpheum/internal/platform/randbytes"
)

// KeyFileName is the key's file in the instance's persistent directory (D198, D242 §3), mode 0600,
// never in the database.
const KeyFileName = "secret.key"

// keyFileHeader is the key file's first line: its format and version, so a later format can
// still read this one (migrations.md, Our files, read later).
const keyFileHeader = "orpheum-secret-key v1"

// Key seals and opens secrets with XChaCha20-Poly1305. Its ID is recorded with each sealed row,
// so a row sealed under another key is known as such rather than failing to open (D194 §3, §4).
type Key struct {
	id       int64
	material [chacha20poly1305.KeySize]byte
}

// GenerateKey makes a new 256-bit key from random.
func GenerateKey(random io.Reader) (Key, error) {
	raw, err := randbytes.Read(random, chacha20poly1305.KeySize)
	if err != nil {
		return Key{}, fmt.Errorf("generate a secret key: %w", err)
	}
	material := [chacha20poly1305.KeySize]byte(raw)
	return Key{id: keyID(material), material: material}, nil
}

// ID identifies the key without revealing it.
func (key Key) ID() int64 {
	return key.id
}

// EncodeKeyFile renders the key as secret.key's contents.
func (key Key) EncodeKeyFile() []byte {
	return []byte(keyFileHeader + "\n" + base64.StdEncoding.EncodeToString(key.material[:]) + "\n")
}

// ParseKeyFile reads secret.key's contents.
func ParseKeyFile(contents []byte) (Key, error) {
	header, body, found := bytes.Cut(contents, []byte("\n"))
	if !found || string(header) != keyFileHeader {
		return Key{}, errors.New("secret.key: not an Orpheum secret key (want the header " + keyFileHeader + ")")
	}
	material, err := base64.StdEncoding.DecodeString(string(bytes.TrimSpace(body)))
	if err != nil {
		return Key{}, fmt.Errorf("secret.key: %w", err)
	}
	var key Key
	if len(material) != len(key.material) {
		return Key{}, fmt.Errorf("secret.key: the key is %d bytes, want %d", len(material), len(key.material))
	}
	copy(key.material[:], material)
	key.id = keyID(key.material)
	return key, nil
}

// keyID derives a stable, positive identifier from the key, domain-separated so it reveals
// nothing usable about the key itself.
func keyID(material [chacha20poly1305.KeySize]byte) int64 {
	digest := sha256.Sum256(append([]byte("orpheum secret key id\x00"), material[:]...))
	return int64(binary.BigEndian.Uint64(digest[:8]) >> 1)
}
