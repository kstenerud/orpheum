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
