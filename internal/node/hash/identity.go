// SPDX-License-Identifier: GPL-3.0-or-later

package hash

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// Scheme names what the identity hash reads and how. A change to either is a new scheme, and
// hashes of different schemes are unrelated: the core never compares them.
const Scheme = "payload-v1"

// window is how much of each end of the payload the hash reads (D116: 1 MB head and tail).
const window = 1 << 20

// Anchor is what the hash was anchored on.
type Anchor string

// The anchors, most specific first. A container the node cannot separate into header and
// payload is hashed whole (D179 §3: MPEG-TS and raw streams keep D116's head, tail and size).
const (
	AnchorMatroska  Anchor = "matroska-clusters"
	AnchorMP4       Anchor = "mp4-mdat"
	AnchorWholeFile Anchor = "whole-file"
)

// Identity is a file's identity: a sampling hash of its media payload and the payload's length
// (D179). A tag edit leaves it unchanged; a remux, which re-lays the payload, changes it.
type Identity struct {
	Hash          [sha256.Size]byte
	PayloadLength int64
	Anchor        Anchor
}

// payload is the byte range the hash is anchored on.
type payload struct {
	start, end int64
	anchor     Anchor
}

// errNotThisContainer means a parser does not recognise the file; the next one is tried.
var errNotThisContainer = errors.New("not this container")

// Payload returns the identity of a file of size bytes, read through file. It reads a few
// small headers, two windows of at most a megabyte, and nothing else.
func Payload(file io.ReaderAt, size int64) (Identity, error) {
	found, err := locate(file, size)
	if err != nil {
		return Identity{}, err
	}
	length := found.end - found.start
	headLength := min(window, length)
	tailLength := min(window, length-headLength)

	digest := sha256.New()
	prefix := fmt.Appendf(nil, "orpheum %s %s\n", Scheme, found.anchor)
	prefix = binary.BigEndian.AppendUint64(prefix, uint64(length)) //nolint:gosec // length is never negative
	if _, err := digest.Write(prefix); err != nil {
		return Identity{}, fmt.Errorf("hash: %w", err)
	}
	for _, span := range [][2]int64{{found.start, headLength}, {found.end - tailLength, tailLength}} {
		if _, err := io.Copy(digest, io.NewSectionReader(file, span[0], span[1])); err != nil {
			return Identity{}, fmt.Errorf("read the payload at %d: %w", span[0], err)
		}
	}
	identity := Identity{PayloadLength: length, Anchor: found.anchor}
	digest.Sum(identity.Hash[:0])
	return identity, nil
}

// locate finds the payload: Matroska's clusters, MP4's mdat, or else the whole file. A file
// that claims to be Matroska or MP4 but cannot be followed — truncated, or with a cluster of
// unknown size — is hashed whole rather than refused, so a file still being written still has
// an identity; it gets its payload identity once it is complete, which is a new file (D173).
func locate(file io.ReaderAt, size int64) (payload, error) {
	for _, parse := range []func(io.ReaderAt, int64) (payload, error){matroskaPayload, mp4Payload} {
		found, err := parse(file, size)
		if err == nil {
			return found, nil
		}
		if readErr, isReadError := errors.AsType[*readError](err); isReadError {
			return payload{}, readErr
		}
	}
	return payload{start: 0, end: size, anchor: AnchorWholeFile}, nil
}

// readError is a failure to read the file, as opposed to a file the parser does not follow;
// the first is returned, the second falls through to the next anchor.
type readError struct{ err error }

func (failure *readError) Error() string { return "read the file: " + failure.err.Error() }

func (failure *readError) Unwrap() error { return failure.err }

// readAt reads exactly len(buffer) bytes at offset. Reading past the end of the file is a
// structural problem (errNotThisContainer), not a read error.
func readAt(file io.ReaderAt, size, offset int64, buffer []byte) error {
	if offset < 0 || offset+int64(len(buffer)) > size {
		return fmt.Errorf("%w: %d bytes at %d run past the end of a %d-byte file",
			errNotThisContainer, len(buffer), offset, size)
	}
	if _, err := file.ReadAt(buffer, offset); err != nil {
		return &readError{err: err}
	}
	return nil
}
