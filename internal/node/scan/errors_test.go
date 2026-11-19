// SPDX-License-Identifier: AGPL-3.0-or-later

package scan

import (
	"errors"
	"fmt"
	"io/fs"
	"syscall"
	"testing"

	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// A probe whose file the platform refused to open for permission is a permission failure, which the
// System page advises on ownership and mode for; one FFmpeg could not read is a probe's, which it
// does not (the first beta install, 2026-11-19, #19b).
func TestClassify_AProbeRefusedForPermissionIsAPermissionFailure(t *testing.T) {
	t.Parallel()
	refused := fmt.Errorf("%w: open Film/Film.mkv: %w", errProbe,
		&fs.PathError{Op: "open", Path: "Film/Film.mkv", Err: syscall.EACCES})
	unreadable := fmt.Errorf("%w: %w", errProbe, errors.New("Invalid data found when processing input"))
	for cause, want := range map[error]nodeproto.ErrorCode{
		refused:    nodeproto.CodePermissionDenied,
		unreadable: nodeproto.CodeProbeFailed,
	} {
		if got := classify(cause); got != want {
			t.Errorf("classify(%v) = %s, want %s", cause, got, want)
		}
	}
}
