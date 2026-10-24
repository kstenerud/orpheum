// SPDX-License-Identifier: AGPL-3.0-or-later

package interest

import (
	"slices"
	"sync"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/clock"
)

// Look is a session looking at a film's or series' page, or at one season's of a series: whose
// session, since when, and when it last said so.
type Look struct {
	ItemID int64
	// Season is the season of the series whose page it is, 0 for the specials; NoSeason for a
	// film's page or a series' own.
	Season int64
	UserID int64
	Since  time.Time
	SaidAt time.Time
}

// NoSeason is the season of a look at a film's page or a series' own, which shows no one season.
const NoSeason int64 = -1

// Register keeps, in memory, the page each session shows (D324 §1): one at a time, as a session
// shows one page. A restart loses it, which every shown page says again within its interval.
type Register struct {
	clock clock.Clock

	mutex sync.Mutex
	looks map[string]Look // guarded by mutex: by session key
}

// NewRegister returns an empty register.
func NewRegister(clockSource clock.Clock) *Register {
	return &Register{clock: clockSource, looks: map[string]Look{}}
}

// Look says the session, userID's, shows itemID's page now, the page of its season when season is
// not NoSeason; a session that showed another, or another season's, moves to this one.
func (register *Register) Look(sessionKey string, userID, itemID, season int64) {
	now := register.clock.Now()
	register.mutex.Lock()
	defer register.mutex.Unlock()
	look, isLooking := register.looks[sessionKey]
	if !isLooking || look.ItemID != itemID || look.Season != season {
		look = Look{ItemID: itemID, Season: season, UserID: userID, Since: now}
	}
	look.SaidAt = now
	register.looks[sessionKey] = look
}

// Leave says the session left itemID's page, its season's when season is not NoSeason; a session
// that has moved on to another, or to another season's, keeps it.
func (register *Register) Leave(sessionKey string, itemID, season int64) {
	register.mutex.Lock()
	defer register.mutex.Unlock()
	if look, isLooking := register.looks[sessionKey]; isLooking && look.ItemID == itemID && look.Season == season {
		delete(register.looks, sessionKey)
	}
}

// Looks are the looks said within lapse, the latest begun first; those past it are forgotten.
func (register *Register) Looks(lapse time.Duration) []Look {
	since := register.clock.Now().Add(-lapse)
	register.mutex.Lock()
	defer register.mutex.Unlock()
	looks := make([]Look, 0, len(register.looks))
	for sessionKey, look := range register.looks {
		if look.SaidAt.Before(since) {
			delete(register.looks, sessionKey)
			continue
		}
		looks = append(looks, look)
	}
	slices.SortFunc(looks, func(first, second Look) int {
		return second.Since.Compare(first.Since)
	})
	return looks
}
