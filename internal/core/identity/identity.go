// SPDX-License-Identifier: AGPL-3.0-or-later

package identity

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"io"

	"github.com/kstenerud/orpheum/discovery/proof"
	"github.com/kstenerud/orpheum/internal/core/instancedir"
)

// KeyFileName is the identity key's file in the instance's persistent directory, beside secret.key,
// mode 0600, never in the database (D214 §4, D273).
const KeyFileName = "identity.key"

// keyFileHeader is the key file's first line: its format and version, so a later format can still
// read this one.
const keyFileHeader = "orpheum-identity-key v1"

// Key is the core's identity: an Ed25519 key pair made on its first start that never changes for the
// life of the instance. A discovery knows the core by its public key, and a client can pin it
// (D214 §4). It is not the TLS key, which changes with each certificate.
type Key struct {
	private ed25519.PrivateKey
}

// GenerateKey makes a new identity key from random.
func GenerateKey(random io.Reader) (Key, error) {
	_, private, err := ed25519.GenerateKey(random)
	if err != nil {
		return Key{}, fmt.Errorf("generate an identity key: %w", err)
	}
	return Key{private: private}, nil
}

// Public is the key's public half.
func (key Key) Public() ed25519.PublicKey {
	public, isEd25519 := key.private.Public().(ed25519.PublicKey)
	if !isEd25519 {
		return nil
	}
	return public
}

// Sign signs a discovery's nonce for purpose: "enrol" or "session".
func (key Key) Sign(purpose, nonce string) []byte {
	return ed25519.Sign(key.private, proof.Message(purpose, nonce))
}

// EncodeKeyFile renders the key as identity.key's contents: the seed, from which both halves follow.
func (key Key) EncodeKeyFile() []byte {
	return []byte(keyFileHeader + "\n" + base64.StdEncoding.EncodeToString(key.private.Seed()) + "\n")
}

// ParseKeyFile reads identity.key's contents.
func ParseKeyFile(contents []byte) (Key, error) {
	header, body, found := bytes.Cut(contents, []byte("\n"))
	if !found || string(header) != keyFileHeader {
		return Key{}, errors.New(KeyFileName + ": not an Orpheum identity key (want the header " + keyFileHeader + ")")
	}
	seed, err := base64.StdEncoding.DecodeString(string(bytes.TrimSpace(body)))
	if err != nil {
		return Key{}, fmt.Errorf("%s: %w", KeyFileName, err)
	}
	if len(seed) != ed25519.SeedSize {
		return Key{}, fmt.Errorf("%s: the seed is %d bytes, want %d", KeyFileName, len(seed), ed25519.SeedSize)
	}
	return Key{private: ed25519.NewKeyFromSeed(seed)}, nil
}

// Files are where the key is kept: instancedir.Directories.
type Files interface {
	ReadPersistentFile(name string) ([]byte, error)
	WritePersistentFile(name string, contents []byte) error
}

// LoadOrMake reads the instance's identity key, making and keeping one the first time.
func LoadOrMake(files Files, random io.Reader) (Key, error) {
	contents, err := files.ReadPersistentFile(KeyFileName)
	if err == nil {
		return ParseKeyFile(contents)
	}
	if !errors.Is(err, instancedir.ErrNotFound) {
		return Key{}, fmt.Errorf("read %s: %w", KeyFileName, err)
	}
	key, err := GenerateKey(random)
	if err != nil {
		return Key{}, err
	}
	if err = files.WritePersistentFile(KeyFileName, key.EncodeKeyFile()); err != nil {
		return Key{}, fmt.Errorf("keep %s: %w", KeyFileName, err)
	}
	return key, nil
}
