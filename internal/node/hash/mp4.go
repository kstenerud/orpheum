// SPDX-License-Identifier: GPL-3.0-or-later

package hash

import (
	"encoding/binary"
	"fmt"
	"io"
)

// mp4Payload returns the span from the first mdat box's data to the last mdat box's end. Tag
// editors rewrite moov and leave mdat alone (D179 §3, not yet measured); a file with several
// mdat boxes (fragmented MP4) is anchored from the first to the last.
func mp4Payload(file io.ReaderAt, size int64) (payload, error) {
	first, err := readBox(file, size, 0)
	if err != nil || first.kind != "ftyp" {
		return payload{}, fmt.Errorf("%w: no ftyp box first", errNotThisContainer)
	}
	start, end := int64(-1), int64(-1)
	position := int64(0)
	for range maxElementHops {
		if position >= size {
			if start < 0 {
				return payload{}, fmt.Errorf("%w: no mdat box", errNotThisContainer)
			}
			return payload{start: start, end: end, anchor: AnchorMP4}, nil
		}
		box, err := readBox(file, size, position)
		if err != nil {
			return payload{}, err
		}
		if box.kind == "mdat" {
			if start < 0 {
				start = box.dataStart
			}
			end = box.end
		}
		position = box.end
	}
	return payload{}, fmt.Errorf("%w: too many top-level boxes", errNotThisContainer)
}

// box is one top-level ISO BMFF box.
type box struct {
	kind           string
	dataStart, end int64
}

// readBox reads the box header at position: a 32-bit size and a type, a 64-bit size when the
// first is 1, and "to the end of the file" when it is 0.
func readBox(file io.ReaderAt, size, position int64) (box, error) {
	var header [16]byte
	if err := readAt(file, size, position, header[:8]); err != nil {
		return box{}, err
	}
	declared := int64(binary.BigEndian.Uint32(header[:4]))
	found := box{kind: string(header[4:8]), dataStart: position + 8}
	switch declared {
	case 0:
		found.end = size
		return found, nil
	case 1:
		if err := readAt(file, size, position+8, header[8:16]); err != nil {
			return box{}, err
		}
		large := binary.BigEndian.Uint64(header[8:16])
		if large > uint64(size) { //nolint:gosec // size >= 0
			return box{}, fmt.Errorf("%w: a box at %d larger than the file", errNotThisContainer, position)
		}
		declared = int64(large) //nolint:gosec // bounded by size above
		found.dataStart = position + 16
	}
	if declared < found.dataStart-position || position+declared > size {
		return box{}, fmt.Errorf("%w: a box at %d of %d bytes in a %d-byte file",
			errNotThisContainer, position, declared, size)
	}
	found.end = position + declared
	return found, nil
}
