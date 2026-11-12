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
	GuestCastsEndingIdle() []live.Cast
	End(userID, deviceID, castID int64) (live.Cast, error)
}

// guestRemover removes a guest receiver's device, its sign-in with it: auth.Service.
type guestRemover interface {
	RemoveGuestReceiver(ctx context.Context, userID, deviceID int64) error
	RemoveEveryGuestReceiver(ctx context.Context, isLive func(userID, deviceID int64) bool) error
}

// castEnder ends a cast's session, each of its devices told why: together.Hub.
type castEnder interface {
	EndCast(sessionID, userID int64, reason string) error
	CastIdleFor(sessionID, userID int64) (time.Duration, error)
}

// endGoneCasts ends, at each tick until ctx ends, the casts whose receiver has been gone for
// live.ReceiverGoneAfter, their sessions ended as receiverGone so their senders are told (D364 §6),
// and those whose receiver left its question unanswered for live.UnansweredAfter, ended as
// unanswered (D358 §4), and the casts to a guest receiver a device code made with nothing played for
// live.GuestIdleAfter, ended as idle (D469 §3); then it removes each guest receiver whose cast is
// over, or that none was cast to in time (D465 §1). As it starts, it removes the guest receivers a stopped core left.
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
		endIdleGuestCasts(ctx, casts, hub, logger)
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

// endIdleGuestCasts ends each cast to a guest receiver a device code made that has had nothing
// playing for live.GuestIdleAfter, as idle (D469 §3): its sender's pages are told, and the guest is
// removed with the casts' other ends.
func endIdleGuestCasts(ctx context.Context, casts goneReceivers, hub castEnder, logger *slog.Logger) {
	for _, each := range casts.GuestCastsEndingIdle() {
		idle, err := hub.CastIdleFor(each.ID, each.UserID)
		if err != nil || idle < live.GuestIdleAfter {
			continue
		}
		if _, err = casts.End(each.UserID, each.ReceiverDeviceID, each.ID); err != nil {
			continue // ended meanwhile
		}
		_ = hub.EndCast(each.ID, each.UserID, together.CastIdle) //nolint:errcheck // each is ended
		logger.InfoContext(ctx, "cast: a cast to a guest receiver ended, idle", "castId", each.ID,
			"userId", each.UserID, "to", each.ReceiverDeviceID, "idleFor", idle.String())
	}
}
