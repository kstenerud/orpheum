// SPDX-License-Identifier: GPL-3.0-or-later

package hash

import (
	"encoding/binary"
	"fmt"
	"io"
)

// openSubtitlesChunk is how much of each end of a file OpenSubtitles' moviehash reads.
const openSubtitlesChunk = 64 << 10

// OpenSubtitles is the file's OpenSubtitles moviehash, the provider's own identifier for these
// exact bytes (D224 §3; opensubtitles-api.md §4): the file's size plus the sum of its first and
// last 64 KiB read as little-endian 64-bit words, wrapping, as 16 hex digits. A file of 128 KiB
// or less has none, and is "". It is written from OpenSubtitles' description, not their code.
func OpenSubtitles(file io.ReaderAt, size int64) (string, error) {
	if size <= 2*openSubtitlesChunk {
		return "", nil
	}
	sum := uint64(size)
	buffer := make([]byte, openSubtitlesChunk)
	for _, offset := range []int64{0, size - openSubtitlesChunk} {
		if err := readAt(file, size, offset, buffer); err != nil {
			return "", fmt.Errorf("the OpenSubtitles hash: %w", err)
		}
		for word := 0; word < openSubtitlesChunk; word += 8 {
			sum += binary.LittleEndian.Uint64(buffer[word:])
		}
	}
	return fmt.Sprintf("%016x", sum), nil
}
