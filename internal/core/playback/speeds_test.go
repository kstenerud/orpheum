// SPDX-License-Identifier: GPL-3.0-or-later

package playback

import (
	"testing"
	"time"
)

// A link speed is kept by the network the client's address is on: an IPv6 address's /64, so a
// phone's changing address on one network is one network, and an IPv4 address whole; a phone that
// moves from Wi-Fi to LTE is on another (D257 §1).
func TestNetworkOf_IsAnIPv6AddressesSlash64OrAnIPv4AddressWhole(t *testing.T) {
	t.Parallel()
	for address, want := range map[string]string{
		"2001:db8:1:2:aaaa::1": "2001:db8:1:2::/64",
		"2001:db8:1:2:bbbb::7": "2001:db8:1:2::/64",
		"2001:db8:9:9::1":      "2001:db8:9:9::/64",
		"203.0.113.7":          "203.0.113.7",
		"::ffff:203.0.113.7":   "203.0.113.7",
		"not an address":       "",
	} {
		if got := networkOf(address); got != want {
			t.Errorf("networkOf(%q) = %q; want %q", address, got, want)
		}
	}
}

// A player's speed is fresh for a day on the network it was measured on, and never stands for
// another network or player (D257 §1).
func TestLinkSpeeds_AreFreshForADayOnTheirOwnNetwork(t *testing.T) {
	t.Parallel()
	speeds := newLinkSpeeds()
	measuredAt := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	home := speedKey{player: playerOf(3, ""), network: "2001:db8:1:2::/64"}
	speeds.remember(home, 2000, measuredAt)
	if kbps, isKnown := speeds.fresh(home, measuredAt.Add(23*time.Hour)); !isKnown || kbps != 2000 {
		t.Errorf("a day less an hour on: %d, %t; want 2000", kbps, isKnown)
	}
	if _, isKnown := speeds.fresh(home, measuredAt.Add(25*time.Hour)); isKnown {
		t.Error("a speed past a day stands; want it not")
	}
	for _, other := range []speedKey{
		{player: playerOf(3, ""), network: "203.0.113.7"},
		{player: playerOf(4, ""), network: "2001:db8:1:2::/64"},
		{player: playerOf(0, "viewer"), network: "2001:db8:1:2::/64"},
	} {
		if _, isKnown := speeds.fresh(other, measuredAt); isKnown {
			t.Errorf("%+v has the speed measured for %+v; want none", other, home)
		}
	}
	speeds.remember(speedKey{player: playerOf(3, ""), network: ""}, 900, measuredAt)
	speeds.remember(home, 0, measuredAt)
	if kbps, _ := speeds.fresh(home, measuredAt); kbps != 2000 {
		t.Errorf("after a measurement of nothing, the speed is %d; want 2000 kept", kbps)
	}
}
