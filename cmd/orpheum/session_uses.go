// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/kstenerud/orpheum/internal/core/settings"
)

// sessionUseFlusher writes the sessions' remembered uses (auth.Service.FlushSessionUses).
type sessionUseFlusher interface {
	FlushSessionUses(ctx context.Context) error
}

// writeSessionUses writes the sessions' remembered uses every
// sessionLastUsedWriteIntervalSeconds, read afresh each time, until ctx ends, and once more
// then, before the databases close: a request only remembers its session's use, and this writes it
// (D303 §4, DL10).
func writeSessionUses(ctx context.Context, uses sessionUseFlusher, config settingReader, logger *slog.Logger) {
	flush := func(ctx context.Context) {
		if err := uses.FlushSessionUses(ctx); err != nil {
			logger.ErrorContext(ctx, "could not write when sessions were last used; the next flush tries again",
				"err", err)
		}
	}
	every := func() time.Duration {
		return lengthOrStarting(ctx, config, settings.SessionLastUsedWriteIntervalSeconds, logger)
	}
	timer := time.NewTimer(every())
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			flush(context.WithoutCancel(ctx))
			return
		case <-timer.C:
			flush(ctx)
			timer.Reset(every())
		}
	}
}
