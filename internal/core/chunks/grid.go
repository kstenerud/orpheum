// SPDX-License-Identifier: AGPL-3.0-or-later

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

// GridKind is what a film's chunks are cut at (D269).
type GridKind string

// The kinds of grid.
const (
	// OnKeyframes is cut at its picture's keyframes, from its timeline: its picture copied can be
	// offered, as each chunk of it begins on a keyframe.
	OnKeyframes GridKind = "keyframes"
	// OnTime is cut at each multiple of the segment length: a film whose file has no index of its
	// keyframes, played encoded at once, without its picture copied (D269 §1).
	OnTime GridKind = "time"
)

// Grid is a film's chunk boundaries, in the film's own time (chunks.md §1): chunk n runs from
// StartsMs[n] to StartsMs[n+1], and the last from its start to FilmDurationMs.
type Grid struct {
	// StartsMs are each chunk's start, in milliseconds, ascending: keyframes of the picture, so a
	// copy of it can begin at each, or, on time, multiples of the segment length.
	StartsMs []int64
	// FilmDurationMs is the film's length, where the last chunk ends.
	FilmDurationMs int64
}

// GridOf is the starts of the chunks a picture with keyframes keyframesMs, ascending, is cut into,
// to the film's end at durationMs (D259 §1, chunks.md §1): the first keyframe starts the first
// chunk, and the first keyframe at or after each later multiple of segmentMs starts the next, so
// each start depends only on the keyframes near it and a stretch of the film cut alone agrees with
// the whole film's grid. A keyframe at or after the film's end starts nothing, and a last chunk
// shorter than segmentMs is merged into the one before it. It fails with ErrCannotBeCut for a
// picture with no keyframe before the film's end, and for a segmentMs that is not positive.
func GridOf(keyframesMs []int64, durationMs, segmentMs int64) ([]int64, error) {
	if segmentMs <= 0 {
		return nil, fmt.Errorf("%w: chunks of %d ms", ErrCannotBeCut, segmentMs)
	}
	if len(keyframesMs) == 0 || keyframesMs[0] >= durationMs {
		return nil, fmt.Errorf("%w: no keyframe before the film's end at %d ms", ErrCannotBeCut, durationMs)
	}
	nextMultipleAfter := func(ms int64) int64 { return (ms/segmentMs + 1) * segmentMs }
	starts := []int64{keyframesMs[0]}
	next := nextMultipleAfter(keyframesMs[0])
	for _, keyframe := range keyframesMs[1:] {
		if keyframe >= durationMs {
			break
		}
		if keyframe >= next {
			starts = append(starts, keyframe)
			next = nextMultipleAfter(keyframe)
		}
	}
	if len(starts) > 1 && durationMs-starts[len(starts)-1] < segmentMs {
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
	film, isCut, err := service.cut(ctx, fileID, OnKeyframes)
	return film.grid, isCut, err
}

// TimeGridOf is the starts of the chunks a film durationMs long is cut into on time (D269 §1): each
// multiple of segmentMs before its end, a last chunk shorter than segmentMs merged into the one
// before it. It fails with ErrCannotBeCut for a film of no known length, and for a segmentMs that is
// not positive.
func TimeGridOf(durationMs, segmentMs int64) ([]int64, error) {
	if segmentMs <= 0 || durationMs <= 0 {
		return nil, fmt.Errorf("%w: chunks of %d ms of a film of %d ms", ErrCannotBeCut, segmentMs, durationMs)
	}
	var multiples []int64
	for start := int64(0); start < durationMs; start += segmentMs {
		multiples = append(multiples, start)
	}
	return GridOf(multiples, durationMs, segmentMs)
}

// cutFilm is a film as it is cut into chunks: its file, its probe, its main picture, the grid and
// the picture's bytes in each second of the film, its bit-rate profile (slow-links.md §3).
type cutFilm struct {
	file              sqlcgen.GetPlayableFileRow
	probe             nodeproto.Probe
	picture           nodeproto.ProbeStream
	grid              Grid
	bytesInEachSecond []int64
	// keyframesMs are the picture's keyframes, ascending, as its timeline has them; none on time.
	keyframesMs []int64
	// kind is what its grid is cut at.
	kind GridKind
}

// cut is the file's film as it is cut into chunks of kind: on its picture's keyframes, as Grid says,
// reporting false while its picture has no timeline; or on time, which needs none (D269).
func (service *Service) cut(ctx context.Context, fileID int64, kind GridKind) (cutFilm, bool, error) {
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
	film := cutFilm{file: file, probe: probe, picture: picture, kind: kind}
	segment, err := service.settings.Duration(ctx, settings.HLSSegmentLengthSeconds)
	if err != nil {
		return film, false, fmt.Errorf("read hls_segment_length_seconds: %w", err)
	}
	if kind == OnTime {
		starts, timeErr := TimeGridOf(probe.Format.DurationMs, segment.Milliseconds())
		if timeErr != nil {
			return film, false, fmt.Errorf("file %d: %w", fileID, timeErr)
		}
		film.grid = Grid{StartsMs: starts, FilmDurationMs: probe.Format.DurationMs}
		return film, true, nil
	}
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
	if err = json.Unmarshal([]byte(timeline.KeyframesMsJSON), &film.keyframesMs); err != nil {
		return film, false, fmt.Errorf("the keyframes of file %d: %w", fileID, err)
	}
	if err = json.Unmarshal([]byte(timeline.BytesInEachSecondJSON), &film.bytesInEachSecond); err != nil {
		return film, false, fmt.Errorf("the bytes in each second of file %d: %w", fileID, err)
	}
	durationMs := probe.Format.DurationMs
	if durationMs <= 0 {
		durationMs = int64(len(film.bytesInEachSecond)) * time.Second.Milliseconds()
	}
	starts, err := GridOf(film.keyframesMs, durationMs, segment.Milliseconds())
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
