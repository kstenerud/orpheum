// SPDX-License-Identifier: AGPL-3.0-or-later

package bounded_test

import (
	"errors"
	"io"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/kstenerud/orpheum/internal/platform/bounded"
)

// countingReader counts the bytes read from it.
type countingReader struct {
	reader io.Reader
	read   int
}

func (counting *countingReader) Read(buffer []byte) (int, error) {
	count, err := counting.reader.Read(buffer)
	counting.read += count
	return count, err
}

func TestReadAll_RefusesPastTheBoundRatherThanCutIt(t *testing.T) {
	t.Parallel()
	contents, err := bounded.ReadAll(strings.NewReader("12345"), 5)
	if err != nil || string(contents) != "12345" {
		t.Errorf("at the bound: %q, %v", contents, err)
	}
	large := &countingReader{reader: strings.NewReader(strings.Repeat("x", 1<<20))}
	contents, err = bounded.ReadAll(large, 5)
	if !errors.Is(err, bounded.ErrTooLarge) || contents != nil {
		t.Errorf("past the bound: %q, %v; want nothing and ErrTooLarge", contents, err)
	}
	if large.read > 6 {
		t.Errorf("read %d bytes of a body bounded at 5; want at most 6", large.read)
	}
	failing := io.MultiReader(strings.NewReader("12"), iotest.ErrReader(errBroken))
	if _, err := bounded.ReadAll(failing, 5); !errors.Is(err, errBroken) {
		t.Errorf("a failing reader: %v, want its error", err)
	}
}

var errBroken = errors.New("broken")

func TestGunzip_GivesBackWhatGzipCompressedAndRefusesPastTheBound(t *testing.T) {
	t.Parallel()
	compressed, err := bounded.Gzip([]byte(strings.Repeat("x", 1000)))
	if err != nil {
		t.Fatal(err)
	}
	body, err := bounded.Gunzip(compressed, 1000)
	if err != nil || string(body) != strings.Repeat("x", 1000) {
		t.Errorf("at the bound: %d bytes, %v; want the 1000 compressed", len(body), err)
	}
	if body, err = bounded.Gunzip(compressed, 999); !errors.Is(err, bounded.ErrTooLarge) || body != nil {
		t.Errorf("past the bound: %d bytes, %v; want nothing and ErrTooLarge", len(body), err)
	}
}
