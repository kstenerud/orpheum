// SPDX-License-Identifier: AGPL-3.0-or-later

package artwork

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/execx"
)

// How a frame still is chosen from a video (D362 §1): FFmpeg's thumbnail filter picks the most
// representative of up to stillFrames pictures, read for at most stillStretch from where the still
// is taken, after every picture at least stillMostBlackPercent black is dropped, so a fade or a black
// lead-in is never the still.
const (
	// How long a stretch of the video is read.
	stillStretch = 20 * time.Second
	// How many pictures that are not black the thumbnail filter weighs.
	stillFrames = 100
	// How much of a picture may be black for it to be weighed.
	stillMostBlackPercent = 95
	// The luma at or under which a pixel counts as black: blackframe's own default.
	stillBlackLuma = 32
	// The most a still is on a side, so an 8K video's stays within the pixel limit its re-encode
	// refuses past; the shape's largest size is 3840 wide.
	stillLongestSide = 3840
)

// stillShortestTimeout is the least a still's run is given: a disk asleep spins up, and a 4K
// picture stream is decoded in software for the frames the thumbnail filter weighs.
const stillShortestTimeout = 2 * time.Minute

// maxStillBytes bounds the frame taken, a lossless PNG of up to 3840 pixels across.
const maxStillBytes = 64 << 20

// ErrNoFrame is a video whose stretch holds no picture that is not black: all black, or no
// picture at all. It is the video's answer, not a failure, and is not asked again (D362 §5).
var ErrNoFrame = errors.New("no frame in the stretch is not black")

// Still takes a frame still from film, the picture stream at streamIndex (D362 §1): the most
// representative frame of a stretch from at, black frames skipped, as a PNG of FFmpeg's own, at
// its pixels' shape, with none of the video's metadata, for Derive to re-encode as any image. The
// film is read as every run reads one (execx.Film, D330), and nothing else is read.
func (deriver *Deriver) Still(
	ctx context.Context, film execx.Film, streamIndex int, from time.Duration,
) ([]byte, error) {
	// Its position moves only when the still is written, at the end: the run is given the longer of
	// the re-encode's timeout and stillShortestTimeout to start, stand still and end.
	timeout := max(deriver.perRunTimeout, stillShortestTimeout)
	ran, err := deriver.runner.RunWatched(ctx, execx.WatchedRun{
		Access:           confine.Access{Read: film.Readable()},
		Arguments:        stillArguments(film.Input(), streamIndex, from),
		StdoutLimitBytes: maxStillBytes,
		Watch:            execx.Watch{StartGrace: timeout, StallWindow: timeout, Deadline: timeout},
		Film:             film.Stream,
	})
	if err != nil {
		return nil, fmt.Errorf("take a still: %w", err)
	}
	if len(ran.Stdout) == 0 {
		return nil, ErrNoFrame
	}
	return ran.Stdout, nil
}

// stillArguments are FFmpeg's for a still of the input's stream at streamIndex from a place: seek
// there, read the stretch, square the pixels, drop the black pictures, choose the most
// representative of what is left, bound its size, and write one PNG with no metadata.
func stillArguments(input []string, streamIndex int, from time.Duration) []string {
	filters := "scale='trunc(iw*sar/2)*2':ih,setsar=1," +
		"blackframe=amount=0:threshold=" + strconv.Itoa(stillBlackLuma) + "," +
		"metadata=mode=select:key=lavfi.blackframe.pblack:value=" + strconv.Itoa(stillMostBlackPercent) +
		":function=less," +
		"thumbnail=n=" + strconv.Itoa(stillFrames) + "," +
		fmt.Sprintf("scale='min(%d,iw)':'min(%d,ih)':force_original_aspect_ratio=decrease:flags=lanczos",
			stillLongestSide, stillLongestSide)
	// Both bound the input, before it: the read starts at the still's place and ends after the stretch,
	// so a video black throughout is read for the stretch alone.
	arguments := []string{"-v", logErrorsOnly, "-ss", seconds(from), "-t", seconds(stillStretch)}
	arguments = append(arguments, input...)
	return append(arguments,
		"-map", "0:"+strconv.Itoa(streamIndex), "-an", "-sn", "-dn",
		"-vf", filters, "-frames:v", "1", "-map_metadata", "-1", "-map_chapters", "-1",
		"-c:v", "png", "-f", "image2pipe", "pipe:1")
}

// seconds is a duration as FFmpeg's seconds, to the millisecond.
func seconds(duration time.Duration) string {
	return strconv.FormatFloat(duration.Seconds(), 'f', 3, 64)
}
