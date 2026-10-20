// SPDX-License-Identifier: AGPL-3.0-or-later

package chunks

import (
	"context"
	"time"

	"github.com/kstenerud/orpheum/internal/core/settings"
)

// chunkTiming is how far chunks are made ahead of a player and in one run, and over how long a run's
// speed is measured (D247 §10, D250 §6). They are settings (the constants sweep, 2026-10-20), kept by
// the run tracker as last read, so its paths, which hold its lock, never wait on the database.
type chunkTiming struct {
	// lookaheadMs is how far past a chunk a player asks for the chunks are made or on their way before
	// the run planned from it waits for the player to come closer.
	lookaheadMs int64
	// runSpanMs is the most film one run makes, at least lookaheadMs: a seek then starts encoding
	// where the player is, rather than waiting for a run elsewhere to end.
	runSpanMs int64
	// speedWindow is how far back a run's speed is measured, and how long after an eviction its node
	// is left to settle before another.
	speedWindow time.Duration
}

// startingChunkTiming is each at its setting's starting value.
func startingChunkTiming() chunkTiming {
	return chunkTiming{
		lookaheadMs: settings.StartingDuration(settings.ChunkLookaheadSeconds).Milliseconds(),
		runSpanMs:   settings.StartingDuration(settings.ChunkRunSpanSeconds).Milliseconds(),
		speedWindow: settings.StartingDuration(settings.ChunkRunSpeedWindowSeconds),
	}
}

// refreshChunkTiming reads the timing's settings into the run tracker and returns it; one that cannot
// be read is its starting value, said in the log. A run span below the look-ahead is the look-ahead.
func (service *Service) refreshChunkTiming(ctx context.Context) chunkTiming {
	timing := startingChunkTiming()
	read := func(key settings.Key) (time.Duration, bool) {
		value, err := service.settings.Duration(ctx, key)
		if err != nil {
			service.logger.WarnContext(ctx, "chunks: could not read a setting; going on at its starting value",
				"key", string(key), "err", err)
			return 0, false
		}
		return value, true
	}
	if lookahead, isRead := read(settings.ChunkLookaheadSeconds); isRead {
		timing.lookaheadMs = lookahead.Milliseconds()
	}
	if span, isRead := read(settings.ChunkRunSpanSeconds); isRead {
		timing.runSpanMs = span.Milliseconds()
	}
	if window, isRead := read(settings.ChunkRunSpeedWindowSeconds); isRead {
		timing.speedWindow = window
	}
	timing.runSpanMs = max(timing.runSpanMs, timing.lookaheadMs)
	service.runs.setTiming(timing)
	return timing
}

// setTiming keeps the timing the tracker's paths read.
func (tracker *runTracker) setTiming(timing chunkTiming) {
	tracker.mutex.Lock()
	defer tracker.mutex.Unlock()
	tracker.timing = timing
}

// timingNow is the timing as last read.
func (tracker *runTracker) timingNow() chunkTiming {
	tracker.mutex.Lock()
	defer tracker.mutex.Unlock()
	return tracker.timing
}
