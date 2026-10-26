// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/together"
)

// togetherStaleEvery is how often Watch Together's links past their lifetime and its idle sessions
// are ended: a bound on how late, against lifetimes of an hour and more. A link is refused from the
// moment its lifetime ends whatever the look's time (D346); one look reads memory alone.
const togetherStaleEvery = time.Minute

// staleEnder ends what has outlived its lifetime: together.Hub.
type staleEnder interface {
	EndStale(lifetimes together.Lifetimes) []int64
}

// endStaleTogether ends, at each tick until ctx ends, the Watch Together links and sessions that have
// outlived the settings' lifetimes, read afresh each time, which links made from then on keep (D346).
func endStaleTogether(
	ctx context.Context, hub staleEnder, config settingReader, tick <-chan time.Time, logger *slog.Logger,
) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick:
		}
		hub.EndStale(together.Lifetimes{
			Link: lengthOrStarting(ctx, config, settings.TogetherLinkLifetimeSeconds, logger),
			Idle: lengthOrStarting(ctx, config, settings.TogetherIdleSessionEndHours, logger),
		})
	}
}
