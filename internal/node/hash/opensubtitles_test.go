// SPDX-License-Identifier: GPL-3.0-or-later

package hash_test

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/kstenerud/orpheum/internal/node/hash"
)

func TestOpenSubtitles_IsTheSizePlusTheWordsOfBothEndsAsSixteenHexDigits(t *testing.T) {
	t.Parallel()
	const size = 200_000
	file := make([]byte, size)
	// Only the first and the last 64 KiB count: a word in each, and one between that does not.
	binary.LittleEndian.PutUint64(file[8:], 0x0102030405060708)
	binary.LittleEndian.PutUint64(file[size-16:], 0xfffffffffffffff0) // wraps
	binary.LittleEndian.PutUint64(file[100_000:], 0xdead)
	got, err := hash.OpenSubtitlesHash(bytes.NewReader(file), size)
	if err != nil {
		t.Fatal(err)
	}
	// 200,000 (0x30d40) + 0x0102030405060708 + 0xfffffffffffffff0, wrapping at 64 bits.
	if want := "0102030405091438"; got != want {
		t.Errorf("OpenSubtitles = %q, want %q: the size plus both ends' words, wrapping (D224 §3)", got, want)
	}
	if small, err := hash.OpenSubtitlesHash(bytes.NewReader(file[:131_072]), 131_072); err != nil || small != "" {
		t.Errorf("a file of 128 KiB = %q, %v; want none, as OpenSubtitles hashes only larger files", small, err)
	}
}
