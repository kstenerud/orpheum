// SPDX-License-Identifier: AGPL-3.0-or-later

package readthrough

import (
	"context"
	"fmt"

	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// ForgetLoudness throws out what was measured of one audio track of a file's bytes, once the facts of
// its probe the measurement was made from changed (library.DerivedDroppers, D392 §3), in the
// ingest's write that keeps the new probe: the sweep finds the bytes lacking it, and measures it
// again. It says whether there was any.
func ForgetLoudness(ctx context.Context, queries *sqlcgen.Queries, of library.DerivedOf) (bool, error) {
	deleted, err := queries.DeleteLoudnessOfSound(ctx, sqlcgen.DeleteLoudnessOfSoundParams{
		PayloadHash: of.PayloadHash, StreamIndex: int64(of.StreamIndex),
	})
	if err != nil {
		return false, fmt.Errorf("forget the loudness of file %d's stream %d: %w", of.FileID, of.StreamIndex, err)
	}
	return deleted > 0, nil
}

// ForgetFailuresOfFiles forgets, in a rescan's write (identify.GivenUpForgetters, D535 §8), the
// measurements and timelines that could not be made of the bytes of the files named, a JSON array of
// ids: the sweep makes each again. A timeline whose read through alone failed keeps its keyframes, so
// the grid a playback is cut on does not move. What was made stays. It says how many it forgot.
func ForgetFailuresOfFiles(ctx context.Context, queries *sqlcgen.Queries, fileIDsJSON string) (int64, error) {
	loudness, err := queries.ForgetLoudnessFailuresOfFiles(ctx, fileIDsJSON)
	if err != nil {
		return 0, fmt.Errorf("forget the failed loudness measurements: %w", err)
	}
	timelines, err := queries.ForgetTimelineFailuresOfFiles(ctx, fileIDsJSON)
	if err != nil {
		return 0, fmt.Errorf("forget the timelines that could not be read: %w", err)
	}
	reads, err := queries.ForgetTimelineReadFailuresOfFiles(ctx, fileIDsJSON)
	if err != nil {
		return 0, fmt.Errorf("forget the timelines' failed reads through: %w", err)
	}
	return loudness + timelines + reads, nil
}

// ForgetTimeline throws out a picture's timeline, once the facts of its probe it was read by changed,
// as ForgetLoudness does a measurement: the sweep reads it again.
func ForgetTimeline(ctx context.Context, queries *sqlcgen.Queries, of library.DerivedOf) (bool, error) {
	deleted, err := queries.DeleteVideoTimeline(ctx, sqlcgen.DeleteVideoTimelineParams{
		PayloadHash: of.PayloadHash, VideoStreamIndex: int64(of.StreamIndex),
	})
	if err != nil {
		return false, fmt.Errorf("forget the timeline of file %d's stream %d: %w", of.FileID, of.StreamIndex, err)
	}
	return deleted > 0, nil
}
