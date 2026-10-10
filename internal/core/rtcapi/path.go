// SPDX-License-Identifier: GPL-3.0-or-later

package rtcapi

import (
	"maps"
	"net/netip"

	"github.com/pion/webrtc/v4"
)

// Path is how a client's connection reaches the core, as the candidate pair it selected says
// (D274 §3): the core's own address, a port its NAT opened to the client's packets, or a relay; over
// IPv6 or IPv4. The closed beta counts them, to say how often direct fails.
type Path string

// The paths a connection takes, and a connection that never took one.
const (
	// PathDirectIPv6 and PathDirectIPv4 reach an address the core has: on the LAN, or a public one
	// the core's router lets through.
	PathDirectIPv6 Path = "direct_ipv6"
	PathDirectIPv4 Path = "direct_ipv4"
	// PathPunchedIPv6 and PathPunchedIPv4 reach a port the core's NAT or firewall opened to the
	// client's packets, as STUN found it: punched through.
	PathPunchedIPv6 Path = "punched_ipv6"
	PathPunchedIPv4 Path = "punched_ipv4"
	// PathRelayed goes through a TURN relay, either side's.
	PathRelayed Path = "relayed"
	// PathFailed is a connection that failed before it took a path.
	PathFailed Path = "failed"
)

// Paths are every path, in the order the System page lists them.
func Paths() []Path {
	return []Path{PathDirectIPv6, PathDirectIPv4, PathPunchedIPv6, PathPunchedIPv4, PathRelayed, PathFailed}
}

// pathOf is the path a selected candidate pair takes: relayed if either end is a relay, else direct
// when the core's end is an address of its own and punched when its NAT opened it; the family is
// the core's end's.
func pathOf(local, remote *webrtc.ICECandidate) Path {
	if local == nil || remote == nil {
		return PathFailed
	}
	if local.Typ == webrtc.ICECandidateTypeRelay || remote.Typ == webrtc.ICECandidateTypeRelay {
		return PathRelayed
	}
	isIPv6 := false
	if address, err := netip.ParseAddr(local.Address); err == nil {
		isIPv6 = address.Unmap().Is6()
	}
	switch {
	case local.Typ == webrtc.ICECandidateTypeHost && isIPv6:
		return PathDirectIPv6
	case local.Typ == webrtc.ICECandidateTypeHost:
		return PathDirectIPv4
	case isIPv6:
		return PathPunchedIPv6
	default:
		return PathPunchedIPv4
	}
}

// recordPath counts the path a connection took, and says it in the log.
func (server *Server) recordPath(connection *webrtc.PeerConnection, path Path) {
	server.mutex.Lock()
	server.paths[path]++
	server.mutex.Unlock()
	server.logger.Info("webrtc: a client's connection", "path", string(path), "remote", remoteAddress(connection))
}

// PathCounts are how many connections took each path since the core started (D274 §3).
func (server *Server) PathCounts() map[Path]int {
	server.mutex.Lock()
	defer server.mutex.Unlock()
	return maps.Clone(server.paths)
}

// selectedPath is the path a connected connection's selected pair takes.
func selectedPath(connection *webrtc.PeerConnection) Path {
	sctp := connection.SCTP()
	if sctp == nil {
		return PathFailed
	}
	pair, err := sctp.Transport().ICETransport().GetSelectedCandidatePair()
	if err != nil || pair == nil {
		return PathFailed
	}
	return pathOf(pair.Local, pair.Remote)
}
