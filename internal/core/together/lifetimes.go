// SPDX-License-Identifier: AGPL-3.0-or-later

package together

import "time"

// Watch Together is a short-term invitation (D346): a link not yet used admits someone for its
// lifetime from when it was made, and a session that nothing plays in and no one does anything in
// for the idle end is ended, as if its host had ended it. Ending a session ends its links and every
// membership at once (End).

// StartingLinkLifetime and StartingIdleEnd are a link's lifetime and a session's idle end before the
// settings', together_link_lifetime_minutes and togetherIdleSessionEndHours, are first read
// (EndStale): their starting values, copies which internal/core/api's
// TestTogether_TheStartingWaitsAreTheSettingsStarts holds equal (D346).
const (
	StartingLinkLifetime = time.Hour
	StartingIdleEnd      = 6 * time.Hour
)

// Lifetimes are how long a link not yet used admits someone, from when it was made, and how long a
// session lasts with nothing playing and no one doing anything in it (D346).
type Lifetimes struct {
	Link time.Duration
	Idle time.Duration
}

// startingLifetimes are the lifetimes until EndStale is first told the settings'.
func startingLifetimes() Lifetimes {
	return Lifetimes{Link: StartingLinkLifetime, Idle: StartingIdleEnd}
}

// EndStale ends what has outlived lifetimes, which links made from now on keep: the links not used
// within their lifetime, and every session idle for lifetimes.Idle, each of its devices told
// "idle" (D346). Idle is neither playing, a playing group with a player attached, nor anything a
// member does: a command, a join, a leave, a link made or withdrawn, a member removed, or the host
// opening their session. What a device's page sends by itself (its state, pings, notes, voice
// signalling) is not something a member does. A cast is not a party and is left to its own end. It
// returns the sessions it ended.
func (hub *Hub) EndStale(lifetimes Lifetimes) []int64 {
	now := hub.clock.Now()
	hub.mutex.Lock()
	defer hub.unlockTellingEnds()
	hub.lifetimes = lifetimes
	var ended []int64
	for id, each := range hub.sessions {
		if each.isCast {
			continue
		}
		each.dropExpiredLinks(now)
		if each.isPlayedNow() {
			each.lastActiveAt = now
			continue
		}
		if idle := now.Sub(each.lastActiveAt); idle >= lifetimes.Idle {
			each.logger.Info("watch together: a session ended, idle", sessionIDAttribute, id, "idleFor", idle.String())
			hub.endSession(each, "idle")
			ended = append(ended, id)
		}
	}
	return ended
}

// isPlayedNow is whether the group plays to a player: playing, with a device that plays attached.
func (current *session) isPlayedNow() bool {
	if current.group.state != GroupPlaying {
		return false
	}
	for _, each := range current.members {
		for _, attached := range each.devices {
			if attached.role.plays() {
				return true
			}
		}
	}
	return false
}

// touch notes that a member did something in the session now: it is not idle (D346).
func (current *session) touch(now time.Time) {
	current.lastActiveAt = now
}

// dropExpiredLinks removes the links not used within their lifetime: they admit no one now.
func (current *session) dropExpiredLinks(now time.Time) {
	for id, each := range current.links {
		if each.usedBy == 0 && !each.waits(now) {
			delete(current.links, id)
		}
	}
}

// waits is whether the link still admits its person now: not used, and within its lifetime (D346).
func (each *link) waits(now time.Time) bool {
	return each.usedBy == 0 && now.Before(each.expiresAt)
}
