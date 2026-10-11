// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build integration

package execx

import (
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/testutil"
)

func TestRun_ThePinnedFFprobeRunsWithAnEmptyEnvironment(t *testing.T) {
	t.Parallel()
	runner, err := NewRunner(testutil.FFmpegDirectory(t))
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := runner.Run(t.Context(), FFprobe, confine.Access{}, []string{"-version"}, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(stdout), "ffprobe version ") {
		t.Errorf("ffprobe -version said %q", stdout)
	}
}
