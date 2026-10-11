// SPDX-License-Identifier: AGPL-3.0-or-later

package randbytes_test

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/kstenerud/orpheum/internal/platform/randbytes"
)

func TestRead_RefusesAShortRead(t *testing.T) {
	t.Parallel()
	raw, err := randbytes.Read(bytes.NewReader([]byte{1, 2, 3}), 3)
	if err != nil || !bytes.Equal(raw, []byte{1, 2, 3}) {
		t.Errorf("enough bytes: %v, %v", raw, err)
	}
	_, err = randbytes.Read(bytes.NewReader([]byte{1, 2}), 3)
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Errorf("too few bytes: %v, want io.ErrUnexpectedEOF", err)
	}
}

func TestBase64URLSecret_IsTheBytesInUnpaddedBase64URL(t *testing.T) {
	t.Parallel()
	secret, err := randbytes.Base64URLSecret(bytes.NewReader([]byte{0xfb, 0xff, 0xfe, 0x01}), 4)
	if err != nil {
		t.Fatal(err)
	}
	if secret != "-__-AQ" {
		t.Errorf("secret %q, want %q", secret, "-__-AQ")
	}
}
