// SPDX-License-Identifier: AGPL-3.0-or-later

package testutil

import (
	"testing"
)

// GenerateImage makes a test pattern image with the pinned FFmpeg: dimensions as "WxH", in an FFmpeg
// codec ("libwebp", "mjpeg", "png") and muxer ("webp", "mjpeg", "image2pipe"), written to its stdout as
// a run that hands an image on writes it. The image is made once and kept in the generated-media cache
// (mediaCache). It is for tests that need an image FFmpeg makes, including tests of packages that may
// not run a process themselves, such as the core's (testing.md §7, tier 2).
func GenerateImage(tb testing.TB, dimensions, codec, format string) []byte {
	tb.Helper()
	image, err := pinnedMediaCache(tb).bytesOf(tb.Context(), []string{
		"-f", "lavfi", "-i", "testsrc2=size=" + dimensions + ":duration=1", "-frames:v", "1",
		"-c:v", codec, "-f", format,
	})
	if err != nil {
		tb.Fatalf("generate a %s %s image: %v", dimensions, codec, err)
	}
	return image
}
