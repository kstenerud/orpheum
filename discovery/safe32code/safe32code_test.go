// SPDX-License-Identifier: AGPL-3.0-or-later

package safe32code

import (
	"errors"
	"testing"
)

func TestDeviceCode_IsSafe32AndReadsWhatAPersonTypes(t *testing.T) {
	t.Parallel()
	if got := Device([DeviceBytes]byte{0, 0, 0, 0, 0}); got != "0000-0000" {
		t.Errorf("zero = %q", got)
	}
	if got := Device([DeviceBytes]byte{0xff, 0xff, 0xff, 0xff, 0xff}); got != "zzzz-zzzz" {
		t.Errorf("all ones = %q", got)
	}
	// 0x0123456789 is 00000 00100 10001 10100 01010 11001 11100 01001: 0, 4, h, m, a, s, w, 9.
	if got := Device([DeviceBytes]byte{0x01, 0x23, 0x45, 0x67, 0x89}); got != "04hm-asw9" {
		t.Errorf("0x0123456789 = %q", got)
	}
	for typed, want := range map[string]string{
		"04hm-asw9": "04hm-asw9", "04HM ASW9": "04hm-asw9", "o4hmasw9": "04hm-asw9",
		"lIuv-0000": "11vv-0000", " 11vv - 0000 ": "11vv-0000",
	} {
		if got, err := Read(typed, DeviceLength); err != nil || got != want {
			t.Errorf("Read(%q) = %q, %v; want %q", typed, got, err, want)
		}
	}
	for _, typed := range []string{"", "04hm-asy", "04hm-asy99", "04hm-as!9"} {
		if _, err := Read(typed, DeviceLength); err == nil {
			t.Errorf("Read(%q) read a code", typed)
		}
	}
}

// A value wider than 64 bits is written as Write writes a narrower one: the same bits, most
// significant first, the same groups.
func TestWriteBytes_WritesTheBitsAsWriteDoes(t *testing.T) {
	t.Parallel()
	if got, err := WriteBytes([]byte{0x01, 0x23, 0x45, 0x67, 0x89}, DeviceLength); err != nil || got != "04hm-asw9" {
		t.Errorf("0x0123456789 = %q, %v; want 04hm-asw9, as Device writes it", got, err)
	}
	// 130 bits: 0xff00 eight times, then the top two bits of 0xc0.
	wide := []byte{0xff, 0, 0xff, 0, 0xff, 0, 0xff, 0, 0xff, 0, 0xff, 0, 0xff, 0, 0xff, 0, 0xc0}
	if got, err := WriteBytes(wide, 26); err != nil || got != "zw0f-y07z-03zg-1zr0-zw0f-y07z-03" {
		t.Errorf("130 bits = %q, %v", got, err)
	}
	if _, err := WriteBytes(wide, 28); !errors.Is(err, ErrTooFewBytes) {
		t.Errorf("28 characters from 136 bits = %v; want ErrTooFewBytes", err)
	}
}
