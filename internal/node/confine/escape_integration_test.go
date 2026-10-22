// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build integration && linux

package confine_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// The pinned FFmpeg, confined by the real shim with the whole filter, encodes with several threads
// and probes: threads are clones with CLONE_THREAD, and glibc reaches them through clone3's ENOSYS.
// An EPERM there failed FFmpeg's threads, "Error while opening decoder: Operation not permitted"
// (ffstreamer's third audit, M3).
func TestEscape_ThePinnedFFmpegEncodesWithThreadsWhenConfined(t *testing.T) {
	t.Parallel()
	requireMechanisms(t)
	directory := testutil.FFmpegDirectory(t)
	for _, run := range []struct {
		tool      string
		arguments []string
	}{
		{"ffmpeg", []string{
			"-nostdin", "-hide_banner", "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=640x360:rate=25:duration=2",
			"-c:v", "libx264", "-threads", "4", "-f", "null", "-",
		}},
		{"ffprobe", []string{
			"-hide_banner", "-v", "error", "-f", "lavfi", "-i", "sine=duration=1", "-show_entries", "stream=codec_type",
		}},
	} {
		tool, err := filepath.EvalSymlinks(filepath.Join(directory, run.tool))
		if err != nil {
			t.Fatal(err)
		}
		spec, err := confine.EncodeSpec(confine.Spec{Tool: tool, Arguments: run.arguments})
		if err != nil {
			t.Fatal(err)
		}
		output, err := testutil.RunSelf(t, helperArguments(actAsShim, spec)...)
		if err != nil || strings.Contains(string(output), "not permitted") {
			t.Errorf("%s, confined: %v; it printed %q", run.tool, err, output)
		}
	}
}
