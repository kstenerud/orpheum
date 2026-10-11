// SPDX-License-Identifier: AGPL-3.0-or-later

package bounded

import (
	"bytes"
	"compress/gzip"
	"fmt"
)

// Gzip is body compressed, as a body another party sent is kept: a provider's answer, a fetched
// subtitle.
func Gzip(body []byte) ([]byte, error) {
	var buffer bytes.Buffer
	writer := gzip.NewWriter(&buffer)
	if _, err := writer.Write(body); err != nil {
		return nil, fmt.Errorf("compress: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("compress: %w", err)
	}
	return buffer.Bytes(), nil
}

// Gunzip is what Gzip compressed, when it is at most maxBytes; past it, ErrTooLarge, so a small
// stored body cannot expand without limit.
func Gunzip(compressed []byte, maxBytes int64) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, fmt.Errorf("decompress: %w", err)
	}
	body, err := ReadAll(reader, maxBytes)
	if err != nil {
		return nil, fmt.Errorf("decompress: %w", err)
	}
	return body, nil
}
