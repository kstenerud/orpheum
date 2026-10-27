// SPDX-License-Identifier: AGPL-3.0-or-later

package playback

import (
	"testing"
	"time"
)

// A player's speed is fresh for a day on the network it was measured on, and never stands for
// another network or player (D257 §1).
func TestLinkSpeeds_AreFreshForADayOnTheirOwnNetwork(t *testing.T) {
	t.Parallel()
	speeds := newLinkSpeeds()
	measuredAt := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	home := speedKey{player: playerOf(3, ""), network: "2001:db8:1:2::/64"}
	speeds.remember(home, 2000, measuredAt, speedFreshFor)
	if kbps, isKnown := speeds.fresh(home, measuredAt.Add(23*time.Hour), speedFreshFor); !isKnown || kbps != 2000 {
		t.Errorf("a day less an hour on: %d, %t; want 2000", kbps, isKnown)
	}
	if _, isKnown := speeds.fresh(home, measuredAt.Add(25*time.Hour), speedFreshFor); isKnown {
		t.Error("a speed past a day stands; want it not")
	}
	for _, other := range []speedKey{
		{player: playerOf(3, ""), network: "203.0.113.7"},
		{player: playerOf(4, ""), network: "2001:db8:1:2::/64"},
		{player: playerOf(0, "viewer"), network: "2001:db8:1:2::/64"},
	} {
		if _, isKnown := speeds.fresh(other, measuredAt, speedFreshFor); isKnown {
			t.Errorf("%+v has the speed measured for %+v; want none", other, home)
		}
	}
	speeds.remember(speedKey{player: playerOf(3, ""), network: ""}, 900, measuredAt, speedFreshFor)
	speeds.remember(home, 0, measuredAt, speedFreshFor)
	if kbps, _ := speeds.fresh(home, measuredAt, speedFreshFor); kbps != 2000 {
		t.Errorf("after a measurement of nothing, the speed is %d; want 2000 kept", kbps)
	}
}

// A speed no longer fresh is forgotten at the next measurement, so a server keeps no more than a
// day's players and networks (the M4 closing audit, 2026-10-08).
func TestLinkSpeeds_ForgetsTheStaleAtTheNextMeasurement(t *testing.T) {
	t.Parallel()
	speeds := newLinkSpeeds()
	start := time.Date(2026, time.October, 8, 20, 0, 0, 0, time.UTC)
	speeds.remember(speedKey{player: "device 1", network: "home"}, 2400, start, speedFreshFor)
	later := start.Add(speedFreshFor + time.Minute)
	speeds.remember(speedKey{player: "device 2", network: "home"}, 900, later, speedFreshFor)
	if len(speeds.byKey) != 1 {
		t.Errorf("after a day, the speeds kept are %v; want only the fresh one", speeds.byKey)
	}
}

// A player's steadiness level is kept beside its speed, by player and network, for as long as the
// speed is; it relaxes by one for each hour since it was reported, and a level of 0 reported after
// replaces a steadier one (D352 Q2).
func TestLinkSteadiness_IsKeptBesideTheSpeedAndRelaxesAnHourAtATime(t *testing.T) {
	t.Parallel()
	speeds := newLinkSpeeds()
	reportedAt := time.Date(2026, 10, 27, 20, 0, 0, 0, time.UTC)
	home := speedKey{player: playerOf(3, ""), network: "2001:db8:1:2::/64"}
	speeds.remember(home, 2000, reportedAt, speedFreshFor)
	speeds.rememberSteadiness(home, 3, reportedAt, speedFreshFor)
	for _, each := range []struct {
		after time.Duration
		want  int
	}{{0, 3}, {59 * time.Minute, 3}, {time.Hour, 2}, {2*time.Hour + time.Minute, 1}, {5 * time.Hour, 0}} {
		if level := speeds.steadiness(home, reportedAt.Add(each.after), speedFreshFor, relaxEvery); level != each.want {
			t.Errorf("%v after steadiness 3 was reported, the level is %d; want %d", each.after, level, each.want)
		}
	}
	if kbps, isKnown := speeds.fresh(home, reportedAt, speedFreshFor); !isKnown || kbps != 2000 {
		t.Errorf("beside the steadiness, the speed is %d, %t; want 2000 kept", kbps, isKnown)
	}
	for _, other := range []speedKey{
		{player: playerOf(3, ""), network: "203.0.113.7"},
		{player: playerOf(4, ""), network: "2001:db8:1:2::/64"},
	} {
		if level := speeds.steadiness(other, reportedAt, speedFreshFor, relaxEvery); level != 0 {
			t.Errorf("%+v starts at steadiness %d, reported for %+v; want 0", other, level, home)
		}
	}
	speeds.rememberSteadiness(home, 0, reportedAt.Add(time.Minute), speedFreshFor)
	if level := speeds.steadiness(home, reportedAt.Add(time.Minute), speedFreshFor, relaxEvery); level != 0 {
		t.Errorf("after steadiness 0 was reported, the level is %d; want 0", level)
	}
	steadyOnly := speedKey{player: playerOf(5, ""), network: "home"}
	speeds.rememberSteadiness(steadyOnly, 9, reportedAt, speedFreshFor)
	if level := speeds.steadiness(steadyOnly, reportedAt, speedFreshFor, relaxEvery); level != highestSteadiness {
		t.Errorf("a level of 9 reported is kept as %d; want the highest, %d", level, highestSteadiness)
	}
	if _, isKnown := speeds.fresh(steadyOnly, reportedAt, speedFreshFor); isKnown {
		t.Error("a player that reported only its steadiness has a speed; want none")
	}
	later := reportedAt.Add(speedFreshFor + time.Minute)
	if level := speeds.steadiness(steadyOnly, later, speedFreshFor, relaxEvery); level != 0 {
		t.Errorf("a day after it was reported, the level is %d; want 0, forgotten as the speed is", level)
	}
	speeds.remember(speedKey{player: playerOf(6, ""), network: "home"}, 900, later, speedFreshFor)
	if _, isKept := speeds.byKey[steadyOnly]; isKept {
		t.Error("a steadiness a day old is still kept after the next measurement; want it forgotten")
	}
}

// How fast a remembered level relaxes between playbacks is link_steadiness_relaxes_seconds, an hour
// to start: set to half an hour, level 3 is 2 after 30 minutes and gone after 90 (D352 Q2).
func TestLinkSteadiness_RelaxesAsOftenAsItsSettingSays(t *testing.T) {
	t.Parallel()
	speeds := newLinkSpeeds()
	reportedAt := time.Date(2026, 10, 27, 20, 0, 0, 0, time.UTC)
	home := speedKey{player: playerOf(3, ""), network: "home"}
	speeds.rememberSteadiness(home, 3, reportedAt, speedFreshFor)
	for _, each := range []struct {
		after time.Duration
		want  int
	}{{29 * time.Minute, 3}, {30 * time.Minute, 2}, {90 * time.Minute, 0}} {
		level := speeds.steadiness(home, reportedAt.Add(each.after), speedFreshFor, 30*time.Minute)
		if level != each.want {
			t.Errorf("relaxing every 30 min, %v after steadiness 3, the level is %d; want %d",
				each.after, level, each.want)
		}
	}
}
