// SPDX-License-Identifier: AGPL-3.0-or-later

package rtcapi_test

import (
	"context"
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
	"github.com/kstenerud/orpheum/internal/testutil"
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
	_, isConnected := rtcapitest.Dial(t, answerRelayedBy(server), 2*time.Second, rtcapitest.OnLoopback())
	if isConnected {
		t.Error("a client on the loopback connected through a discovery")
	}
	if count := rtcapi.ConnectionCount(server); count != 0 {
		t.Errorf("the core holds %d connections after the bound to connect; want none", count)
	}
	rtcapitest.Connect(t, server.Answer)
	_, err := server.AnswerRelayed(t.Context(), rtcapi.Description{Type: "offer", SDP: offerOf()}, nil)
	if !errors.Is(err, rtcapi.ErrOfferRefused) {
		t.Errorf("an offer with no fingerprint = %v; want ErrOfferRefused", err)
	}
	if count := rtcapi.ConnectionCount(server); count != 1 {
		t.Errorf("the core holds %d connections; want the direct one alone", count)
	}
}

// A connection a discovery's offer made that connects directly from a home network is told to its
// answerer as proven at home, once it connects (D356 §1).
func TestAnswerRelayed_TellsAConnectionProvenAtHome(t *testing.T) {
	t.Parallel()
	server := rtcapi.New(t.Context(), http.NotFoundHandler(), slog.New(slog.DiscardHandler))
	t.Cleanup(func() { _ = server.Close() }) //nolint:errcheck // the test is over
	// The test's client is on this machine, whose addresses are the core's own networks: a development
	// build's lifting of the address check lets it connect, and every IPv4 address is home here.
	server.AcceptAnyRelayedAddress()
	server.SetHomeNetworks([]netip.Prefix{netip.MustParsePrefix("0.0.0.0/0")})
	proven := make(chan rtcapi.ProvenAtHome, 1)
	rtcapitest.Connect(t, func(ctx context.Context, offer rtcapi.Description) (rtcapi.Description, error) {
		return server.AnswerRelayed(ctx, offer, func(each rtcapi.ProvenAtHome) { proven <- each })
	}, rtcapitest.OverIPv4())
	if told := testutil.Receive(t, proven, "the connection proven at home"); told.Path != rtcapi.PathDirectIPv4 ||
		!told.Device.Is4() {
		t.Errorf("the connection was told proven at home as %+v; want direct over IPv4 from the client", told)
	}
}

// answerRelayedBy is server's AnswerRelayed, telling no one of a connection proven at home.
func answerRelayedBy(server *rtcapi.Server) rtcapitest.Answerer {
	return func(ctx context.Context, offer rtcapi.Description) (rtcapi.Description, error) {
		return server.AnswerRelayed(ctx, offer, nil)
	}
}

// The public IPv6 addresses an offer names are its candidates', a device's own: never a private,
// link-local or IPv4 one, nor an mDNS name, and each once (D356, Q5).
func TestOfferedIPv6Addresses_AreTheOffersPublicIPv6Candidates(t *testing.T) {
	t.Parallel()
	sdp := offerOf(sha256Fingerprint, candidate("2a0f:1000:1:2::9"), candidate("2a0f:1000:1:2::9"),
		"a=candidate:2 1 udp 1686052607 2a0f:3000:5:6::1 50001 typ srflx raddr :: rport 0",
		candidate("fd77::1"), candidate("fe80::1"), candidate("11.22.33.144"),
		candidate("0a1b2c3d-1111-2222-3333-444455556666.local"))
	got := rtcapi.OfferedIPv6Addresses(sdp)
	if len(got) != 2 || got[0].String() != "2a0f:1000:1:2::9" || got[1].String() != "2a0f:3000:5:6::1" {
		t.Errorf("the offer's public IPv6 addresses are %v; want its two public IPv6 candidates, once each", got)
	}
}

// connectedCount is how many connections the core counted as connected, by any path.
func connectedCount(server *rtcapi.Server) int {
	count := 0
	for path, each := range server.PathCounts() {
		if path != rtcapi.PathFailed {
			count += each
		}
	}
	return count
}

// awaitConnected waits, ten seconds at most, until the core counted count connections as connected.
func awaitConnected(t *testing.T, server *rtcapi.Server, count int) {
	t.Helper()
	giveUp := time.NewTimer(10 * time.Second)
	defer giveUp.Stop()
	look := time.NewTicker(10 * time.Millisecond)
	defer look.Stop()
	for connectedCount(server) < count {
		select {
		case <-giveUp.C:
			t.Fatalf("the core counted %d connections; want %d", connectedCount(server), count)
		case <-look.C:
		}
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
	// One request in flight; the channel every client opens to have something to negotiate takes no
	// request's place.
	server, release := waitingServer(t, rtcapi.Bounds{PerSource: 1, Channels: 1})
	client := rtcapitest.Connect(t, server.Answer, rtcapitest.OnLoopback())
	if !strings.Contains(client.AnswerSDP(), "a=max-message-size:65536") {
		t.Errorf("the answer does not bound a client's messages to %d bytes:\n%s", rtcapi.MaxMessageBytes,
			client.AnswerSDP())
	}
	// The core counts the first as connected before the second dials: otherwise, on a loaded machine,
	// the core may see the second connect first and close the first as the one past the share.
	awaitConnected(t, server, 1)
	if second, isConnected := rtcapitest.Dial(t, server.Answer, 10*time.Second, rtcapitest.OnLoopback()); isConnected &&
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

// A connection past its network's share is told why before it is closed, and serves nothing
// meanwhile; one within it is told it is held: closed with nothing said, the owner's Fire TV said its
// network could not reach the server (staging, 2026-10-25).
func TestAnswer_TellsAConnectionPastItsNetworksShareWhy(t *testing.T) {
	t.Parallel()
	server, _ := waitingServer(t, rtcapi.Bounds{PerSource: 1})
	first := rtcapitest.Connect(t, server.Answer, rtcapitest.OnLoopback())
	if verdict := first.Verdict(); verdict != (rtcapi.Verdict{Held: true}) {
		t.Errorf("the first connection was told %+v; want it held", verdict)
	}
	awaitConnected(t, server, 1)
	second := rtcapitest.Connect(t, server.Answer, rtcapitest.OnLoopback())
	if verdict := second.Verdict(); verdict != (rtcapi.Verdict{Refused: rtcapi.RefusedNetworkFull}) {
		t.Errorf("the connection past its network's share was told %+v; want %q", verdict, rtcapi.RefusedNetworkFull)
	}
	if !second.Begin(rtcapi.RequestHead{Method: http.MethodGet, Path: "/now"}, true).IsClosedWithin(5 * time.Second) {
		t.Error("a connection turned away served a request")
	}
	if !second.IsClosedWithin(10 * time.Second) {
		t.Error("a connection turned away was kept")
	}
}

// A network's share is its setting's (webrtc_connections_per_network, the owner, 2026-10-25): raised,
// a connection the old share turned away is held.
func TestServer_ANetworksShareIsAsItIsSet(t *testing.T) {
	t.Parallel()
	server, _ := waitingServer(t, rtcapi.Bounds{PerSource: 1})
	server.SetConnectionsPerNetwork(2)
	_ = rtcapitest.Connect(t, server.Answer, rtcapitest.OnLoopback())
	awaitConnected(t, server, 1)
	second := rtcapitest.Connect(t, server.Answer, rtcapitest.OnLoopback())
	if verdict := second.Verdict(); verdict != (rtcapi.Verdict{Held: true}) {
		t.Errorf("the second connection, within a share set to 2, was told %+v; want it held", verdict)
	}
}

// The channel a client opens only to negotiate its offer takes no request's place: on a connection
// that may hold one request at a time, a request is answered at once, not after the negotiation
// channel's deadline (staging, 2026-10-14, where each connection's showed as a request that never
// arrived).
func TestChannel_TheNegotiationChannelTakesNoRequestsPlace(t *testing.T) {
	t.Parallel()
	server, _ := waitingServer(t, rtcapi.Bounds{Channels: 1, RequestDeadline: 20 * time.Second})
	client := rtcapitest.Connect(t, server.Answer)
	if !client.Begin(rtcapi.RequestHead{Method: http.MethodGet, Path: "/now"}, true).IsAnsweredWithin(5 * time.Second) {
		t.Error("a request waited behind the negotiation channel")
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

// A request's head is the page's: the handler sees no header in it that says where the request came
// from, and knows the request came over WebRTC (the security audit's A1).
func TestChannel_TheHandlerBelievesNothingTheHeadSaysOfWhereItCameFrom(t *testing.T) {
	t.Parallel()
	told := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("X-Told", strings.Join([]string{
			request.Header.Get("Origin"), request.Header.Get("Forwarded"), request.Header.Get("X-Forwarded-For"),
			request.Host, request.Header.Get("Authorization"),
		}, "|"))
		if rtcapi.IsOverWebRTC(request.Context()) {
			writer.Header().Set("X-Over-WebRTC", "yes")
		}
		writer.WriteHeader(http.StatusNoContent)
	})
	server := rtcapi.New(t.Context(), told, slog.New(slog.DiscardHandler))
	t.Cleanup(func() { _ = server.Close() }) //nolint:errcheck // the test is over
	got := rtcapitest.Connect(t, server.Answer).Do(rtcapi.RequestHead{
		Method: http.MethodGet, Path: "/",
		Headers: http.Header{
			"origin": {"https://core.example"}, "forwarded": {"for=192.0.2.1"}, "x-forwarded-for": {"192.0.2.1"},
			"host": {"core.example"}, "authorization": {"Bearer t"},
		},
	}, nil)
	if told := got.Head.Headers.Get("X-Told"); told != "||||Bearer t" {
		t.Errorf("the handler was told %q; want only the sign-in", told)
	}
	if got.Head.Headers.Get("X-Over-WebRTC") != "yes" {
		t.Error("the handler did not know the request came over WebRTC")
	}
}

// A request whose channel the client closes, as the browser's transport does with one its caller
// aborted, ends its handler's work: the request's context ends with the channel, so a refresh given
// up or a chunk no longer wanted is not still made for no one.
func TestChannel_AChannelTheClientClosesEndsItsRequest(t *testing.T) {
	t.Parallel()
	started, stopped := make(chan struct{}), make(chan struct{})
	holding := http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		close(started)
		select {
		case <-request.Context().Done():
			close(stopped)
		case <-t.Context().Done():
		}
	})
	server := rtcapi.New(t.Context(), holding, slog.New(slog.DiscardHandler))
	t.Cleanup(func() { _ = server.Close() }) //nolint:errcheck // the test is over
	client := rtcapitest.Connect(t, server.Answer)
	begun := client.Begin(rtcapi.RequestHead{Method: http.MethodGet, Path: "/hold"}, true)
	awaitClosed(t, started, "the handler to start")
	begun.Close()
	awaitClosed(t, stopped, "the handler's request to end once its channel closed")
}

// awaitClosed waits, ten seconds at most, for ready to close.
func awaitClosed(t *testing.T, ready <-chan struct{}, what string) {
	t.Helper()
	giveUp := time.NewTimer(10 * time.Second)
	defer giveUp.Stop()
	select {
	case <-ready:
	case <-giveUp.C:
		t.Fatalf("waited 10s for %s", what)
	}
}
