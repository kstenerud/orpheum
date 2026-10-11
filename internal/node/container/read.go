// SPDX-License-Identifier: AGPL-3.0-or-later

package container

import (
	"errors"
	"fmt"
	"io"
)

// ErrNotThisContainer is a file a parser does not follow: not its container, or not one it can
// follow; the caller tries another, or reads the file another way.
var ErrNotThisContainer = errors.New("not this container")

// MaxElementHops bounds how many elements or boxes a walk steps over.
const MaxElementHops = 1 << 20

// ReadError is a failure to read the file, as opposed to a file the parser does not follow
// (ErrNotThisContainer).
type ReadError struct{ err error }

func (failure *ReadError) Error() string { return "read the file: " + failure.err.Error() }

func (failure *ReadError) Unwrap() error { return failure.err }

// ReadAt reads exactly len(buffer) bytes at offset of a file of sizeBytes. Reading past the end of
// the file is a structural problem (ErrNotThisContainer), not a read error.
func ReadAt(file io.ReaderAt, sizeBytes, offset int64, buffer []byte) error {
	if offset < 0 || offset+int64(len(buffer)) > sizeBytes {
		return fmt.Errorf("%w: %d bytes at %d run past the end of a %d-byte file",
			ErrNotThisContainer, len(buffer), offset, sizeBytes)
	}
	if _, err := file.ReadAt(buffer, offset); err != nil {
		return &ReadError{err: err}
	}
	return nil
}
