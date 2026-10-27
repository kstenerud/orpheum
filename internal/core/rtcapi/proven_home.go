// SPDX-License-Identifier: AGPL-3.0-or-later

package rtcapi

import (
	"net"
	"net/netip"
	"slices"

	"github.com/pion/webrtc/v4"
)

// ProvenAtHome is a connection a discovery's offer made that reached the core directly from the
// house: its selected pair's core end an address of the core's own (a host candidate, not a port its
// NAT opened, a relay or the NAT both ends share), and its client's end in the home networks or in
// the core's own public IPv6 networks, as the core's end says (D300 §3, D356 §1). Device is that
// client's end, the device's address in the house.
type ProvenAtHome struct {
	Path   Path
	Device netip.Addr
}

// provenAtHome is what a connection that just connected over pair proves, with own the core's own
// public IPv6 networks on it: a ProvenAtHome when its pair is direct and its client's end at home,
// false otherwise. A punched, hairpinned or relayed pair proves nothing (D354 §4, D356 §1).
func (server *Server) provenAtHome(pair *webrtc.ICECandidatePair, own []netip.Prefix) (ProvenAtHome, bool) {
	if pair == nil || pair.Local == nil || pair.Remote == nil {
		return ProvenAtHome{}, false
	}
	path := pathOf(pair.Local, pair.Remote)
	device, err := netip.ParseAddr(pair.Remote.Address)
	if err != nil || (path != PathDirectIPv4 && path != PathDirectIPv6) {
		return ProvenAtHome{}, false
	}
	device = device.Unmap().WithZone("")
	isOwnNetwork := slices.ContainsFunc(own, func(network netip.Prefix) bool { return network.Contains(device) })
	if !isOwnNetwork && !server.isHomeIP(net.IP(device.AsSlice())) {
		return ProvenAtHome{}, false
	}
	return ProvenAtHome{Path: path, Device: device}, true
}

// tellIfProvenAtHome tells onProvenAtHome of a connection that just connected over pair when it
// proves itself from home; nil tells no one.
func (server *Server) tellIfProvenAtHome(
	pair *webrtc.ICECandidatePair, own []netip.Prefix, onProvenAtHome func(ProvenAtHome),
) {
	if onProvenAtHome == nil {
		return
	}
	if proven, isProven := server.provenAtHome(pair, own); isProven {
		onProvenAtHome(proven)
	}
}
