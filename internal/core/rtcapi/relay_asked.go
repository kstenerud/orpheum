// SPDX-License-Identifier: AGPL-3.0-or-later

package rtcapi

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/pion/webrtc/v4"

	"github.com/kstenerud/orpheum/discovery/relaycred"
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
		urls := slices.DeleteFunc(slices.Clone(server.URLs), isTURN)
		if len(urls) > 0 {
			kept = append(kept, webrtc.ICEServer{URLs: urls})
		}
	}
	return kept
}

// relayCredentialFreshFor is how long a relay credential must still run for the core to count it
// fresh: one closer to its expiry than this has the core ask for a new one (askForRelayCredential).
const relayCredentialFreshFor = time.Minute

// isTURN reports whether url names a TURN server.
func isTURN(url string) bool {
	return strings.HasPrefix(url, "turn:") || strings.HasPrefix(url, "turns:")
}

// holdsFreshRelay reports whether servers name a TURN server whose credential runs on past
// relayCredentialFreshFor after now. Its username says until when (relaycred's form, hosted-relay.md
// §1); a username not in that form says nothing of its expiry, and is counted fresh, as the core cannot
// tell otherwise.
func holdsFreshRelay(servers []webrtc.ICEServer, now time.Time) bool {
	for _, server := range servers {
		if !slices.ContainsFunc(server.URLs, isTURN) {
			continue
		}
		credential, err := relaycred.Parse(server.Username)
		if err != nil || credential.ExpiresAt.Sub(now) > relayCredentialFreshFor {
			return true
		}
	}
	return false
}

// AskForRelayCredentialWith has ask called when a client's offer carries a relayed candidate and the
// core holds no relay credential fresh for relayCredentialFreshFor: no TURN server at all, or only
// ones whose credentials run out within it. The ask must not wait on its answer: the offer is answered
// with the servers the core holds, and the client's next try finds the credential the ask brought
// (hosted-relay.md, finding 3).
func (server *Server) AskForRelayCredentialWith(ask func(context.Context)) {
	server.relayAsk.Store(&ask)
}

// askForRelayCredential calls the ask AskForRelayCredentialWith set, unless none, when the core holds no
// fresh relay credential.
func (server *Server) askForRelayCredential(ctx context.Context) {
	ask := server.relayAsk.Load()
	if ask == nil {
		return
	}
	var servers []webrtc.ICEServer
	if source := server.iceServers.Load(); source != nil {
		servers = (*source)()
	}
	server.mutex.Lock()
	now := server.clock.Now()
	server.mutex.Unlock()
	if !holdsFreshRelay(servers, now) {
		(*ask)(ctx)
	}
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
