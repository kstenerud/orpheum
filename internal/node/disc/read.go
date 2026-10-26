// SPDX-License-Identifier: AGPL-3.0-or-later

package disc

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// sectorBytes is a disc's sector, and a UDF logical block on every video disc.
const sectorBytes = 2048

// Bounds on what one image may make the node read or keep: far past any real disc's, so a hostile
// or broken image is refused rather than walked without end.
const (
	maxDescriptorSectors = 64        // a volume descriptor sequence
	maxDirectoryBytes    = 1 << 20   // one directory's entries
	maxExtentsPerFile    = 4096      // a file's allocation descriptors, continuations included
	maxStructureBytes    = 8 << 20   // an IFO or a playlist read whole
	maxPlaylists         = 4096      // a Blu-ray's playlists (an obfuscated disc has some hundreds)
	maxEntriesPerDir     = 8192      // names in one directory
	maxReadBytes         = 256 << 20 // all a disc's structure, read to find its main title
	maxTitleCells        = 4096      // the cells of a DVD title, across its program chains
)

// The image's errors.
var (
	// ErrNotADisc is a file that is not a DVD's or a Blu-ray's image: no UDF volume, or neither
	// VIDEO_TS nor BDMV in it.
	ErrNotADisc = errors.New("not a DVD or Blu-ray disc image")
	// ErrMalformed is an image whose structure contradicts itself or passes the node's bounds.
	ErrMalformed = errors.New("the disc image is malformed")
	// ErrNoTitle is a disc with no title to play.
	ErrNoTitle = errors.New("the disc has no title to play")
	// ErrEncrypted is a disc whose title is still encrypted (CSS or AACS), which the node does not
	// decrypt (D350 §5).
	ErrEncrypted = errors.New("the disc is encrypted")
	// ErrInterleaved is a DVD title whose cells are interleaved, for angles or seamless branching,
	// which the node does not follow yet (D350 §5).
	ErrInterleaved = errors.New("the disc's main title is interleaved (angles or seamless branching)")
)

// ReadError is a read of the image that failed: the storage's error, not the image's fault.
type ReadError struct{ Err error }

func (err *ReadError) Error() string { return "read the disc image: " + err.Err.Error() }
func (err *ReadError) Unwrap() error { return err.Err }

// readExactly reads length bytes of image at offset, refusing a read past sizeBytes as malformed.
func readExactly(image io.ReaderAt, sizeBytes, offset, length int64) ([]byte, error) {
	if offset < 0 || length < 0 || offset > sizeBytes || length > sizeBytes-offset {
		return nil, fmt.Errorf("%w: %d bytes at %d of an image of %d", ErrMalformed, length, offset, sizeBytes)
	}
	buffer := make([]byte, length)
	if _, err := image.ReadAt(buffer, offset); err != nil {
		return nil, &ReadError{Err: err}
	}
	return buffer, nil
}

// read reads length bytes of the image at offset, within the volume's budget for reading its
// structure, so an image whose structures point at each other cannot make the node read without end.
func (disc *volume) read(offset, length int64) ([]byte, error) {
	disc.readBytes += length
	if disc.readBytes > maxReadBytes {
		return nil, fmt.Errorf("%w: its structure takes more than %d bytes to read", ErrMalformed, maxReadBytes)
	}
	return readExactly(disc.image, disc.sizeBytes, offset, length)
}

// field reads within a structure already in memory, each read checked against its end, so no
// malformed offset can panic: a read past the end leaves ok false and returns zero.
type field struct {
	data []byte
	ok   bool
}

func fieldsOf(data []byte) *field { return &field{data: data, ok: true} }

func (f *field) bytes(offset, length int) []byte {
	if offset < 0 || length < 0 || offset > len(f.data) || length > len(f.data)-offset {
		f.ok = false
		return make([]byte, min(max(length, 0), 8)) // enough for a number; a run is checked before use
	}
	return f.data[offset : offset+length]
}

func (f *field) u8(offset int) int { return int(f.bytes(offset, 1)[0]) }

func (f *field) be16(offset int) int { return int(binary.BigEndian.Uint16(f.bytes(offset, 2))) }

func (f *field) be32(offset int) int64 { return int64(binary.BigEndian.Uint32(f.bytes(offset, 4))) }

func (f *field) le16(offset int) int { return int(binary.LittleEndian.Uint16(f.bytes(offset, 2))) }

func (f *field) le32(offset int) int64 { return int64(binary.LittleEndian.Uint32(f.bytes(offset, 4))) }

func (f *field) le64(offset int) int64 {
	value := binary.LittleEndian.Uint64(f.bytes(offset, 8))
	if value > 1<<62 {
		f.ok = false
		return 0
	}
	return int64(value)
}

// err is ErrMalformed, naming what was read, when any read went past the structure's end.
func (f *field) err(what string) error {
	if !f.ok {
		return fmt.Errorf("%w: %s is cut short", ErrMalformed, what)
	}
	return nil
}
