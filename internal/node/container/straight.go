// SPDX-License-Identifier: AGPL-3.0-or-later

package container

import (
	"bytes"
	"io"
)

// ReadsStraightThrough is whether FFmpeg can read the file from a pipe, in one pass from its start,
// as it would read the file itself: a Matroska or WebM file, an MPEG transport stream, or an MP4 whose
// index (moov) comes before its media (mdat). An MP4 whose index sits at its end, and any container
// not known here, needs to seek, so it is read as a file (D329 §7). It reads a few bytes at the
// start, and an MP4's top-level box headers until the first of moov or mdat. A failure to read is
// "no": the file is read as a file, as before.
func ReadsStraightThrough(file io.ReaderAt, sizeBytes int64) bool {
	var start [transportPacketBytes + 1]byte
	if ReadAt(file, sizeBytes, 0, start[:min(int64(len(start)), sizeBytes)]) != nil {
		return false
	}
	switch {
	case bytes.HasPrefix(start[:], matroskaMagic):
		return true
	case sizeBytes > transportPacketBytes && start[0] == transportSync && start[transportPacketBytes] == transportSync:
		return true
	}
	return mp4IndexComesFirst(file, sizeBytes)
}

// matroskaMagic begins every Matroska and WebM file: its EBML header's ID.
var matroskaMagic = []byte{0x1A, 0x45, 0xDF, 0xA3} //nolint:gochecknoglobals // a constant byte string

// An MPEG transport stream is packets of 188 bytes, each beginning with its sync byte.
const (
	transportPacketBytes = 188
	transportSync        = 0x47
)

// mp4IndexComesFirst is whether an ISO BMFF file's moov box comes before its first mdat box.
func mp4IndexComesFirst(file io.ReaderAt, sizeBytes int64) bool {
	position := int64(0)
	for range MaxElementHops {
		if position >= sizeBytes {
			return false
		}
		box, err := ReadBox(file, sizeBytes, position)
		if err != nil {
			return false
		}
		switch box.Kind {
		case "moov":
			return true
		case "mdat":
			return false
		}
		position = box.End
	}
	return false
}
