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

// ParseKey is an authorised key's line, "<type> <base64> [comment]", as a key a discovery may hold:
// one of allowedKeyTypes.
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

// isAllowedType is whether a root, delegated or online key may be of keyType: a hardware key's
// Ed25519 or ECDSA P-256 (a YubiKey's), or a plain Ed25519 key, an operator's without a hardware key
// and the tests' (D391 §2).
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

// verifySSH is whether armoured is the SSH signature of a key among keys over message in namespace;
// an error says why it is not.
func verifySSH(armoured, namespace string, message []byte, keys []ssh.PublicKey) error {
	body, err := unarmour(armoured)
	if err != nil {
		return err
	}
	if body.Namespace != namespace {
		return fmt.Errorf("%w: %q, not %q", ErrWrongNamespace, body.Namespace, namespace)
	}
	signer, err := ssh.ParsePublicKey(body.PublicKey)
	if err != nil {
		return fmt.Errorf("%w: its key: %w", ErrNotASignature, err)
	}
	if !isAllowedType(signer.Type()) {
		return fmt.Errorf("%s: %w", signer.Type(), ErrKeyType)
	}
	trusted := false
	for _, key := range keys {
		trusted = trusted || bytes.Equal(key.Marshal(), signer.Marshal())
	}
	if !trusted {
		return fmt.Errorf("%w: %s", ErrUntrustedKey, Fingerprint(signer))
	}
	var signature ssh.Signature
	if err = ssh.Unmarshal(body.Signature, &signature); err != nil {
		return fmt.Errorf("%w: its signature: %w", ErrNotASignature, err)
	}
	data, err := signedData(namespace, body.Hash, message)
	if err != nil {
		return err
	}
	if err = signer.Verify(data, &signature); err != nil {
		return fmt.Errorf("%w: %w", ErrBadSignature, err)
	}
	return nil
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

// signSSH is signer's SSH signature over message in namespace, armoured as ssh-keygen writes it.
func signSSH(signer ssh.Signer, namespace string, message []byte) (string, error) {
	if !isAllowedType(signer.PublicKey().Type()) {
		return "", fmt.Errorf("%s: %w", signer.PublicKey().Type(), ErrKeyType)
	}
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
