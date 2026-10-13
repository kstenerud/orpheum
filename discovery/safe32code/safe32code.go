// SPDX-License-Identifier: AGPL-3.0-or-later

package safe32code

import (
	"errors"
	"strings"
)

// alphabet is safe32's: digits and lowercase letters without i, l, o and u, which a person reads as
// 1, 1, 0 and v.
const alphabet = "0123456789abcdefghjkmnpqrstvwxyz"

// DeviceLength is a device code's length, and a claim code's: one full safe32 group of 40 bits
// (D218 §3).
const DeviceLength = 8

// DeviceBytes is the random bytes behind a device code.
const DeviceBytes = 5

// ErrNotACode is text that cannot be a code, however it is read.
var ErrNotACode = errors.New("not a code")

// ErrTooFewBytes is WriteBytes given fewer bits than the code's characters need.
var ErrTooFewBytes = errors.New("fewer bytes than the code's characters need")

// Device writes five bytes as a device code, "xxxx-xxxx": the 40 bits big-endian, five to a
// character.
func Device(raw [DeviceBytes]byte) string {
	var value uint64
	for _, b := range raw {
		value = value<<8 | uint64(b)
	}
	return Write(value, DeviceLength)
}

// Write writes the low five bits per character of value as a code of length characters, the most
// significant first, in groups of four joined by dashes: the one way a code a person types is
// written (D223 §2).
func Write(value uint64, length int) string {
	written := make([]byte, length)
	for index := length - 1; index >= 0; index-- {
		written[index] = alphabet[value&31]
		value >>= 5
	}
	return grouped(written)
}

// WriteBytes writes the first five bits per character of raw, most significant first, as a code of
// length characters, grouped as Write groups them: for a value wider than Write's 64 bits, such as a
// core's 130-bit id. Raw holding fewer than length*5 bits is ErrTooFewBytes.
func WriteBytes(raw []byte, length int) (string, error) {
	if len(raw)*8 < length*5 {
		return "", ErrTooFewBytes
	}
	written := make([]byte, length)
	for index := range written {
		var value byte
		for bit := index * 5; bit < index*5+5; bit++ {
			value = value<<1 | raw[bit/8]>>(7-bit%8)&1
		}
		written[index] = alphabet[value]
	}
	return grouped(written), nil
}

// Read reads what a person typed as the code of length characters it means, as safe32 allows: either
// case, o for 0, l or i for 1, u for v, and dashes and spaces ignored.
func Read(typed string, length int) (string, error) {
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
		if !strings.ContainsRune(alphabet, char) || len(read) == length {
			return "", ErrNotACode
		}
		read = append(read, byte(char))
	}
	if len(read) != length {
		return "", ErrNotACode
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
