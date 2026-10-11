// SPDX-License-Identifier: AGPL-3.0-or-later

package auth

import (
	"errors"
	"strings"
)

// safe32Alphabet is safe32's (github.com/kstenerud/safe-encoding): digits and lowercase letters
// without i, l, o and u, which a person reads as 1, 1, 0 and v.
const safe32Alphabet = "0123456789abcdefghjkmnpqrstvwxyz"

// codeChars is a device code's length: one full safe32 group of 40 bits (D218 §3).
const codeChars = 8

// codeBytes is the random bytes behind a device code.
const codeBytes = 5

// errNotACode is text that cannot be a code, however it is read.
var errNotACode = errors.New("not a code")

// encodeCode writes five bytes as a device code, "xxxx-xxxx": the 40 bits big-endian, five to a
// character.
func encodeCode(raw [codeBytes]byte) string {
	var value uint64
	for _, b := range raw {
		value = value<<8 | uint64(b)
	}
	return safe32Code(value, codeChars)
}

// normalizeCode reads what a person typed as the device code it means, "xxxx-xxxx".
func normalizeCode(typed string) (string, error) {
	return readSafe32(typed, codeChars)
}

// safe32Code writes the low five bits per character of value as a code of chars characters, the
// most significant first, in groups of four joined by dashes: the one way a code a person types
// is written (D223 §2).
func safe32Code(value uint64, length int) string {
	written := make([]byte, length)
	for index := length - 1; index >= 0; index-- {
		written[index] = safe32Alphabet[value&31]
		value >>= 5
	}
	return grouped(written)
}

// readSafe32 reads what a person typed as the code of chars characters it means, as safe32
// allows: either case, o for 0, l or i for 1, u for v, and dashes and spaces ignored.
func readSafe32(typed string, length int) (string, error) {
	read := make([]byte, 0, length)
	for _, char := range strings.ToLower(typed) {
		switch char {
		case '-', ' ', '\t', '\n', '\r':
			continue
		case 'o':
			char = '0'
		case 'l', 'i':
			char = '1'
		case 'u':
			char = 'v'
		}
		if !strings.ContainsRune(safe32Alphabet, char) || len(read) == length {
			return "", errNotACode
		}
		read = append(read, byte(char))
	}
	if len(read) != length {
		return "", errNotACode
	}
	return grouped(read), nil
}

// grouped joins a code's characters in groups of four with dashes.
func grouped(characters []byte) string {
	var text strings.Builder
	for index, char := range characters {
		if index > 0 && index%4 == 0 {
			text.WriteByte('-')
		}
		text.WriteByte(char)
	}
	return text.String()
}
