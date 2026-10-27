// SPDX-License-Identifier: AGPL-3.0-or-later

package testutil

import (
	"os/exec"
	"path/filepath"
	"testing"
)

// GenerateFilm writes a film at path with the pinned FFmpeg, from arguments: its inputs, its codecs
// and its muxer, as FFmpeg's command line names them, the path last. It is for tests of packages that
// may not run a process themselves, such as cmd/orpheum's (testing.md §7, tier 2).
func GenerateFilm(tb testing.TB, path string, arguments ...string) {
	tb.Helper()
	ffmpeg := filepath.Join(FFmpegDirectory(tb), "ffmpeg")
	command := exec.CommandContext(tb.Context(), ffmpeg, //nolint:gosec // the pinned tool, argv as a slice
		append(append([]string{"-v", "error"}, arguments...), path)...)
	command.Dir = "/"
	if output, err := command.CombinedOutput(); err != nil {
		tb.Fatalf("generate %s: %v: %s", path, err, output)
	}
}
