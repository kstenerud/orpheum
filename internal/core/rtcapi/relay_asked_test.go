// SPDX-License-Identifier: AGPL-3.0-or-later

package rtcapi_test

import (
	"context"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"

	"github.com/kstenerud/orpheum/discovery/relaycred"
	"github.com/kstenerud/orpheum/internal/core/rtcapi"
	"github.com/kstenerud/orpheum/internal/core/rtcapi/rtcapitest"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
)

// askedRelay is a relay on the loopback that answers nothing and counts what it is asked; it
// returns its URL and the count.
func askedRelay(t *testing.T) (string, *atomic.Int64) {
	t.Helper()
	connection, err := (&net.ListenConfig{}).ListenPacket(t.Context(), "udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() }) //nolint:errcheck // the test is over
	var asked atomic.Int64
	go func() {
		buffer := make([]byte, 1500)
		for {
			if _, _, err := connection.ReadFrom(buffer); err != nil {
				return
			}
			asked.Add(1)
		}
	}()
	return "turn:" + connection.LocalAddr().String() + "?transport=udp", &asked
}

// relayedCandidate is a candidate line as a browser's relayed address is offered, at a
// documentation address nothing answers from.
const relayedCandidate = "a=candidate:9 1 udp 41885695 192.0.2.7 50000 typ relay raddr 0.0.0.0 rport 0"

// The core asks a relay for an address only for an offer that carries a relayed candidate: a relay
// passes a household's datagrams between its two ends' relayed addresses alone, so for any other
// offer the core's address there would carry nothing and hold one of the relay's ports (D559 §2;
// staging, 2026-11-26: the core took 11 allocations for one browser's connections).
func TestAnswer_AsksARelayOnlyForAnOfferThatCarriesARelayedCandidate(t *testing.T) {
	t.Parallel()
	server := newServer(t)
	relayURL, asked := askedRelay(t)
	server.UseICEServers(func() []webrtc.ICEServer {
		return []webrtc.ICEServer{{URLs: []string{relayURL}, Username: "core", Credential: "word"}}
	})
	var offered string
	rtcapitest.Connect(t, func(ctx context.Context, offer rtcapi.Description) (rtcapi.Description, error) {
		offered = offer.SDP
		return server.Answer(ctx, offer)
	})
	if count := asked.Load(); count != 0 {
		t.Fatalf("the relay was asked %d times for an offer with no relayed candidate, want 0", count)
	}

	relayed := strings.Replace(offered, "a=candidate:", relayedCandidate+"\r\na=candidate:", 1)
	if !strings.Contains(relayed, relayedCandidate) {
		t.Fatalf("the offer has no candidate line to put the relayed one before:\n%s", offered)
	}
	// The relay answers nothing, so this answer waits out its gathering: the asking is what is seen.
	ctx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
	defer cancel()
	_, _ = server.Answer(ctx, rtcapi.Description{Type: "offer", SDP: relayed}) //nolint:errcheck // above
	awaitAsked(t, asked)
}

// awaitAsked waits, five seconds at most, for the relay to have been asked.
func awaitAsked(t *testing.T, asked *atomic.Int64) {
	t.Helper()
	giveUp := time.NewTimer(5 * time.Second)
	defer giveUp.Stop()
	look := time.NewTicker(10 * time.Millisecond)
	defer look.Stop()
	for asked.Load() == 0 {
		select {
		case <-giveUp.C:
			t.Fatal("the relay was not asked within 5 s for an offer with a relayed candidate")
		case <-look.C:
		}
	}
}

// An offer with a relayed candidate while the core holds no relay credential, or only one within a
// minute of its expiry, has the core ask its discovery for one at once, so the client's next try finds
// it rather than one up to a heartbeat later (hosted-relay.md, finding 3). A fresh credential asks
// nothing, nor does an offer with no relayed candidate. The asking offer is answered all the same.
func TestAnswer_AnOfferThroughARelayAsksForARelayCredentialOnlyWhenTheCoreHoldsNoneFresh(t *testing.T) {
	t.Parallel()
	now := time.Unix(1_800_000_000, 0)
	relay := func(expiresIn time.Duration) []webrtc.ICEServer {
		username, err := relaycred.Credential{
			ExpiresAt: now.Add(expiresIn), Role: relaycred.RoleCore, Household: "household1", Kbps: 8000,
		}.Username()
		if err != nil {
			t.Fatal(err)
		}
		// A documentation address nothing answers from: the offers below are refused before any
		// gathering, so the relay is never asked.
		return []webrtc.ICEServer{
			{URLs: []string{"stun:192.0.2.1:3478"}},
			{URLs: []string{"turn:192.0.2.1:3478?transport=udp"}, Username: username, Credential: "word"},
		}
	}
	const viaRelay = "v=0\r\n" + relayedCandidate + "\r\n"
	const direct = "v=0\r\na=candidate:1 1 udp 2130706431 192.0.2.8 50000 typ host\r\n"
	for _, held := range []struct {
		name    string
		servers []webrtc.ICEServer
		offer   string
		isAsked bool
	}{
		{"no server at all", nil, viaRelay, true},
		{"STUN alone", []webrtc.ICEServer{{URLs: []string{"stun:192.0.2.1:3478"}}}, viaRelay, true},
		{"a credential half a minute from its end", relay(30 * time.Second), viaRelay, true},
		{"a credential an hour from its end", relay(time.Hour), viaRelay, false},
		{"no relayed candidate, no server", nil, direct, false},
	} {
		t.Run(held.name, func(t *testing.T) {
			t.Parallel()
			server := newServer(t)
			rtcapi.SetClock(server, clocktest.NewFake(now))
			server.UseICEServers(func() []webrtc.ICEServer { return held.servers })
			var asked atomic.Int64
			server.AskForRelayCredentialWith(func(context.Context) { asked.Add(1) })
			// The offer is no whole SDP, so its answer fails at once: the ask is what is seen.
			offer := rtcapi.Description{Type: "offer", SDP: held.offer}
			_, _ = server.Answer(t.Context(), offer) //nolint:errcheck // above
			if isAsked := asked.Load() > 0; isAsked != held.isAsked {
				t.Errorf("holding %s, the offer asked for a relay credential %t; want %t", held.name, isAsked,
					held.isAsked)
			}
		})
	}
}
