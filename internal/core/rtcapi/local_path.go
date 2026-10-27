// SPDX-License-Identifier: AGPL-3.0-or-later

package rtcapi

import (
	"net/netip"
	"slices"
	"time"
)

// HairpinWarningLasts is how long the warning of a hairpinned connection stands after the last one
// with no direct connection from the house since (D354 §3): a household whose UDP port is not
// reachable hairpins every time it plays, so a week with none says that whoever hairpinned has gone,
// a visitor or another household behind the same carrier address.
const HairpinWarningLasts = 7 * 24 * time.Hour

// Hairpin is the standing warning that devices reach the core through the NAT they share with it
// and back, not across the house's network (D354 §2): when the last such connection came, from which
// public address, how many came since the warning began, and where the core takes WebRTC at home,
// which a device in the house could not reach (a port of 0 and no addresses when it takes it nowhere).
type Hairpin struct {
	LastAt       time.Time
	Client       string
	Count        int
	UDPPort      int
	LANAddresses []netip.Addr
}

// localPathWatch is what the core's connections said of the path from the house: the hairpinned
// ones since the last direct one from a home network. Its owner guards it.
type localPathWatch struct {
	lastAt time.Time
	client string
	count  int
}

// saw takes a connection's path, whether its client's end is in the home networks, that end's
// address, and when it connected: a hairpin starts or adds to the warning, and a direct connection
// from the house, which proves its devices reach the core across its network, clears it (D354 §3).
func (watch *localPathWatch) saw(path Path, isFromHome bool, client string, at time.Time) {
	switch {
	case path == PathHairpinnedIPv4:
		watch.lastAt, watch.client = at, client
		watch.count++
	case isFromHome && (path == PathDirectIPv4 || path == PathDirectIPv6):
		*watch = localPathWatch{}
	}
}

// standing is the warning at now, and whether one stands: none before a hairpin, after a direct
// connection from the house, or HairpinWarningLasts after the last hairpin.
func (watch *localPathWatch) standing(now time.Time) (Hairpin, bool) {
	if watch.count == 0 || now.Sub(watch.lastAt) > HairpinWarningLasts {
		return Hairpin{}, false
	}
	return Hairpin{LastAt: watch.lastAt, Client: watch.client, Count: watch.count}, true
}

// HairpinWarning is the standing warning that devices reach the core through the NAT and back, with
// where it takes WebRTC at home, and whether one stands (D354).
func (server *Server) HairpinWarning() (Hairpin, bool) {
	now := server.clock.Now()
	server.mutex.Lock()
	defer server.mutex.Unlock()
	hairpin, isStanding := server.localPath.standing(now)
	if isStanding {
		hairpin.UDPPort, hairpin.LANAddresses = server.lanPort, slices.Clone(server.lanIPs)
	}
	return hairpin, isStanding
}
