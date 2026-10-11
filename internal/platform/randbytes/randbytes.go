// SPDX-License-Identifier: AGPL-3.0-or-later

package randbytes

import (
	"encoding/base64"
	"fmt"
	"io"
)

// Read is byteCount bytes from random, all of them or an error.
func Read(random io.Reader, byteCount int) ([]byte, error) {
	raw := make([]byte, byteCount)
	if _, err := io.ReadFull(random, raw); err != nil {
		return nil, fmt.Errorf("read %d random bytes: %w", byteCount, err)
	}
	return raw, nil
}

// Base64URLSecret is byteCount bytes from random in unpadded base64url: a bearer secret, such as a
// session's or a media token.
func Base64URLSecret(random io.Reader, byteCount int) (string, error) {
	raw, err := Read(random, byteCount)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}
