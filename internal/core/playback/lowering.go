// SPDX-License-Identifier: AGPL-3.0-or-later

package playback

import (
	"cmp"
	"slices"
	"time"

	"github.com/kstenerud/orpheum/internal/core/chunks"
)

// How the server lowers and raises its players' qualities to keep within its upload (D286 §4, §5;
// plans/upload-budget.md §4).
const (
	// The share of the films' share the need must stay under, for roomFor, before a lowered
	// playback is raised a quality: room enough that raising one does not lower it again at once.
	raiseUnderPercent = 75
	// How long the need must have had that room before each raise.
	roomFor = 30 * time.Second
)

// lowerable is a playback of chunks the server may lower: its ladder, lowest first, the rung its
// player loads, the rung it may load at most, and when it started and was last lowered.
type lowerable struct {
	token     string
	fileID    int64
	startedAt time.Time
	loweredAt time.Time
	rungs     []chunks.Rung
	loads     int
	ceiling   int
}

// rung is the rung it needs: the one it loads, under its ceiling.
func (playback lowerable) rung() int { return min(playback.loads, playback.ceiling) }

// needKbps is the rate of the rung it needs.
func (playback lowerable) needKbps() int64 { return playback.rungs[playback.rung()].AverageKbps }

// ceilingKbps is its ceiling as the player is told it: the rate of its highest rung allowed, zero
// when that is the top.
func (playback lowerable) ceilingKbps() int64 {
	if playback.ceiling >= len(playback.rungs)-1 {
		return 0
	}
	return playback.rungs[playback.ceiling].AverageKbps
}

// uploadControl is what the server keeps between its looks at its upload: since when the need has
// had room for a raise.
type uploadControl struct {
	roomSince time.Time
}

// look lowers playbacks, one quality at a time, the one needing most first, while the need, theirs
// and otherKbps that cannot be lowered, is over shareKbps; or, once the need has had room for roomFor,
// raises the playback lowered longest ago a quality. It changes playbacks' ceilings and loweredAt in
// place, and says whether any changed and whether the need is still over the share with every one
// at its lowest (D286 §5's last resort).
func (control *uploadControl) look(
	playbacks []lowerable, otherKbps, shareKbps int64, now time.Time,
) (isChanged, isOverAtLowest bool) {
	need := func() int64 {
		total := otherKbps
		for _, playback := range playbacks {
			total += playback.needKbps()
		}
		return total
	}
	for need() > shareKbps {
		index := toLower(playbacks)
		if index < 0 {
			control.roomSince = time.Time{}
			return isChanged, true
		}
		playbacks[index].ceiling = loweredRung(playbacks, index)
		playbacks[index].loweredAt = now
		isChanged = true
	}
	if isChanged || need() > shareKbps*raiseUnderPercent/100 {
		control.roomSince = time.Time{}
		return isChanged, false
	}
	if control.roomSince.IsZero() {
		control.roomSince = now
	}
	if now.Sub(control.roomSince) < roomFor {
		return false, false
	}
	index := toRaise(playbacks)
	if index < 0 {
		return false, false
	}
	raised := playbacks[index]
	// Counted at its raised ceiling, which its player may climb to.
	raisedNeed := need() - raised.needKbps() + raised.rungs[raised.ceiling+1].AverageKbps
	if raisedNeed > shareKbps*raiseUnderPercent/100 {
		return false, false // no room for it yet
	}
	playbacks[index].ceiling++
	control.roomSince = now
	return true, false
}

// toLower is the playback to lower next: the one needing most that is above its lowest rung, the
// newest of those needing the same; -1 when every one is at its lowest.
func toLower(playbacks []lowerable) int {
	chosen := -1
	for index, playback := range playbacks {
		if playback.rung() == 0 {
			continue
		}
		if chosen < 0 {
			chosen = index
			continue
		}
		best := playbacks[chosen]
		if order := cmp.Compare(playback.needKbps(), best.needKbps()); order > 0 ||
			(order == 0 && playback.startedAt.After(best.startedAt)) {
			chosen = index
		}
	}
	return chosen
}

// loweredRung is the rung the playback at index is lowered to: the next lower, or, where that is made
// for no other playback of its film and the one below it is, that one, so one encode feeds both and
// the node's work does not grow (D286 §4).
func loweredRung(playbacks []lowerable, index int) int {
	lowered := playbacks[index].rung() - 1
	if lowered > 0 && !isMadeForAnother(playbacks, index, lowered) && isMadeForAnother(playbacks, index, lowered-1) {
		return lowered - 1
	}
	return lowered
}

// isMadeForAnother is whether another playback of the film the playback at index plays needs the
// stream of its rung.
func isMadeForAnother(playbacks []lowerable, index, rung int) bool {
	name := playbacks[index].rungs[rung].StreamName
	return slices.ContainsFunc(playbacks, func(other lowerable) bool {
		return other.token != playbacks[index].token && other.fileID == playbacks[index].fileID &&
			other.rungs[other.rung()].StreamName == name
	})
}

// toRaise is the lowered playback lowered longest ago, -1 when none is lowered.
func toRaise(playbacks []lowerable) int {
	chosen := -1
	for index, playback := range playbacks {
		if playback.ceiling >= len(playback.rungs)-1 {
			continue
		}
		if chosen < 0 || playback.loweredAt.Before(playbacks[chosen].loweredAt) {
			chosen = index
		}
	}
	return chosen
}
