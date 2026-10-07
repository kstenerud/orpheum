// SPDX-License-Identifier: GPL-3.0-or-later

package playback

import (
	"net/netip"
	"strconv"
	"sync"
	"time"
)

// speedFreshFor is how long a measured link speed stands for its player on its network: a day, after
// which a link may have changed for a reason no address shows, and a new measurement is waited for
// (D257 §1, "counting for less after a day").
const speedFreshFor = 24 * time.Hour

// speedKey is who measured a link speed, a device or, for one with no device row, its session, and
// on which network (networkOf).
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

// remember keeps a player's measurement on a network, in place of the last.
func (speeds *linkSpeeds) remember(key speedKey, kbps int64, at time.Time) {
	if kbps <= 0 || key.network == "" {
		return
	}
	speeds.mutex.Lock()
	defer speeds.mutex.Unlock()
	speeds.byKey[key] = measuredSpeed{kbps: kbps, at: at}
}

// fresh is a player's speed on a network, and whether one was measured within speedFreshFor of now.
func (speeds *linkSpeeds) fresh(key speedKey, now time.Time) (int64, bool) {
	speeds.mutex.Lock()
	defer speeds.mutex.Unlock()
	measured, found := speeds.byKey[key]
	if !found || now.Sub(measured.at) > speedFreshFor {
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

// networkOf names the network a client's address is on, as far as a link speed goes: an IPv6
// address's /64, the prefix one network gives its devices, or an IPv4 address whole, the public
// address of a household or a phone's carrier; "" for an address it cannot read. A phone that moves
// from Wi-Fi to LTE comes from another (D257 §1).
func networkOf(address string) string {
	parsed, err := netip.ParseAddr(address)
	if err != nil {
		return ""
	}
	parsed = parsed.Unmap()
	if parsed.Is4() {
		return parsed.String()
	}
	prefix, err := parsed.Prefix(64)
	if err != nil {
		return ""
	}
	return prefix.String()
}
