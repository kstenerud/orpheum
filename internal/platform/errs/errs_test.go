// SPDX-License-Identifier: AGPL-3.0-or-later

package errs_test

import (
	"errors"
	"io/fs"
	"testing"

	"github.com/kstenerud/orpheum/internal/platform/errs"
)

func TestWrapIf_WrapsOnlyAFailure(t *testing.T) {
	t.Parallel()
	if err := errs.WrapIf("close the file", nil); err != nil {
		t.Errorf("success: %v", err)
	}
	err := errs.WrapIf("close the file", fs.ErrClosed)
	if !errors.Is(err, fs.ErrClosed) {
		t.Errorf("the failure is not kept: %v", err)
	}
	if got, want := err.Error(), "close the file: "+fs.ErrClosed.Error(); got != want {
		t.Errorf("message %q, want %q", got, want)
	}
}
