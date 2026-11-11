// SPDX-License-Identifier: AGPL-3.0-or-later

package chunks

import (
	"context"
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
	"github.com/kstenerud/orpheum/pkg/strictjson"
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
	// OnKeyframesSized is cut on its picture's keyframes as OnKeyframes is, and at more of them
	// where a chunk would hold more than largestChunkBytes of the picture, by its bytes in each
	// second (SizedGridOf): a film whose read-through has found them.
	OnKeyframesSized GridKind = "keyframes-sized"
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
// the whole film's grid. So a chunk may be shorter than segmentMs, never merged into another: one
// started by a keyframe just before a multiple ends at the next keyframe past it. A keyframe at or
// after the film's end starts nothing, and a last chunk shorter than segmentMs is merged into the one
// before it. It fails with ErrCannotBeCut for a
// picture with no keyframe before the film's end, and for a segmentMs that is not positive.
func GridOf(keyframesMs []int64, durationMs, segmentMs int64) ([]int64, error) {
	return gridOf(keyframesMs, durationMs, segmentMs, nil)
}

// largestChunkBytes is the most of a picture's bytes SizedGridOf puts in a chunk where its keyframes
// allow: a sixth of the least a browser holds of a picture, Chromium's 30 MiB on a device of 1 GB, so
// that the chunk playing, two ahead and the room behind them fit. A 1080p film's 10 s chunk of 21 MB
// was refused there, and its playback stood still (the owner's Fire TV, 2026-11-11). Every device
// plays the one grid; a device with more room holds more chunks ahead (the owner, 2026-11-11).
const largestChunkBytes = 5 << 20

// SizedGridOf is GridOf's grid with a chunk ended early, at the keyframe before the one that would
// take it past largestChunkBytes of the picture, by its bytesInEachSecond (entry n is second n's),
// so a stretch of a high rate is played at its full quality in chunks a small buffer holds (the
// owner, 2026-11-11). A chunk begun at a multiple of segmentMs still begins there, so each start
// depends only on the keyframes and the bytes near it, and a stretch of the film cut alone agrees with
// the whole film's grid. The stretch from one keyframe to the next is never split: one larger than
// largestChunkBytes is a chunk of its own. A last chunk is merged into the one before it only where
// the two stay within largestChunkBytes. With no bytes it is GridOf's grid.
func SizedGridOf(keyframesMs, bytesInEachSecond []int64, durationMs, segmentMs int64) ([]int64, error) {
	if len(bytesInEachSecond) == 0 {
		return GridOf(keyframesMs, durationMs, segmentMs)
	}
	return gridOf(keyframesMs, durationMs, segmentMs, bytesBeforeOf(bytesInEachSecond))
}

// gridOf is GridOf's grid, and SizedGridOf's where bytesBefore, the picture's bytes before each time
// of the film, is given.
func gridOf(keyframesMs []int64, durationMs, segmentMs int64, bytesBefore func(ms int64) int64) ([]int64, error) {
	if segmentMs <= 0 {
		return nil, fmt.Errorf("%w: chunks of %d ms", ErrCannotBeCut, segmentMs)
	}
	if len(keyframesMs) == 0 || keyframesMs[0] >= durationMs {
		return nil, fmt.Errorf("%w: no keyframe before the film's end at %d ms", ErrCannotBeCut, durationMs)
	}
	nextMultipleAfter := func(ms int64) int64 { return (ms/segmentMs + 1) * segmentMs }
	// isTooLarge is whether a chunk from its start to toMs would hold more than largestChunkBytes.
	isTooLarge := func(fromMs, toMs int64) bool {
		return bytesBefore != nil && bytesBefore(toMs)-bytesBefore(fromMs) > largestChunkBytes
	}
	starts := []int64{keyframesMs[0]}
	next := nextMultipleAfter(keyframesMs[0])
	// before is the keyframe before the one at hand: where a chunk grown too large ends.
	before := keyframesMs[0]
	for _, keyframe := range keyframesMs[1:] {
		if keyframe >= durationMs {
			break
		}
		if start := starts[len(starts)-1]; before != start && isTooLarge(start, keyframe) {
			starts = append(starts, before)
		}
		if keyframe >= next {
			starts = append(starts, keyframe)
			next = nextMultipleAfter(keyframe)
		}
		before = keyframe
	}
	if start := starts[len(starts)-1]; before != start && isTooLarge(start, durationMs) {
		starts = append(starts, before)
	}
	if len(starts) > 1 && durationMs-starts[len(starts)-1] < segmentMs &&
		!isTooLarge(starts[len(starts)-2], durationMs) {
		starts = starts[:len(starts)-1]
	}
	return starts, nil
}

// bytesBeforeOf is the bytes before each time of a film whose picture has bytesInEachSecond, each
// second's spread evenly over it.
func bytesBeforeOf(bytesInEachSecond []int64) func(ms int64) int64 {
	before := make([]int64, len(bytesInEachSecond)+1)
	for second, bytes := range bytesInEachSecond {
		before[second+1] = before[second] + bytes
	}
	return func(ms int64) int64 {
		ms = max(ms, 0)
		second := ms / 1000
		if second >= int64(len(bytesInEachSecond)) {
			return before[len(bytesInEachSecond)]
		}
		return before[second] + bytesInEachSecond[second]*(ms%1000)/1000
	}
}

// Grid is the grid the file's film is cut on, from its main picture's timeline as a read-through
// kept it, its keyframes merged into chunks by GridOf at hlsSegmentLengthSeconds: a chunk may be
// shorter than that, down to the gap between two keyframes, where the first keyframe past one
// multiple falls just before the next, as each start is its multiple's own (D259 §1). It reports
// false, with no error, while the picture has no timeline: a sweep reads every film through, and a
// player that will not wait for it asks readthrough.Service.ReadForPlay. A film with no picture, or whose
// picture cannot be cut, fails with ErrCannotBeCut. The film's length is its probe's, or, when the
// probe does not say, the timeline's seconds.
func (service *Service) Grid(ctx context.Context, fileID int64) (Grid, bool, error) {
	film, isCut, err := service.cut(ctx, fileID, OnKeyframes)
	return film.grid, isCut, err
}

// KeyframeAtOrBefore is where the film's picture's last keyframe at or before positionMs is: the
// start of its grid's chunk holding it, false while the film is not cut or cannot be (D285 §1).
func (service *Service) KeyframeAtOrBefore(ctx context.Context, fileID, positionMs int64) (int64, bool) {
	grid, isCut, err := service.Grid(ctx, fileID)
	if err != nil || !isCut || len(grid.StartsMs) == 0 {
		return 0, false
	}
	return grid.StartsMs[grid.chunkAt(positionMs)], true
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
		return film, false, fmt.Errorf("read hlsSegmentLengthSeconds: %w", err)
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
	if err = strictjson.Unmarshal([]byte(timeline.KeyframesMsJSON), &film.keyframesMs); err != nil {
		return film, false, fmt.Errorf("the keyframes of file %d: %w", fileID, err)
	}
	if err = strictjson.Unmarshal([]byte(timeline.BytesInEachSecondJSON), &film.bytesInEachSecond); err != nil {
		return film, false, fmt.Errorf("the bytes in each second of file %d: %w", fileID, err)
	}
	durationMs := probe.Format.DurationMs
	if durationMs <= 0 {
		durationMs = int64(len(film.bytesInEachSecond)) * time.Second.Milliseconds()
	}
	if film, err = cutOnKeyframes(film, durationMs, segment.Milliseconds()); err != nil {
		return film, false, fmt.Errorf("file %d: %w", fileID, err)
	}
	return film, true, nil
}

// cutOnKeyframes is the film with its grid from its keyframes, sized by its bytes in each second for
// OnKeyframesSized; a film whose bytes are not read yet is cut on its keyframes alone, as a
// playback begun on it then stays.
func cutOnKeyframes(film cutFilm, durationMs, segmentMs int64) (cutFilm, error) {
	if film.kind == OnKeyframesSized && len(film.bytesInEachSecond) == 0 {
		film.kind = OnKeyframes
	}
	var bytesInEachSecond []int64
	if film.kind == OnKeyframesSized {
		bytesInEachSecond = film.bytesInEachSecond
	}
	starts, err := SizedGridOf(film.keyframesMs, bytesInEachSecond, durationMs, segmentMs)
	if err != nil {
		return film, err
	}
	film.grid = Grid{StartsMs: starts, FilmDurationMs: durationMs}
	return film, nil
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
