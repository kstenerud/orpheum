// SPDX-License-Identifier: AGPL-3.0-or-later

package playback

import (
	"slices"

	"github.com/kstenerud/orpheum/internal/core/decide"
)

// settleWishes is the playback's decision once its ladder is known, kept in started and the playback:
// a film converted only for the device's wishes, its ladder not there to take them, plays as it is
// (D436, D489).
func (playback *tokenPlayback) settleWishes(started *Started) decide.Decision {
	decision := directForWishesOffTheLadder(playback.decision, started.OnLadder != nil)
	started.Decision, playback.decision = decision, decision
	return decision
}

// isWish is whether reason is one to convert a film its device could play as it is: a raise the
// device leaves to the server (D436), or the ladder a device's data cap asks for (D488).
func isWish(reason decide.Reason) bool {
	return reason == decide.ReasonLevellingRaise || reason == decide.ReasonUserChoice
}

// directForWishesOffTheLadder is decision, or, where only the device's wishes convert the film and it
// found no ladder to play on, the film played as it is: unraised, and over the device's data cap, as
// a wish is not worth a playback refused (D436, D489).
func directForWishesOffTheLadder(decision decide.Decision, isOnLadder bool) decide.Decision {
	isOnlyWished := !slices.ContainsFunc(decision.NotDirectReasons, func(reason decide.Reason) bool {
		return !isWish(reason)
	})
	if isOnLadder || decision.Delivery != decide.HLS || !isOnlyWished {
		return decision
	}
	return decide.Decision{Delivery: decide.Direct}
}
