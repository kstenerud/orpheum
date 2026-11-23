// SPDX-License-Identifier: AGPL-3.0-or-later

package artwork

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/kstenerud/orpheum/internal/node/execx"
)

// How a key frame is made, the picture a cast's remote shows where the video would be (D545): one
// picture, the keyframe at or before the place, so a single frame is decoded, at FrameWidth across.
const (
	// FrameWidth is a key frame's width; its height keeps the picture's shape, even.
	FrameWidth = 640
	// The WebP quality of a key frame: libwebp's own default, for a picture a few inches wide.
	frameWebPQuality = 75
	// The most bytes a key frame may be: a 640-wide WebP is tens of kilobytes.
	maxFrameBytes = 4 << 20
)

// ErrNoKeyFrame is a video with no picture at or before the place asked: past its end, or with no
// picture there at all. It is the video's answer, not a failure.
var ErrNoKeyFrame = errors.New("no picture at or before the place")

// KeyFrame takes one picture of film, the picture stream at streamIndex, at the keyframe at or before
// at (D545): only keyframes are decoded, from where the input's seek lands, and the first is the one
// written, FrameWidth across at its pixels' shape, as our own WebP with none of the video's metadata.
// The film is read as a still's is (runOnFilm), and nothing else is read. Nothing keeps it.
func (deriver *Deriver) KeyFrame(
	ctx context.Context, film execx.Film, streamIndex int, at time.Duration,
) ([]byte, error) {
	frame, err := deriver.runOnFilm(ctx, film, keyFrameArguments(film.Input(), streamIndex, at), maxFrameBytes,
		deriver.perRunTimeout)
	if err != nil {
		return nil, fmt.Errorf("take a key frame: %w", err)
	}
	if len(frame) == 0 {
		return nil, ErrNoKeyFrame
	}
	return frame, nil
}

// keyFrameArguments are FFmpeg's for one picture of the input's stream at streamIndex at the keyframe
// at or before a place: seek the input there, landing on the keyframe before it, decode keyframes
// alone and keep the first, not the one after the place an accurate seek would decode on to; square
// the pixels, scale it FrameWidth across, and write one WebP with no metadata.
func keyFrameArguments(input []string, streamIndex int, at time.Duration) []string {
	filters := squarePixels + "," + fmt.Sprintf("scale=%d:-2:flags=lanczos", FrameWidth)
	// All three are the input's, before it: the seek, and what its decoder decodes from there.
	arguments := []string{"-v", logErrorsOnly, "-skip_frame", "nokey", "-noaccurate_seek", "-ss", seconds(at)}
	arguments = append(arguments, input...)
	// The keyframe kept is before the place, so its time is before the output's start: passed through,
	// it is written, where a constant rate would drop it and write the next one, after the place.
	return append(arguments,
		"-map", "0:"+strconv.Itoa(streamIndex), "-an", "-sn", "-dn",
		"-vf", filters, "-fps_mode", "passthrough", "-frames:v", "1", "-map_metadata", "-1", "-map_chapters", "-1",
		"-c:v", "libwebp", "-lossless", "0", "-quality", strconv.Itoa(frameWebPQuality), "-f", "webp", "pipe:1")
}
