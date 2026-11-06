// SPDX-License-Identifier: AGPL-3.0-or-later

package playback

import (
	"slices"

	"github.com/kstenerud/orpheum/internal/core/decide"
)

// settleRaise is the playback's decision once its ladder is known, kept in started and the playback:
// a film converted only for a raise its ladder could not take plays as it is (D436).
func (playback *tokenPlayback) settleRaise(started *Started) decide.Decision {
	decision := directForARaiseOffTheLadder(playback.decision, started.OnLadder != nil)
	started.Decision, playback.decision = decision, decision
	return decision
}

// directForARaiseOffTheLadder is decision, or, where only a raise the device leaves to the server
// converts the film and it found no ladder to play on, the film played as it is, unraised: a raise
// is not worth a playback refused (D436).
func directForARaiseOffTheLadder(decision decide.Decision, isOnLadder bool) decide.Decision {
	onlyRaise := []decide.Reason{decide.ReasonLevellingRaise}
	if isOnLadder || !slices.Equal(decision.NotDirectReasons, onlyRaise) {
		return decision
	}
	return decide.Decision{Delivery: decide.Direct}
}
