// SPDX-License-Identifier: GPL-3.0-or-later

package bounded

import (
	"errors"
	"fmt"
	"io"
)

// ErrTooLarge is a body past the bound it was read under.
var ErrTooLarge = errors.New("larger than the bound")

// ReadAll is everything reader holds, when that is at most maxBytes; past it, ErrTooLarge.
func ReadAll(reader io.Reader, maxBytes int64) ([]byte, error) {
	contents, err := io.ReadAll(io.LimitReader(reader, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	if int64(len(contents)) > maxBytes {
		return nil, fmt.Errorf("%w of %d bytes", ErrTooLarge, maxBytes)
	}
	return contents, nil
}
