// SPDX-License-Identifier: GPL-3.0-or-later

package randbytes

import (
	"encoding/base64"
	"fmt"
	"io"
)

// Read is count bytes from random, all of them or an error.
func Read(random io.Reader, count int) ([]byte, error) {
	raw := make([]byte, count)
	if _, err := io.ReadFull(random, raw); err != nil {
		return nil, fmt.Errorf("read %d random bytes: %w", count, err)
	}
	return raw, nil
}

// Base64URLSecret is count bytes from random in unpadded base64url: a bearer secret, such as a
// session's or a media token.
func Base64URLSecret(random io.Reader, count int) (string, error) {
	raw, err := Read(random, count)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}
