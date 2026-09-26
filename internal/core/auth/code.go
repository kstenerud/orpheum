// SPDX-License-Identifier: GPL-3.0-or-later

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

// errNotACode is text that cannot be a device code, however it is read.
var errNotACode = errors.New("not a device code")

// encodeCode writes five bytes as a device code, "xxxx-xxxx": the 40 bits big-endian, five to a
// character.
func encodeCode(raw [codeBytes]byte) string {
	var value uint64
	for _, b := range raw {
		value = value<<8 | uint64(b)
	}
	chars := make([]byte, codeChars)
	for index := codeChars - 1; index >= 0; index-- {
		chars[index] = safe32Alphabet[value&31]
		value >>= 5
	}
	return string(chars[:4]) + "-" + string(chars[4:])
}

// normalizeCode reads what a person typed as the code it means, "xxxx-xxxx", as safe32 allows:
// either case, o for 0, l or i for 1, u for v, and dashes and spaces ignored.
func normalizeCode(typed string) (string, error) {
	chars := make([]byte, 0, codeChars)
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
		if !strings.ContainsRune(safe32Alphabet, char) || len(chars) == codeChars {
			return "", errNotACode
		}
		chars = append(chars, byte(char))
	}
	if len(chars) != codeChars {
		return "", errNotACode
	}
	return string(chars[:4]) + "-" + string(chars[4:]), nil
}
