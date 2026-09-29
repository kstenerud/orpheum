// SPDX-License-Identifier: GPL-3.0-or-later

package hash

import (
	"encoding/binary"
	"fmt"
	"io"
)

// openSubtitlesChunkBytes is how much of each end of a file OpenSubtitles' moviehash reads.
const openSubtitlesChunkBytes = 64 << 10

// OpenSubtitlesHash is the file's OpenSubtitles moviehash, the provider's own identifier for these
// exact bytes (D224 §3; opensubtitles-api.md §4): the file's size plus the sum of its first and
// last 64 KiB read as little-endian 64-bit words, wrapping, as 16 hex digits. A file of 128 KiB
// or less has none, and is "". It is written from OpenSubtitles' description, not their code.
func OpenSubtitlesHash(file io.ReaderAt, sizeBytes int64) (string, error) {
	if sizeBytes <= 2*openSubtitlesChunkBytes {
		return "", nil
	}
	sum := uint64(sizeBytes)
	buffer := make([]byte, openSubtitlesChunkBytes)
	for _, offset := range []int64{0, sizeBytes - openSubtitlesChunkBytes} {
		if err := readAt(file, sizeBytes, offset, buffer); err != nil {
			return "", fmt.Errorf("the OpenSubtitles hash: %w", err)
		}
		for word := 0; word < openSubtitlesChunkBytes; word += 8 {
			sum += binary.LittleEndian.Uint64(buffer[word:])
		}
	}
	return fmt.Sprintf("%016x", sum), nil
}
