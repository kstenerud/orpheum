// SPDX-License-Identifier: AGPL-3.0-or-later

package playback

import (
	"strconv"
	"sync"
	"time"
)

// speedKey is who measured a link speed, a device or, for one with no device row, its session, and
// on which network (clientnet.Of).
type speedKey struct {
	player, network string
}

// measuredSpeed is a link speed a player measured, and when.
type measuredSpeed struct {
	kbps int64
	at   time.Time
}

// linkSpeeds are the link speeds players measured on real video, the latest of each player on each
// network (D257 §1). They are kept in memory: a restart forgets them, and the next playback measures
// again.
type linkSpeeds struct {
	mutex sync.Mutex
	// byKey is the latest speed of each player on each network; guarded by mutex.
	byKey map[speedKey]measuredSpeed
}

func newLinkSpeeds() *linkSpeeds { return &linkSpeeds{byKey: map[speedKey]measuredSpeed{}} }

// remember keeps a player's measurement on a network, in place of the last, and forgets every one no
// longer fresh: each player on each network it ever played from was kept for the server's life (the
// M4 closing audit, 2026-10-08).
func (speeds *linkSpeeds) remember(key speedKey, kbps int64, measuredAt time.Time, freshFor time.Duration) {
	if kbps <= 0 || key.network == "" {
		return
	}
	speeds.mutex.Lock()
	defer speeds.mutex.Unlock()
	for known, measured := range speeds.byKey {
		if measuredAt.Sub(measured.at) > freshFor {
			delete(speeds.byKey, known)
		}
	}
	speeds.byKey[key] = measuredSpeed{kbps: kbps, at: measuredAt}
}

// fresh is a player's speed on a network, and whether one was measured within freshFor of now:
// link_speed_fresh_hours, a day to start (D257 §1).
func (speeds *linkSpeeds) fresh(key speedKey, now time.Time, freshFor time.Duration) (int64, bool) {
	speeds.mutex.Lock()
	defer speeds.mutex.Unlock()
	measured, found := speeds.byKey[key]
	if !found || now.Sub(measured.at) > freshFor {
		return 0, false
	}
	return measured.kbps, true
}

// playerOf names who measures a link: a device by its id, or a session with no device row, a Watch
// Together viewer's, by its key.
func playerOf(deviceID int64, sessionKey string) string {
	if deviceID != 0 {
		return "device " + strconv.FormatInt(deviceID, 10)
	}
	return "session " + sessionKey
}
