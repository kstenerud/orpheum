// SPDX-License-Identifier: GPL-3.0-or-later

package chunks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/kstenerud/orpheum/internal/core/decide"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/settings"
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

// Settings are the settings the grid reads.
type Settings interface {
	Duration(ctx context.Context, key settings.Key) (time.Duration, error)
}

// Parts are what the chunks service is built from.
type Parts struct {
	// Database holds the files and their pictures' timelines, which internal/core/readthrough keeps.
	Database *store.Database
	Settings Settings
}

// Service cuts films into chunks made on request (D247 §10): for now, the grid each is cut on.
type Service struct {
	database *store.Database
	settings Settings
}

// NewService returns the chunks service.
func NewService(parts Parts) *Service {
	return &Service{database: parts.Database, settings: parts.Settings}
}

// Grid is the grid the file's film is cut on, from its main picture's timeline as a read-through
// kept it, merged so each chunk is at least hls_segment_length_seconds long. It reports false, with
// no error, while the picture has no timeline: a sweep reads every film through, and a player that
// will not wait for it asks readthrough.Service.ReadForPlay. A film with no picture, or whose
// picture cannot be cut, fails with ErrCannotBeCut. The film's length is its probe's, or, when the
// probe does not say, the timeline's seconds.
func (service *Service) Grid(ctx context.Context, fileID int64) (Grid, bool, error) {
	file, err := library.PlayableFile(ctx, service.database.Read(), fileID)
	if err != nil {
		return Grid{}, false, fmt.Errorf("file %d: %w", fileID, err)
	}
	probe, err := library.ProbeOf(file.ProbeJSON)
	if err != nil {
		return Grid{}, false, fmt.Errorf("the probe of file %d: %w", fileID, err)
	}
	picture, hasPicture := decide.MainVideo(probe)
	if !hasPicture {
		return Grid{}, false, fmt.Errorf("%w: file %d has no picture", ErrCannotBeCut, fileID)
	}
	timeline, err := store.OrNotFound(service.database.Read().GetVideoTimeline(ctx, sqlcgen.GetVideoTimelineParams{
		PayloadHash: file.PayloadHash, VideoStreamIndex: int64(picture.StreamIndex),
	}))
	if errors.Is(err, store.ErrNotFound) {
		return Grid{}, false, nil
	}
	if err != nil {
		return Grid{}, false, fmt.Errorf("read the timeline of file %d: %w", fileID, err)
	}
	if timeline.Failure != nil {
		return Grid{}, false, fmt.Errorf("%w: FFmpeg could not read file %d through: %s", ErrCannotBeCut, fileID,
			*timeline.Failure)
	}
	var keyframesMs, bytesInEachSecond []int64
	if err = json.Unmarshal([]byte(timeline.KeyframesMsJSON), &keyframesMs); err != nil {
		return Grid{}, false, fmt.Errorf("the keyframes of file %d: %w", fileID, err)
	}
	if err = json.Unmarshal([]byte(timeline.BytesInEachSecondJSON), &bytesInEachSecond); err != nil {
		return Grid{}, false, fmt.Errorf("the bytes in each second of file %d: %w", fileID, err)
	}
	minChunk, err := service.settings.Duration(ctx, settings.HLSSegmentLengthSeconds)
	if err != nil {
		return Grid{}, false, fmt.Errorf("read hls_segment_length_seconds: %w", err)
	}
	durationMs := probe.Format.DurationMs
	if durationMs <= 0 {
		durationMs = int64(len(bytesInEachSecond)) * time.Second.Milliseconds()
	}
	starts, err := GridOf(keyframesMs, durationMs, minChunk.Milliseconds())
	if err != nil {
		return Grid{}, false, fmt.Errorf("file %d: %w", fileID, err)
	}
	return Grid{StartsMs: starts, FilmDurationMs: durationMs}, true, nil
}
