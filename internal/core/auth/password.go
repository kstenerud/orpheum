// SPDX-License-Identifier: AGPL-3.0-or-later

package auth

import (
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"

	"golang.org/x/crypto/argon2"

	"github.com/kstenerud/orpheum/internal/platform/randbytes"
)

// Argon2Parameters are argon2id's costs. The hash carries them as a PHC string, so they can change
// later and an old hash still verifies (D194 §2).
type Argon2Parameters struct {
	MemoryKiB   uint32
	Iterations  uint32
	Parallelism uint8
	SaltBytes   int
	KeyBytes    uint32
}

// ErrMalformedHash is returned for a stored hash that is not an argon2id PHC string.
var ErrMalformedHash = errors.New("not an argon2id PHC string")

const phcPrefix = "$argon2id$v=19$"

// HashPassword hashes password with argon2id under parameters and a fresh salt, as a PHC string:
// $argon2id$v=19$m=65536,t=3,p=4$<salt>$<hash>.
func HashPassword(password string, parameters Argon2Parameters, random io.Reader) (string, error) {
	salt, err := randbytes.Read(random, parameters.SaltBytes)
	if err != nil {
		return "", fmt.Errorf("make a salt: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, parameters.Iterations, parameters.MemoryKiB,
		parameters.Parallelism, parameters.KeyBytes)
	encoding := base64.RawStdEncoding
	return fmt.Sprintf("%sm=%d,t=%d,p=%d$%s$%s", phcPrefix, parameters.MemoryKiB, parameters.Iterations,
		parameters.Parallelism, encoding.EncodeToString(salt), encoding.EncodeToString(key)), nil
}

// VerifyPassword reports whether password matches hash, comparing in constant time, and the
// parameters the hash was made with, so the caller can re-hash one made under older costs.
func VerifyPassword(password, hash string) (isMatch bool, hashParameters Argon2Parameters, err error) {
	parsed, err := parsePHC(hash)
	if err != nil {
		return false, Argon2Parameters{}, err
	}
	hashParameters = parsed.parameters
	got := argon2.IDKey([]byte(password), parsed.salt, hashParameters.Iterations, hashParameters.MemoryKiB,
		hashParameters.Parallelism, hashParameters.KeyBytes)
	return subtle.ConstantTimeCompare(got, parsed.key) == 1, hashParameters, nil
}

// phcHash is a parsed PHC string.
type phcHash struct {
	parameters Argon2Parameters
	salt       []byte
	key        []byte
}

// maximumKeyBytes bounds a stored hash's key length; argon2id keys are tens of bytes.
const maximumKeyBytes = 1024

func parsePHC(hash string) (phcHash, error) {
	var parsed phcHash
	rest, found := strings.CutPrefix(hash, phcPrefix)
	if !found {
		return parsed, ErrMalformedHash
	}
	fields := strings.Split(rest, "$")
	if len(fields) != 3 {
		return parsed, ErrMalformedHash
	}
	var parallelism uint8
	_, err := fmt.Sscanf(fields[0], "m=%d,t=%d,p=%d",
		&parsed.parameters.MemoryKiB, &parsed.parameters.Iterations, &parallelism)
	if err != nil || parallelism == 0 || parsed.parameters.Iterations == 0 {
		return parsed, fmt.Errorf("%w: parameters %q", ErrMalformedHash, fields[0])
	}
	parsed.parameters.Parallelism = parallelism
	parsed.salt, err = base64.RawStdEncoding.DecodeString(fields[1])
	if err != nil {
		return parsed, fmt.Errorf("%w: salt: %w", ErrMalformedHash, err)
	}
	parsed.key, err = base64.RawStdEncoding.DecodeString(fields[2])
	if err != nil || len(parsed.key) == 0 || len(parsed.key) > maximumKeyBytes {
		return parsed, fmt.Errorf("%w: key", ErrMalformedHash)
	}
	parsed.parameters.SaltBytes = len(parsed.salt)
	parsed.parameters.KeyBytes = uint32(len(parsed.key)) //nolint:gosec // G115: bounded by maximumKeyBytes above
	return parsed, nil
}

// sameCosts reports whether a hash made under used needs no re-hash under want.
func sameCosts(used, current Argon2Parameters) bool {
	return used.MemoryKiB == current.MemoryKiB && used.Iterations == current.Iterations &&
		used.Parallelism == current.Parallelism && used.KeyBytes == current.KeyBytes &&
		used.SaltBytes == current.SaltBytes
}
