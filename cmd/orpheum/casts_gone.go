// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/kstenerud/orpheum/internal/core/live"
	"github.com/kstenerud/orpheum/internal/core/together"
)

// castsGoneEvery is how often the casts whose receiver is gone, or never answered, are looked for: a
// bound on how late, past live.ReceiverGoneAfter's two minutes or live.UnansweredAfter, a sender is
// told (D364 §6, D358 §4); a look reads memory alone.
const castsGoneEvery = 10 * time.Second

// goneReceivers is what knows which casts' receivers are gone or never answered, and which guest
// receivers' casts are over: live.Registry.
type goneReceivers interface {
	EndGone() []live.Cast
	EndUnanswered() []live.Cast
	EndGuests() []live.Guest
	IsGuest(userID, deviceID int64) bool
}

// guestRemover removes a guest receiver's device, its sign-in with it: auth.Service.
type guestRemover interface {
	RemoveGuestReceiver(ctx context.Context, userID, deviceID int64) error
	RemoveEveryGuestReceiver(ctx context.Context, isLive func(userID, deviceID int64) bool) error
}

// castEnder ends a cast's session, each of its devices told why: together.Hub.
type castEnder interface {
	EndCast(sessionID, userID int64, reason string) error
}

// endGoneCasts ends, at each tick until ctx ends, the casts whose receiver has been gone for
// live.ReceiverGoneAfter, their sessions ended as receiverGone so their senders are told (D364 §6),
// and those whose receiver left its question unanswered for live.UnansweredAfter, ended as
// unanswered (D358 §4); then it removes each guest receiver whose cast is over, or that none was cast
// to in time (D465 §1). As it starts, it removes the guest receivers a stopped core left.
func endGoneCasts(
	ctx context.Context, casts goneReceivers, hub castEnder, guests guestRemover, tick <-chan time.Time,
	logger *slog.Logger,
) {
	if err := guests.RemoveEveryGuestReceiver(ctx, casts.IsGuest); err != nil {
		logger.ErrorContext(ctx, "cast: the guest receivers a stopped core left were not removed", "err", err)
	}
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
		for _, unanswered := range casts.EndUnanswered() {
			_ = hub.EndCast(unanswered.ID, unanswered.UserID, together.CastUnanswered) //nolint:errcheck // ended
			logger.InfoContext(ctx, "cast: a cast ended, never answered", "castId", unanswered.ID,
				"userId", unanswered.UserID, "to", unanswered.ReceiverDeviceID)
		}
		for _, guest := range casts.EndGuests() {
			if err := guests.RemoveGuestReceiver(ctx, guest.UserID, guest.DeviceID); err != nil {
				logger.ErrorContext(ctx, "cast: a guest receiver was not removed", "userId", guest.UserID,
					"deviceId", guest.DeviceID, "err", err)
				continue
			}
			logger.InfoContext(ctx, "cast: a guest receiver was removed, its cast over", "userId", guest.UserID,
				"deviceId", guest.DeviceID)
		}
	}
}
