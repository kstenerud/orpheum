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

	"github.com/kstenerud/orpheum/internal/core/rtcapi"
	"github.com/kstenerud/orpheum/internal/core/rtcapi/rtcapitest"
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
