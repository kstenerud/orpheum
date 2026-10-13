// SPDX-License-Identifier: AGPL-3.0-or-later

package rtcapi_test

import (
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/rtcapi"
	"github.com/kstenerud/orpheum/internal/core/rtcapi/rtcapitest"
)

// offerOf is an offer's SDP with the given attribute lines in its media section.
func offerOf(lines ...string) string {
	return strings.Join(append([]string{
		"v=0", "o=- 1 2 IN IP4 127.0.0.1", "s=-", "t=0 0",
		"m=application 9 UDP/DTLS/SCTP webrtc-datachannel", "c=IN IP4 0.0.0.0",
	}, lines...), "\r\n") + "\r\n"
}

// candidate is an a=candidate line for address.
func candidate(address string) string {
	return "a=candidate:1 1 udp 2122260223 " + address + " 50000 typ host"
}

const sha256Fingerprint = "a=fingerprint:sha-256 AB:CD"

// An offer a discovery passed on holds exactly one sha-256 fingerprint and at most
// MaxRelayedCandidates candidates; its mDNS host candidates are dropped (the security audit's A2,
// A9).
func TestCheckRelayedOffer_TakesOneSHA256FingerprintAndAFewCandidates(t *testing.T) {
	t.Parallel()
	many := []string{sha256Fingerprint}
	for index := range rtcapi.MaxRelayedCandidates + 1 {
		many = append(many, candidate(netip.AddrFrom4([4]byte{203, 0, 113, byte(index + 1)}).String()))
	}
	for name, sdp := range map[string]string{
		"no fingerprint":            offerOf(candidate("203.0.113.1")),
		"a second fingerprint":      offerOf("a=fingerprint:sha-256 EF:01", sha256Fingerprint),
		"a sha-1 fingerprint":       offerOf("a=fingerprint:sha-1 AB:CD"),
		"too many candidates":       offerOf(many...),
		"a session-level extra one": "a=fingerprint:sha-256 99:99\r\n" + offerOf(sha256Fingerprint),
	} {
		if _, err := rtcapi.CheckRelayedOffer(sdp); err == nil {
			t.Errorf("an offer with %s was taken", name)
		}
	}
	checked, err := rtcapi.CheckRelayedOffer(offerOf(sha256Fingerprint, candidate("203.0.113.1"),
		candidate("0a1b2c3d-1111-2222-3333-444455556666.local")))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(checked, ".local") || !strings.Contains(checked, "203.0.113.1") {
		t.Errorf("the offer as taken is %q; want its mDNS candidate dropped and the rest kept", checked)
	}
}

// A relayed offer's connection checks no remote address the core's own network or a private one
// holds: an offer cannot make the core send ICE checks into its LAN (the security audit's A2).
func TestIsFromOutside_RefusesEveryAddressOfTheCoresOwnNetworks(t *testing.T) {
	t.Parallel()
	own := func() []netip.Prefix { return []netip.Prefix{netip.MustParsePrefix("2a0f:1000:1:2::/64")} }
	for address, want := range map[string]bool{
		"203.0.113.7": true, "2a0f:1000:9:9::1": true,
		"127.0.0.1": false, "::1": false, "169.254.1.1": false, "fe80::1": false, "10.0.0.2": false,
		"192.168.1.20": false, "172.16.0.1": false, "fd00::1": false, "100.64.0.1": false,
		"224.0.0.251": false, "0.0.0.0": false, "2a0f:1000:1:2::99": false, "::ffff:192.168.1.20": false,
	} {
		if got := rtcapi.IsFromOutside(net.ParseIP(address), own); got != want {
			t.Errorf("IsFromOutside(%s) = %t; want %t", address, got, want)
		}
	}
}

// The same client, on the loopback, connects by the core's own offer endpoint and not by a
// discovery: the relayed connection checks none of its addresses, and is closed once its bound to
// connect passed. A refused offer makes no connection at all.
func TestAnswerRelayed_ConnectsNoClientOnTheCoresOwnNetwork(t *testing.T) {
	t.Parallel()
	server := newServer(t)
	rtcapi.SetBounds(server, rtcapi.Bounds{ConnectWithin: time.Second})
	if _, isConnected := rtcapitest.Dial(t, server.AnswerRelayed, 2*time.Second); isConnected {
		t.Error("a client on the loopback connected through a discovery")
	}
	if count := rtcapi.ConnectionCount(server); count != 0 {
		t.Errorf("the core holds %d connections after the bound to connect; want none", count)
	}
	rtcapitest.Connect(t, server.Answer)
	_, err := server.AnswerRelayed(t.Context(), rtcapi.Description{Type: "offer", SDP: offerOf()})
	if !errors.Is(err, rtcapi.ErrOfferRefused) {
		t.Errorf("an offer with no fingerprint = %v; want ErrOfferRefused", err)
	}
	if count := rtcapi.ConnectionCount(server); count != 1 {
		t.Errorf("the core holds %d connections; want the direct one alone", count)
	}
}

// waitingServer is a server whose handler holds a request for /wait until the test releases it.
func waitingServer(t *testing.T, bounds rtcapi.Bounds) (*rtcapi.Server, chan<- struct{}) {
	t.Helper()
	release := make(chan struct{})
	waiting := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/wait" {
			select {
			case <-release:
			case <-request.Context().Done():
			}
		}
		writer.WriteHeader(http.StatusNoContent)
	})
	server := rtcapi.New(t.Context(), waiting, slog.New(slog.DiscardHandler))
	t.Cleanup(func() {
		close(release)
		_ = server.Close() //nolint:errcheck // the test is over
	})
	rtcapi.SetBounds(server, bounds)
	return server, release
}

// A client takes only as much as the core lets it: messages no larger than MaxMessageBytes, which
// the answer says; no more connections from one network than its share; and no more requests in
// flight on one connection than its limit (the security audit's A2, code K16).
func TestAnswer_HoldsEachClientWithinItsBounds(t *testing.T) {
	t.Parallel()
	// Two in flight: the channel every client opens to have something to negotiate, and one request.
	server, release := waitingServer(t, rtcapi.Bounds{PerSource: 1, Channels: 2})
	client := rtcapitest.Connect(t, server.Answer)
	if !strings.Contains(client.AnswerSDP(), "a=max-message-size:65536") {
		t.Errorf("the answer does not bound a client's messages to %d bytes:\n%s", rtcapi.MaxMessageBytes,
			client.AnswerSDP())
	}
	if second, isConnected := rtcapitest.Dial(t, server.Answer, 10*time.Second); isConnected &&
		!second.IsClosedWithin(10*time.Second) {
		t.Error("a second connection from a network allowed one was kept")
	}
	waited := client.Begin(rtcapi.RequestHead{Method: http.MethodGet, Path: "/wait"}, true)
	if !client.Begin(rtcapi.RequestHead{Method: http.MethodGet, Path: "/now"}, true).IsClosedWithin(5 * time.Second) {
		t.Error("a request past the connection's limit in flight was taken")
	}
	release <- struct{}{}
	if !waited.IsAnsweredWithin(5 * time.Second) {
		t.Fatal("the request in flight was not answered once released")
	}
	if !client.Begin(rtcapi.RequestHead{Method: http.MethodGet, Path: "/now"}, true).IsAnsweredWithin(5 * time.Second) {
		t.Error("a request once the one in flight was answered was not taken")
	}
}

// A request whose head and body do not arrive whole in time is cut off, so no one holds the core's
// requests open by sending slowly (code K16).
func TestChannel_ARequestThatDoesNotArriveInTimeIsCutOff(t *testing.T) {
	t.Parallel()
	server, _ := waitingServer(t, rtcapi.Bounds{RequestDeadline: time.Second})
	client := rtcapitest.Connect(t, server.Answer)
	if !client.Begin(rtcapi.RequestHead{Method: http.MethodGet, Path: "/slow"}, false).IsClosedWithin(5 * time.Second) {
		t.Error("a request whose end never came was held past its deadline")
	}
}
