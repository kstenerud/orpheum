// SPDX-License-Identifier: AGPL-3.0-or-later

package home

import (
	"net/netip"
	"strings"
)

// zeroRoute is the destination and mask of the default route in the routing table.
const zeroRoute = "00000000"

// DefaultRouteAddress is the private IPv4 address of the interface carrying the default route, as
// table (the kernel's /proc/net/route) names it, among interfaces; invalid when there is none, or the
// route leaves through a container's interface (Docker's bridge, a veth): a core in a container sees
// its own bridge there, and so guesses nothing, which is why only a core sharing the host's network
// asks (D452 §3).
func DefaultRouteAddress(table string, interfaces []Interface) netip.Addr {
	for line := range strings.SplitSeq(table, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 8 || fields[1] != zeroRoute || fields[7] != zeroRoute || isContainerInterface(fields[0]) {
			continue
		}
		for _, each := range interfaces {
			if each.Name != fields[0] {
				continue
			}
			for _, address := range each.Addresses {
				if IsAtHome(address) {
					return address.Unmap()
				}
			}
		}
	}
	return netip.Addr{}
}
