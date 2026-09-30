// SPDX-License-Identifier: GPL-3.0-or-later

package readonly_test

import (
	"io/fs"
	"testing"

	"github.com/kstenerud/orpheum/internal/platform/readonly"
)

// failingCloser counts its closes and fails each one.
type failingCloser struct{ closes int }

func (closer *failingCloser) Close() error {
	closer.closes++
	return fs.ErrClosed
}

func TestClose_ClosesAndLetsTheErrorGo(t *testing.T) {
	t.Parallel()
	closer := &failingCloser{}
	readonly.Close(closer)
	if closer.closes != 1 {
		t.Errorf("closed %d times, want once", closer.closes)
	}
}
