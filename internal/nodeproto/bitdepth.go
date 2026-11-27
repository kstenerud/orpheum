// SPDX-License-Identifier: AGPL-3.0-or-later

package nodeproto

import (
	"regexp"
	"strconv"
)

// pixelFormatDepth is the bit depth FFmpeg's pixel format names: "yuv420p10le" is 10.
var pixelFormatDepth = regexp.MustCompile(`p(\d{1,2})(?:le|be)?$`)

// BitDepth is the bit depth of the picture's pixel format: 8 for a format naming none, 0 for a
// picture whose format is not known, as a stream not a picture's.
func (video ProbeVideo) BitDepth() int {
	if video.PixelFormat == "" {
		return 0
	}
	if match := pixelFormatDepth.FindStringSubmatch(video.PixelFormat); match != nil {
		depth, err := strconv.Atoi(match[1])
		if err == nil {
			return depth
		}
	}
	return 8 //nolint:mnd // a pixel format naming no depth is FFmpeg's eight bits
}
