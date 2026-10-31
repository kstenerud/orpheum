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
