// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/kstenerud/orpheum/internal/core/artwork"
	"github.com/kstenerud/orpheum/internal/core/settings"
)

// maintainArtworkCache holds the artwork cache to its ceiling until ctx ends (D212 §6): a sweep
// when the server starts, every artwork_cache_sweep_every_hours, and whenever a write takes it past
// the ceiling; and when each image was served is written every
// artwork_served_at_flush_every_seconds and when it stops. Each wait is read afresh when it ends.
func maintainArtworkCache(ctx context.Context, service *artwork.Service, config settingReader, logger *slog.Logger) {
	sweep := func() {
		if _, err := service.Sweep(ctx); err != nil && ctx.Err() == nil {
			logger.ErrorContext(ctx, "the artwork cache sweep failed", "err", err)
		}
	}
	flush := func(ctx context.Context) {
		if err := service.Flush(ctx); err != nil {
			logger.ErrorContext(ctx, "could not write when artwork was served", "err", err)
		}
	}
	sweep()
	sweepTimer := time.NewTimer(lengthOrStarting(ctx, config, settings.ArtworkCacheSweepEveryHours, logger))
	flushTimer := time.NewTimer(lengthOrStarting(ctx, config, settings.ArtworkServedAtFlushEverySeconds, logger))
	defer sweepTimer.Stop()
	defer flushTimer.Stop()
	for {
		select {
		case <-ctx.Done():
			flush(context.WithoutCancel(ctx))
			return
		case <-service.SweepWakes():
			sweep()
		case <-sweepTimer.C:
			sweep()
			sweepTimer.Reset(lengthOrStarting(ctx, config, settings.ArtworkCacheSweepEveryHours, logger))
		case <-flushTimer.C:
			flush(ctx)
			flushTimer.Reset(lengthOrStarting(ctx, config, settings.ArtworkServedAtFlushEverySeconds, logger))
		}
	}
}
