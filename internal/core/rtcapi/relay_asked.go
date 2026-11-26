// SPDX-License-Identifier: AGPL-3.0-or-later

package rtcapi

import (
	"slices"
	"strings"

	"github.com/pion/webrtc/v4"
)

// offersRelayed reports whether an offer's SDP carries a relayed candidate. A relay passes a
// household's datagrams only between its two ends' relayed addresses (D558 §4), so the core's own
// address at a relay serves a client that has one and no other (D559 §2).
func offersRelayed(offerSDP string) bool {
	for line := range strings.Lines(offerSDP) {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "a=candidate:") && strings.Contains(line, " typ relay") {
			return true
		}
	}
	return false
}

// withoutRelays are servers less every TURN URL, and less each server that had no other.
func withoutRelays(servers []webrtc.ICEServer) []webrtc.ICEServer {
	var kept []webrtc.ICEServer
	for _, server := range servers {
		urls := slices.DeleteFunc(slices.Clone(server.URLs), func(url string) bool {
			return strings.HasPrefix(url, "turn:") || strings.HasPrefix(url, "turns:")
		})
		if len(urls) > 0 {
			kept = append(kept, webrtc.ICEServer{URLs: urls})
		}
	}
	return kept
}

// serversFor are the servers a new connection gathers through, its relays among them only when
// mayRelay.
func (server *Server) serversFor(mayRelay bool) []webrtc.ICEServer {
	source := server.iceServers.Load()
	if source == nil {
		return nil
	}
	if !mayRelay {
		return withoutRelays((*source)())
	}
	return (*source)()
}
