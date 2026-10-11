// SPDX-License-Identifier: AGPL-3.0-or-later

package container

import (
	"encoding/binary"
	"fmt"
	"io"
)

// Box is one ISO BMFF box: its type, and where its data lies.
type Box struct {
	Kind           string
	DataStart, End int64
}

// ReadBox reads the box header at position, in a file of sizeBytes or a parent box ending there:
// a 32-bit size and a type, a 64-bit size when the first is 1, and "to the end" when it is 0.
func ReadBox(file io.ReaderAt, sizeBytes, position int64) (Box, error) {
	var header [16]byte
	if err := ReadAt(file, sizeBytes, position, header[:8]); err != nil {
		return Box{}, err
	}
	declared := int64(binary.BigEndian.Uint32(header[:4]))
	found := Box{Kind: string(header[4:8]), DataStart: position + 8}
	switch declared {
	case 0:
		found.End = sizeBytes
		return found, nil
	case 1:
		if err := ReadAt(file, sizeBytes, position+8, header[8:16]); err != nil {
			return Box{}, err
		}
		large := binary.BigEndian.Uint64(header[8:16])
		if large > uint64(sizeBytes) { //nolint:gosec // sizeBytes >= 0
			return Box{}, fmt.Errorf("%w: a box at %d larger than the file", ErrNotThisContainer, position)
		}
		declared = int64(large) //nolint:gosec // bounded by sizeBytes above
		found.DataStart = position + 16
	}
	if declared < found.DataStart-position || position+declared > sizeBytes {
		return Box{}, fmt.Errorf("%w: a box at %d of %d bytes in a %d-byte file",
			ErrNotThisContainer, position, declared, sizeBytes)
	}
	found.End = position + declared
	return found, nil
}
