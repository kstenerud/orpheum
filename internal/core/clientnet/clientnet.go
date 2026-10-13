// SPDX-License-Identifier: AGPL-3.0-or-later

package clientnet

import "net/netip"

// Of names the network a client's address is on: an IPv6 address's /64, or an IPv4 address whole;
// "" for an address it cannot read. A phone that moves from Wi-Fi to LTE comes from another.
func Of(address string) string {
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

// SiteOf names the site a client's address is in, wider than its network: an IPv6 address's /48,
// the prefix a provider commonly gives one customer, so someone with a site's 65,536 networks is still
// one site; an IPv4 address whole, as Of; "" for an address it cannot read.
func SiteOf(address string) string {
	parsed, err := netip.ParseAddr(address)
	if err != nil {
		return ""
	}
	parsed = parsed.Unmap()
	if parsed.Is4() {
		return parsed.String()
	}
	prefix, err := parsed.Prefix(48)
	if err != nil {
		return ""
	}
	return prefix.String()
}
