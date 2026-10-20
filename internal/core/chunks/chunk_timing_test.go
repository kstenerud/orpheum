// SPDX-License-Identifier: AGPL-3.0-or-later

package chunks

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/settings"
)

// timingSettings answers each length of time it has, and fails every other.
type timingSettings map[settings.Key]time.Duration

func (answers timingSettings) Int(context.Context, settings.Key) (int64, error) {
	return 0, errors.New("not asked for")
}

func (answers timingSettings) String(context.Context, settings.Key) (string, error) {
	return "", errors.New("not asked for")
}

func (answers timingSettings) Duration(_ context.Context, key settings.Key) (time.Duration, error) {
	if value, found := answers[key]; found {
		return value, nil
	}
	return 0, errors.New("the database is gone")
}

// How far ahead and how long runs are made, and their speed measured, are settings, read into the
// tracker: a run planned at a 20 s span makes 20 s of film, not the starting five minutes; a span set
// below the look-ahead is the look-ahead, and a setting that cannot be read its start (the constants
// sweep, 2026-10-20).
func TestRefreshChunkTiming_IsTheSettingsAndARunIsPlannedByThem(t *testing.T) {
	t.Parallel()
	answers := timingSettings{}
	answers[settings.ChunkLookaheadSeconds] = 10 * time.Second
	answers[settings.ChunkRunSpanSeconds] = 20 * time.Second
	service := &Service{settings: answers, logger: slog.New(slog.DiscardHandler), runs: newRunTracker()}
	timing := service.refreshChunkTiming(t.Context())
	if want := (chunkTiming{lookaheadMs: 10_000, runSpanMs: 20_000, speedWindow: speedWindow}); timing != want {
		t.Errorf("the timing read is %+v; want %+v, the window, unread, at its start", timing, want)
	}
	starts := make([]int64, 60)
	for index := range starts {
		starts[index] = int64(index) * 6000
	}
	grid := Grid{StartsMs: starts, FilmDurationMs: 360_000}
	first, last, isPlanned := service.runs.plan("store", grid, 0, wantsEvery)
	if !isPlanned || first != 0 || last != 3 {
		t.Errorf("a run planned from the start at a 20 s span = %d to %d (%t); want chunks 0 to 3, "+
			"those starting within 20 s", first, last, isPlanned)
	}
	answers[settings.ChunkRunSpanSeconds] = 5 * time.Second
	if raised := service.refreshChunkTiming(t.Context()); raised.runSpanMs != 10_000 {
		t.Errorf("a 5 s span with a 10 s look-ahead is %d ms; want the look-ahead's 10 000", raised.runSpanMs)
	}
}
