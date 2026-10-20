// SPDX-License-Identifier: AGPL-3.0-or-later

package playback

import (
	"context"
	"log/slog"
	"time"

	"github.com/kstenerud/orpheum/internal/core/settings"
)

// playbackTiming is how long the service holds what its players tell it: a playing player's
// interest in its chunks with it silent (D309 §3), a quality ceiling for a node that could not make
// it fast enough (D309 §6, D286 §4), and a measured link speed (D257 §1). They are settings, kept here
// as of the last look at the upload (KeepUnderShare), so the paths that read them, which have no
// context and hold the tokens' lock, never wait on the database (the constants sweep, 2026-10-20).
type playbackTiming struct {
	interestLapsesAfter, cpuCeilingFor, speedFreshFor time.Duration
}

// startingPlaybackTiming is each at its setting's starting value.
func startingPlaybackTiming() playbackTiming {
	return playbackTiming{
		interestLapsesAfter: settings.StartingDuration(settings.ChunkInterestLapsesAfterSeconds),
		cpuCeilingFor:       settings.StartingDuration(settings.ChunkCPUCeilingKeptSeconds),
		speedFreshFor:       settings.StartingDuration(settings.LinkSpeedFreshHours),
	}
}

// timingNow is the timing as last read, or the starting values before the first read.
func (service *Service) timingNow() playbackTiming {
	if timing := service.timing.Load(); timing != nil {
		return *timing
	}
	return startingPlaybackTiming()
}

// refreshTiming reads the timing's settings, one that cannot be read at its starting value.
func (service *Service) refreshTiming(ctx context.Context, logger *slog.Logger) {
	timing := startingPlaybackTiming()
	for _, read := range []struct {
		key  settings.Key
		into *time.Duration
	}{
		{settings.ChunkInterestLapsesAfterSeconds, &timing.interestLapsesAfter},
		{settings.ChunkCPUCeilingKeptSeconds, &timing.cpuCeilingFor},
		{settings.LinkSpeedFreshHours, &timing.speedFreshFor},
	} {
		key, into := read.key, read.into
		value, err := service.settings.Duration(ctx, key)
		if err != nil {
			logger.WarnContext(ctx, "could not read a setting; going on at its starting value", "key", string(key),
				"err", err)
			continue
		}
		*into = value
	}
	service.timing.Store(&timing)
}
