// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/kstenerud/orpheum/internal/core/live"
	"github.com/kstenerud/orpheum/internal/core/together"
)

// castsGoneEvery is how often the casts whose receiver is gone are looked for: a bound on how late,
// past live.ReceiverGoneAfter's two minutes, a sender is told (D364 §6); a look reads memory alone.
const castsGoneEvery = 10 * time.Second

// goneReceivers is what knows which casts' receivers are gone: live.Registry.
type goneReceivers interface {
	EndGone() []live.Cast
}

// castEnder ends a cast's session, each of its devices told why: together.Hub.
type castEnder interface {
	EndCast(sessionID, userID int64, reason string) error
}

// endGoneCasts ends, at each tick until ctx ends, the casts whose receiver has been gone for
// live.ReceiverGoneAfter, their sessions ended as receiverGone so their senders are told (D364 §6).
func endGoneCasts(ctx context.Context, casts goneReceivers, hub castEnder, tick <-chan time.Time, logger *slog.Logger) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick:
		}
		for _, gone := range casts.EndGone() {
			_ = hub.EndCast(gone.ID, gone.UserID, together.CastReceiverGone) //nolint:errcheck // gone is ended
			logger.InfoContext(ctx, "cast: a cast ended, its receiver gone", "castId", gone.ID,
				"userId", gone.UserID, "to", gone.ReceiverDeviceID)
		}
	}
}
