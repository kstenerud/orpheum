// SPDX-License-Identifier: AGPL-3.0-or-later

package testutil

import (
	"os/exec"
	"path/filepath"
	"testing"
)

// GenerateImage makes a test pattern image with the pinned FFmpeg: dimensions as "WxH", in an FFmpeg
// codec ("libwebp", "mjpeg", "png") and muxer ("webp", "mjpeg", "image2pipe"). It is for tests of
// packages that may not run a process themselves, such as the core's (testing.md §7, tier 2).
func GenerateImage(tb testing.TB, dimensions, codec, format string) []byte {
	tb.Helper()
	directory := FFmpegDirectory(tb)
	ffmpeg := filepath.Join(directory, "ffmpeg")
	command := exec.CommandContext(tb.Context(), ffmpeg, //nolint:gosec // the pinned tool, argv as a slice
		"-v", "error", "-f", "lavfi", "-i", "testsrc2=size="+dimensions+":duration=1", "-frames:v", "1",
		"-c:v", codec, "-f", format, "pipe:1")
	command.Dir = "/"
	image, err := command.Output()
	if err != nil {
		tb.Fatalf("generate a %s %s image: %v", dimensions, codec, err)
	}
	return image
}
