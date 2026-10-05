// SPDX-License-Identifier: GPL-3.0-or-later

package chunks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/kstenerud/orpheum/internal/core/decide"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// ErrCannotBeCut is a film whose picture has no grid and never will, as its bytes are: FFmpeg could
// not read it through, or it has no keyframe a chunk could begin at. A film with no timeline yet is
// not this: Grid says it has none, and a read-through makes one.
var ErrCannotBeCut = errors.New("the film's picture cannot be cut into chunks")

// Grid is a film's chunk boundaries, in the film's own time (chunks.md §1): chunk n runs from
// StartsMs[n] to StartsMs[n+1], and the last from its start to FilmDurationMs.
type Grid struct {
	// StartsMs are each chunk's start, in milliseconds, ascending: keyframes of the picture, so a
	// copy of it can begin at each.
	StartsMs []int64
	// FilmDurationMs is the film's length, where the last chunk ends.
	FilmDurationMs int64
}

// GridOf merges a picture's keyframes, keyframesMs, ascending, into the starts of chunks each at
// least minChunkMs long, to the film's end at durationMs (D247 §10, chunks.md §1). The first chunk
// starts at the first keyframe; each keyframe that is minChunkMs or more after the last start
// starts the next, greedily; and a last chunk shorter than minChunkMs is merged into the one before
// it, so only a film shorter than minChunkMs from its first keyframe has a chunk shorter than that.
// A keyframe at or after the film's end starts nothing. It fails with ErrCannotBeCut for a picture
// with no keyframe before the film's end, and for a minChunkMs that is not positive.
func GridOf(keyframesMs []int64, durationMs, minChunkMs int64) ([]int64, error) {
	if minChunkMs <= 0 {
		return nil, fmt.Errorf("%w: a chunk of at least %d ms", ErrCannotBeCut, minChunkMs)
	}
	if len(keyframesMs) == 0 || keyframesMs[0] >= durationMs {
		return nil, fmt.Errorf("%w: no keyframe before the film's end at %d ms", ErrCannotBeCut, durationMs)
	}
	starts := []int64{keyframesMs[0]}
	for _, keyframe := range keyframesMs[1:] {
		if keyframe >= durationMs {
			break
		}
		if keyframe-starts[len(starts)-1] >= minChunkMs {
			starts = append(starts, keyframe)
		}
	}
	if len(starts) > 1 && durationMs-starts[len(starts)-1] < minChunkMs {
		starts = starts[:len(starts)-1]
	}
	return starts, nil
}

// Grid is the grid the file's film is cut on, from its main picture's timeline as a read-through
// kept it, merged so each chunk is at least hls_segment_length_seconds long. It reports false, with
// no error, while the picture has no timeline: a sweep reads every film through, and a player that
// will not wait for it asks readthrough.Service.ReadForPlay. A film with no picture, or whose
// picture cannot be cut, fails with ErrCannotBeCut. The film's length is its probe's, or, when the
// probe does not say, the timeline's seconds.
func (service *Service) Grid(ctx context.Context, fileID int64) (Grid, bool, error) {
	film, isCut, err := service.cut(ctx, fileID)
	return film.grid, isCut, err
}

// cutFilm is a film as it is cut into chunks: its file, its probe, its main picture, the grid and
// the picture's bytes in each second of the film, its bit-rate profile (slow-links.md §3).
type cutFilm struct {
	file              sqlcgen.GetPlayableFileRow
	probe             nodeproto.Probe
	picture           nodeproto.ProbeStream
	grid              Grid
	bytesInEachSecond []int64
}

// cut is the file's film as it is cut into chunks, as Grid says.
func (service *Service) cut(ctx context.Context, fileID int64) (cutFilm, bool, error) {
	file, err := library.PlayableFile(ctx, service.database.Read(), fileID)
	if err != nil {
		return cutFilm{}, false, fmt.Errorf("file %d: %w", fileID, err)
	}
	probe, err := library.ProbeOf(file.ProbeJSON)
	if err != nil {
		return cutFilm{}, false, fmt.Errorf("the probe of file %d: %w", fileID, err)
	}
	picture, hasPicture := decide.MainVideo(probe)
	if !hasPicture {
		return cutFilm{}, false, fmt.Errorf("%w: file %d has no picture", ErrCannotBeCut, fileID)
	}
	film := cutFilm{file: file, probe: probe, picture: picture}
	timeline, err := store.OrNotFound(service.database.Read().GetVideoTimeline(ctx, sqlcgen.GetVideoTimelineParams{
		PayloadHash: file.PayloadHash, VideoStreamIndex: int64(picture.StreamIndex),
	}))
	if errors.Is(err, store.ErrNotFound) {
		return film, false, nil
	}
	if err != nil {
		return film, false, fmt.Errorf("read the timeline of file %d: %w", fileID, err)
	}
	if timeline.Failure != nil {
		return film, false, fmt.Errorf("%w: FFmpeg could not read file %d through: %s", ErrCannotBeCut, fileID,
			*timeline.Failure)
	}
	var keyframesMs []int64
	if err = json.Unmarshal([]byte(timeline.KeyframesMsJSON), &keyframesMs); err != nil {
		return film, false, fmt.Errorf("the keyframes of file %d: %w", fileID, err)
	}
	if err = json.Unmarshal([]byte(timeline.BytesInEachSecondJSON), &film.bytesInEachSecond); err != nil {
		return film, false, fmt.Errorf("the bytes in each second of file %d: %w", fileID, err)
	}
	minChunk, err := service.settings.Duration(ctx, settings.HLSSegmentLengthSeconds)
	if err != nil {
		return film, false, fmt.Errorf("read hls_segment_length_seconds: %w", err)
	}
	durationMs := probe.Format.DurationMs
	if durationMs <= 0 {
		durationMs = int64(len(film.bytesInEachSecond)) * time.Second.Milliseconds()
	}
	starts, err := GridOf(keyframesMs, durationMs, minChunk.Milliseconds())
	if err != nil {
		return film, false, fmt.Errorf("file %d: %w", fileID, err)
	}
	film.grid = Grid{StartsMs: starts, FilmDurationMs: durationMs}
	return film, true, nil
}

// lastChunk is the number of the grid's last chunk.
func (grid Grid) lastChunk() int { return len(grid.StartsMs) - 1 }

// endMs is where chunk n ends in the film: the next chunk's start, or the film's end for the last.
func (grid Grid) endMs(n int) int64 {
	if n+1 < len(grid.StartsMs) {
		return grid.StartsMs[n+1]
	}
	return grid.FilmDurationMs
}

// chunkAt is the chunk the film's time positionMs falls in: the last that starts at or before it,
// or the first for a time before the first starts.
func (grid Grid) chunkAt(positionMs int64) int {
	n, isExact := slices.BinarySearch(grid.StartsMs, positionMs)
	if isExact || n == 0 {
		return n
	}
	return n - 1
}
