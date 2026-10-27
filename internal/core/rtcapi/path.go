// SPDX-License-Identifier: AGPL-3.0-or-later

package rtcapi

import (
	"maps"
	"net"
	"net/netip"
	"strconv"
	"time"

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
	// PathHairpinnedIPv4 is a punched IPv4 pair whose two ends are one public address: the core's
	// server-reflexive address is the client's, so the pair goes out to the NAT both sit behind, a
	// router's or a carrier's, and back. A device in the house whose direct pair to the core's LAN
	// port failed takes it, and so does any other household behind the same carrier address (D354).
	PathHairpinnedIPv4 Path = "hairpinned_ipv4"
	// PathRelayed goes through a TURN relay, either side's.
	PathRelayed Path = "relayed"
	// PathFailed is a connection that failed before it took a path.
	PathFailed Path = "failed"
)

// Paths are every path, in the order the System page lists them.
func Paths() []Path {
	return []Path{
		PathDirectIPv6, PathDirectIPv4, PathPunchedIPv6, PathPunchedIPv4, PathHairpinnedIPv4, PathRelayed, PathFailed,
	}
}

// pathOf is the path a selected candidate pair takes: relayed if either end is a relay, else direct
// when the core's end is an address of its own and punched when its NAT opened it, hairpinned when
// that punched IPv4 address is the client's own too; the family is the core's end's.
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
	case isSameAddress(local.Address, remote.Address):
		return PathHairpinnedIPv4
	default:
		return PathPunchedIPv4
	}
}

// isSameAddress is whether two candidates' addresses are one IP address.
func isSameAddress(one, other string) bool {
	first, err := netip.ParseAddr(one)
	if err != nil {
		return false
	}
	second, err := netip.ParseAddr(other)
	return err == nil && first.Unmap() == second.Unmap()
}

// recordPath counts the path a connection took, and says it in the log with how long it took from
// its offer: a sign-in that waited on its connections said nothing of where the wait was (staging,
// 2026-10-23). It names both ends of the selected pair, pair nil for a connection that failed: the
// core's end by type and address says a hairpin at once, where the path and the client's address
// alone took a day's digging (local-path-2026-10-27.md §4).
func (server *Server) recordPath(path Path, pair *webrtc.ICECandidatePair, took time.Duration) {
	now := server.clock.Now()
	server.mutex.Lock()
	server.paths[path]++
	if remote := candidateOf(pair, false); remote != nil {
		server.localPath.saw(path, server.isHomeIP(net.ParseIP(remote.Address)), remote.Address, now)
	}
	server.mutex.Unlock()
	attributes := []any{
		"path", string(path), "remote", endOf(pair, false), "remoteType", typeOf(pair, false),
		"local", endOf(pair, true), "localType", typeOf(pair, true), "connectMs", took.Milliseconds(),
	}
	if path == PathHairpinnedIPv4 {
		server.logger.Warn("webrtc: a client's connection went out to the NAT both ends share and back (D354)",
			attributes...)
		return
	}
	server.logger.Info("webrtc: a client's connection", attributes...)
}

// endOf is one end of pair as "address:port", the core's when isLocal; "" for none.
func endOf(pair *webrtc.ICECandidatePair, isLocal bool) string {
	candidate := candidateOf(pair, isLocal)
	if candidate == nil {
		return ""
	}
	return net.JoinHostPort(candidate.Address, strconv.Itoa(int(candidate.Port)))
}

// typeOf is one end of pair's candidate type (host, srflx, prflx, relay), the core's when isLocal;
// "" for none.
func typeOf(pair *webrtc.ICECandidatePair, isLocal bool) string {
	candidate := candidateOf(pair, isLocal)
	if candidate == nil {
		return ""
	}
	return candidate.Typ.String()
}

// candidateOf is one end of pair, the core's when isLocal; nil for none.
func candidateOf(pair *webrtc.ICECandidatePair, isLocal bool) *webrtc.ICECandidate {
	switch {
	case pair == nil:
		return nil
	case isLocal:
		return pair.Local
	default:
		return pair.Remote
	}
}

// PathCounts are how many connections took each path since the core started (D274 §3).
func (server *Server) PathCounts() map[Path]int {
	server.mutex.Lock()
	defer server.mutex.Unlock()
	return maps.Clone(server.paths)
}

// selectedPath is the path a connected connection's selected pair takes.
func selectedPath(connection *webrtc.PeerConnection) Path {
	pair := selectedPair(connection)
	if pair == nil {
		return PathFailed
	}
	return pathOf(pair.Local, pair.Remote)
}

// selectedPair is a connection's selected candidate pair, nil before it has one.
func selectedPair(connection *webrtc.PeerConnection) *webrtc.ICECandidatePair {
	sctp := connection.SCTP()
	if sctp == nil {
		return nil
	}
	pair, err := sctp.Transport().ICETransport().GetSelectedCandidatePair()
	if err != nil {
		return nil
	}
	return pair
}
