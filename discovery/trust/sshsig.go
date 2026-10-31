// SPDX-License-Identifier: AGPL-3.0-or-later

package trust

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"errors"
	"fmt"
	"slices"
	"strings"

	"golang.org/x/crypto/ssh"
)

// An SSH signature (OpenSSH's PROTOCOL.sshsig), as `ssh-keygen -Y sign` writes it: armoured, the blob
// "SSHSIG", its version, the signer's public key, the namespace, a reserved string, the hash and the
// signature, over "SSHSIG", the namespace, the reserved string, the hash's name and the message's hash.
const (
	sshsigMagic   = "SSHSIG"
	sshsigVersion = 1
	armourBegin   = "-----BEGIN SSH SIGNATURE-----"
	armourEnd     = "-----END SSH SIGNATURE-----"
	// The base64 line length ssh-keygen writes.
	armourWidth = 70
)

// The hashes an SSH signature may be over: ssh-keygen's default, and the other PROTOCOL.sshsig names.
const (
	hashSHA512 = "sha512"
	hashSHA256 = "sha256"
)

// Errors a signature's check returns.
var (
	ErrNotASignature  = errors.New("not an SSH signature (ssh-keygen -Y sign)")
	ErrWrongNamespace = errors.New("the signature was made for another kind of object")
	ErrUntrustedKey   = errors.New("the signature is by no key trusted for it")
	ErrBadSignature   = errors.New("the signature does not hold over what it signs")
	ErrKeyType        = errors.New("not a key type a discovery's keys may be: " +
		"sk-ssh-ed25519, sk-ecdsa-sha2-nistp256 or ssh-ed25519")
)

// sshsigBody is the signature's blob after its magic.
type sshsigBody struct {
	Version   uint32
	PublicKey []byte
	Namespace string
	Reserved  string
	Hash      string
	Signature []byte
}

// signedPart is what the key signs, after the magic.
type signedPart struct {
	Namespace string
	Reserved  string
	Hash      string
	Digest    []byte
}

// ParseKey is an authorised key's line, "<type> <base64> [comment]", as a root key may be: of an
// allowed type (isAllowedType).
func ParseKey(line string) (ssh.PublicKey, error) {
	key, _, options, _, err := ssh.ParseAuthorizedKey([]byte(line))
	if err != nil {
		return nil, fmt.Errorf("read the key %q: %w", line, err)
	}
	if len(options) != 0 {
		return nil, fmt.Errorf("the key %q: a discovery's key has no options", line)
	}
	if !isAllowedType(key.Type()) {
		return nil, fmt.Errorf("%s: %w", key.Type(), ErrKeyType)
	}
	return key, nil
}

// KeyLine is key as an authorised key's line without a comment, the form a discovery's keys are
// written in.
func KeyLine(key ssh.PublicKey) string {
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key)))
}

// Fingerprint is key's SHA-256 fingerprint as `ssh-keygen -l` shows it, "SHA256:…", which a person
// compares with the one their key shows.
func Fingerprint(key ssh.PublicKey) string {
	return ssh.FingerprintSHA256(key)
}

// isAllowedType is whether a root key may be of keyType: a hardware key's Ed25519 or ECDSA P-256 (a
// YubiKey's), or a plain Ed25519 key, the paper recovery key's or an operator's without a hardware key
// (the root key audit's F16; D393).
func isAllowedType(keyType string) bool {
	switch keyType {
	case ssh.KeyAlgoSKED25519, ssh.KeyAlgoSKECDSA256, ssh.KeyAlgoED25519:
		return true
	}
	return false
}

// digestOf is message's hash by the named algorithm.
func digestOf(hash string, message []byte) ([]byte, error) {
	switch hash {
	case hashSHA512:
		digest := sha512.Sum512(message)
		return digest[:], nil
	case hashSHA256:
		digest := sha256.Sum256(message)
		return digest[:], nil
	}
	return nil, fmt.Errorf("%w: hash %q", ErrNotASignature, hash)
}

// signedData is what a key signs for message in namespace under hash.
func signedData(namespace, hash string, message []byte) ([]byte, error) {
	digest, err := digestOf(hash, message)
	if err != nil {
		return nil, err
	}
	part := signedPart{Namespace: namespace, Hash: hash, Digest: digest}
	return append([]byte(sshsigMagic), ssh.Marshal(part)...), nil
}

// userVerified is the flag a hardware key sets in its signature when its PIN was given (FIDO's UV).
const userVerified = 0x04

// Key is a key a signature may be by: its public half, and whether every signature by it must carry
// the user-verified flag (a "uv" key line).
type Key struct {
	Public       ssh.PublicKey
	MustVerifyUV bool
}

// Block is one SSH signature of a signature file, as the watcher reports it: its key's fingerprint,
// its namespace, and for a hardware key its flags and counter.
type Block struct {
	Fingerprint, Namespace string
	Flags                  byte
	Counter                uint32
}

// verifyBlocks are the keys among keys whose SSH signature, one of the armoured blocks in file, holds
// over message in namespace (the root key audit's §3.2): the namespace the one expected, never the
// block's own word for it; the hash SHA-512 or SHA-256; the key one of keys, byte for byte, of an
// allowed type; its signature holding, a hardware key's user-present; and for a key marked so, the
// user-verified flag set. A block that fails any of them signs nothing; none at all is an error.
func verifyBlocks(file, namespace string, message []byte, keys []Key) ([]Key, error) {
	bodies, err := unarmourAll(file)
	if err != nil {
		return nil, err
	}
	var signers []Key
	var refusals []error
	for _, body := range bodies {
		key, blockErr := verifyBlock(body, namespace, message, keys)
		if blockErr != nil {
			refusals = append(refusals, blockErr)
			continue
		}
		if !slices.ContainsFunc(signers, func(known Key) bool { return sameKey(known.Public, key.Public) }) {
			signers = append(signers, key)
		}
	}
	if len(signers) == 0 {
		return nil, errors.Join(append([]error{ErrUntrustedKey}, refusals...)...)
	}
	return signers, nil
}

// verifyBlock is the key of keys whose signature body is over message in namespace.
func verifyBlock(body sshsigBody, namespace string, message []byte, keys []Key) (Key, error) {
	if body.Namespace != namespace {
		return Key{}, fmt.Errorf("%w: %q, not %q", ErrWrongNamespace, body.Namespace, namespace)
	}
	signer, err := ssh.ParsePublicKey(body.PublicKey)
	if err != nil {
		return Key{}, fmt.Errorf("%w: its key: %w", ErrNotASignature, err)
	}
	if !isAllowedType(signer.Type()) {
		return Key{}, fmt.Errorf("%s: %w", signer.Type(), ErrKeyType)
	}
	index := slices.IndexFunc(keys, func(key Key) bool { return sameKey(key.Public, signer) })
	if index < 0 {
		return Key{}, fmt.Errorf("%w: %s", ErrUntrustedKey, Fingerprint(signer))
	}
	var signature ssh.Signature
	if err = ssh.Unmarshal(body.Signature, &signature); err != nil {
		return Key{}, fmt.Errorf("%w: its signature: %w", ErrNotASignature, err)
	}
	data, err := signedData(namespace, body.Hash, message)
	if err != nil {
		return Key{}, err
	}
	if err = signer.Verify(data, &signature); err != nil {
		return Key{}, fmt.Errorf("%w: %w", ErrBadSignature, err)
	}
	if keys[index].MustVerifyUV {
		if flags, _, isHardware := hardwareFields(&signature); !isHardware || flags&userVerified == 0 {
			return Key{}, fmt.Errorf("%w: %s", ErrNotUserVerified, Fingerprint(signer))
		}
	}
	return keys[index], nil
}

// ErrNotUserVerified is a signature by a key that must be user-verified, made without its PIN.
var ErrNotUserVerified = errors.New("the signature was made without the key's PIN (no user-verified flag)")

// hardwareFields are a hardware key's signature's flags and counter, and whether it is one's.
func hardwareFields(signature *ssh.Signature) (byte, uint32, bool) {
	if signature.Format != ssh.KeyAlgoSKED25519 && signature.Format != ssh.KeyAlgoSKECDSA256 {
		return 0, 0, false
	}
	var fields struct {
		Flags   byte
		Counter uint32
	}
	if ssh.Unmarshal(signature.Rest, &fields) != nil {
		return 0, 0, false
	}
	return fields.Flags, fields.Counter, true
}

// Inspect is each block of a signature file as it stands, checking nothing: what the watcher reports.
func Inspect(file string) ([]Block, error) {
	bodies, err := unarmourAll(file)
	if err != nil {
		return nil, err
	}
	blocks := make([]Block, 0, len(bodies))
	for _, body := range bodies {
		block := Block{Namespace: body.Namespace}
		if key, keyErr := ssh.ParsePublicKey(body.PublicKey); keyErr == nil {
			block.Fingerprint = Fingerprint(key)
		}
		var signature ssh.Signature
		if ssh.Unmarshal(body.Signature, &signature) == nil {
			block.Flags, block.Counter, _ = hardwareFields(&signature)
		}
		blocks = append(blocks, block)
	}
	return blocks, nil
}

// sameKey is whether two keys are one, byte for byte.
func sameKey(one, other ssh.PublicKey) bool {
	return bytes.Equal(one.Marshal(), other.Marshal())
}

// unarmourAll are the blobs of every armoured block in file, concatenated as ssh-keygen's output may
// be; none is an error.
func unarmourAll(file string) ([]sshsigBody, error) {
	var bodies []sshsigBody
	rest := file
	for {
		start := strings.Index(rest, armourBegin)
		if start < 0 {
			break
		}
		end := strings.Index(rest[start:], armourEnd)
		if end < 0 {
			return nil, ErrNotASignature
		}
		body, err := unarmour(rest[start : start+end+len(armourEnd)])
		if err != nil {
			return nil, err
		}
		bodies = append(bodies, body)
		rest = rest[start+end+len(armourEnd):]
	}
	if len(bodies) == 0 || strings.TrimSpace(rest) != "" {
		return nil, ErrNotASignature
	}
	return bodies, nil
}

// unarmour is armoured's blob after its magic, of the one version there is.
func unarmour(armoured string) (sshsigBody, error) {
	text := strings.TrimSpace(armoured)
	if !strings.HasPrefix(text, armourBegin) || !strings.HasSuffix(text, armourEnd) {
		return sshsigBody{}, ErrNotASignature
	}
	encoded := strings.Join(strings.Fields(text[len(armourBegin):len(text)-len(armourEnd)]), "")
	blob, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || !bytes.HasPrefix(blob, []byte(sshsigMagic)) {
		return sshsigBody{}, ErrNotASignature
	}
	var body sshsigBody
	if err = ssh.Unmarshal(blob[len(sshsigMagic):], &body); err != nil || body.Version != sshsigVersion {
		return sshsigBody{}, ErrNotASignature
	}
	return body, nil
}

// Sign is signer's SSH signature over payload in namespace, armoured as `ssh-keygen -Y sign` writes
// it: how a test, the development discovery and a plain key sign. The owner's root keys sign with
// ssh-keygen itself (cmd/discovery-sign).
func Sign(signer ssh.Signer, namespace string, payload []byte) (string, error) {
	return signSSH(signer, namespace, payload)
}

// signSSH is signer's SSH signature over message in namespace, armoured as ssh-keygen writes it.
func signSSH(signer ssh.Signer, namespace string, message []byte) (string, error) {
	data, err := signedData(namespace, hashSHA512, message)
	if err != nil {
		return "", err
	}
	signature, err := signer.Sign(rand.Reader, data)
	if err != nil {
		return "", fmt.Errorf("sign: %w", err)
	}
	blob := append([]byte(sshsigMagic), ssh.Marshal(sshsigBody{
		Version: sshsigVersion, PublicKey: signer.PublicKey().Marshal(), Namespace: namespace, Hash: hashSHA512,
		Signature: ssh.Marshal(signature),
	})...)
	encoded := base64.StdEncoding.EncodeToString(blob)
	var armoured strings.Builder
	armoured.WriteString(armourBegin + "\n")
	for len(encoded) > armourWidth {
		armoured.WriteString(encoded[:armourWidth] + "\n")
		encoded = encoded[armourWidth:]
	}
	armoured.WriteString(encoded + "\n" + armourEnd + "\n")
	return armoured.String(), nil
}
