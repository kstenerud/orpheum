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

// reportedSteadiness is a chunked player's steadiness level (D352 §3), and when it reported it.
type reportedSteadiness struct {
	level int
	at    time.Time
}

// rememberedLink is what a player last said of its link on a network: its speed and its steadiness
// level, each with when, a zero time for one never said.
type rememberedLink struct {
	speed      measuredSpeed
	steadiness reportedSteadiness
}

// newestAt is when the player last said anything of the link.
func (link rememberedLink) newestAt() time.Time {
	if link.steadiness.at.After(link.speed.at) {
		return link.steadiness.at
	}
	return link.speed.at
}

// highestSteadiness is the player's steadiest level (highestSteadiness in steadiness.ts): a level
// reported above it is kept as it.
const highestSteadiness = 3

// steadinessRelaxesEvery is how long between playbacks relaxes a remembered steadiness level by
// one (D352 Q2). A playing player relaxes a level every 10 calm minutes it watched (D352 §3);
// between playbacks nothing is watched, so the level relaxes six times slower: a film started again
// within the hour starts as steady as the last ended, and a bad evening is forgotten by the next
// morning, well before the day the speed is kept.
const steadinessRelaxesEvery = time.Hour

// linkSpeeds are what players said of their links on real video, the latest of each player on each
// network (D257 §1): the speed, and the steadiness level a chunked player ended at (D352 Q2). They
// are kept in memory: a restart forgets them, and the next playback measures again from level 0.
type linkSpeeds struct {
	mutex sync.Mutex
	// byKey is the latest of each player on each network; guarded by mutex.
	byKey map[speedKey]rememberedLink
}

func newLinkSpeeds() *linkSpeeds { return &linkSpeeds{byKey: map[speedKey]rememberedLink{}} }

// remember keeps a player's measurement on a network, in place of the last, and forgets every link
// said nothing of within freshFor: each player on each network it ever played from was kept for the
// server's life (the M4 closing audit, 2026-10-08).
func (speeds *linkSpeeds) remember(key speedKey, kbps int64, measuredAt time.Time, freshFor time.Duration) {
	if kbps <= 0 || key.network == "" {
		return
	}
	speeds.keep(key, measuredAt, freshFor, func(link *rememberedLink) {
		link.speed = measuredSpeed{kbps: kbps, at: measuredAt}
	})
}

// rememberSteadiness keeps the steadiness level a player reported on a network, in place of the
// last, as remember keeps its speed; a level of 0 is kept too, over a steadier one before it.
func (speeds *linkSpeeds) rememberSteadiness(key speedKey, level int, reportedAt time.Time, freshFor time.Duration) {
	if level < 0 || key.network == "" {
		return
	}
	speeds.keep(key, reportedAt, freshFor, func(link *rememberedLink) {
		link.steadiness = reportedSteadiness{level: min(level, highestSteadiness), at: reportedAt}
	})
}

// keep changes the link key names by change, after forgetting every one said nothing of within
// freshFor of now.
func (speeds *linkSpeeds) keep(
	key speedKey, now time.Time, freshFor time.Duration, change func(link *rememberedLink),
) {
	speeds.mutex.Lock()
	defer speeds.mutex.Unlock()
	for known, link := range speeds.byKey {
		if now.Sub(link.newestAt()) > freshFor {
			delete(speeds.byKey, known)
		}
	}
	link := speeds.byKey[key]
	change(&link)
	speeds.byKey[key] = link
}

// fresh is a player's speed on a network, and whether one was measured within freshFor of now:
// link_speed_fresh_hours, a day to start (D257 §1).
func (speeds *linkSpeeds) fresh(key speedKey, now time.Time, freshFor time.Duration) (int64, bool) {
	speeds.mutex.Lock()
	defer speeds.mutex.Unlock()
	link, found := speeds.byKey[key]
	if !found || link.speed.at.IsZero() || now.Sub(link.speed.at) > freshFor {
		return 0, false
	}
	return link.speed.kbps, true
}

// steadiness is the steadiness level a player's next playback on a network starts at (D352 Q2): the
// one it last reported there within freshFor of now, as its speed is kept, relaxed by one for each
// steadinessRelaxesEvery since; 0 for none.
func (speeds *linkSpeeds) steadiness(key speedKey, now time.Time, freshFor time.Duration) int {
	speeds.mutex.Lock()
	defer speeds.mutex.Unlock()
	link, found := speeds.byKey[key]
	if !found || link.steadiness.at.IsZero() {
		return 0
	}
	since := max(0, now.Sub(link.steadiness.at))
	if since > freshFor {
		return 0
	}
	return max(0, link.steadiness.level-int(since/steadinessRelaxesEvery))
}

// playerOf names who measures a link: a device by its id, or a session with no device row, a Watch
// Together viewer's, by its key.
func playerOf(deviceID int64, sessionKey string) string {
	if deviceID != 0 {
		return "device " + strconv.FormatInt(deviceID, 10)
	}
	return "session " + sessionKey
}
