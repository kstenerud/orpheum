// SPDX-License-Identifier: AGPL-3.0-or-later

package clientnet

import "testing"

// A client's network is an IPv6 address's /64, so a phone's changing address on one network is one
// network, and an IPv4 address whole; a phone that moves from Wi-Fi to LTE is on another (D257 §1).
func TestOf_IsAnIPv6AddressesSlash64OrAnIPv4AddressWhole(t *testing.T) {
	t.Parallel()
	for address, want := range map[string]string{
		"2001:db8:1:2:aaaa::1": "2001:db8:1:2::/64",
		"2001:db8:1:2:bbbb::7": "2001:db8:1:2::/64",
		"2001:db8:9:9::1":      "2001:db8:9:9::/64",
		"203.0.113.7":          "203.0.113.7",
		"::ffff:203.0.113.7":   "203.0.113.7",
		"not an address":       "",
	} {
		if got := Of(address); got != want {
			t.Errorf("Of(%q) = %q; want %q", address, got, want)
		}
	}
}

// A client's site is an IPv6 address's /48, so a customer's many /64s are one site, and an IPv4
// address whole (D288 §6, the second audit's K5).
func TestSiteOf_IsAnIPv6AddressesSlash48OrAnIPv4AddressWhole(t *testing.T) {
	t.Parallel()
	for address, want := range map[string]string{
		"2001:db8:1:2::1":    "2001:db8:1::/48",
		"2001:db8:1:ffff::1": "2001:db8:1::/48",
		"203.0.113.7":        "203.0.113.7",
		"::ffff:203.0.113.7": "203.0.113.7",
		"not an address":     "",
	} {
		if got := SiteOf(address); got != want {
			t.Errorf("SiteOf(%q) = %q; want %q", address, got, want)
		}
	}
}
