// SPDX-License-Identifier: AGPL-3.0-or-later

package rtcapi_test

import (
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/pion/webrtc/v4"

	"github.com/kstenerud/orpheum/internal/core/home"
	"github.com/kstenerud/orpheum/internal/core/rtcapi"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// offerSDP is a client's offer, every candidate gathered.
func offerSDP(t *testing.T) string {
	t.Helper()
	connection, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() }) //nolint:errcheck // the test is over
	if _, err = connection.CreateDataChannel(rtcapi.NegotiationLabel, nil); err != nil {
		t.Fatal(err)
	}
	offer, err := connection.CreateOffer(nil)
	if err != nil {
		t.Fatal(err)
	}
	gathered := webrtc.GatheringCompletePromise(connection)
	if err = connection.SetLocalDescription(offer); err != nil {
		t.Fatal(err)
	}
	testutil.Receive(t, gathered, "the offer's candidates")
	return connection.LocalDescription().SDP
}

// freeUDPPort is a UDP port nothing holds now.
func freeUDPPort(t *testing.T) int {
	t.Helper()
	conn, err := net.ListenUDP("udp", &net.UDPAddr{})
	if err != nil {
		t.Fatal(err)
	}
	address, isUDP := conn.LocalAddr().(*net.UDPAddr)
	if !isUDP {
		t.Fatalf("a UDP listener's address is %T", conn.LocalAddr())
	}
	port := address.Port
	if err = conn.Close(); err != nil {
		t.Fatal(err)
	}
	return port
}

// A core taking WebRTC at home answers with the house's address, at its one published UDP port, as a
// host candidate, so a device in the house reaches it there and not through the VPS (D300 §1).
func TestListenLAN_AnswersWithTheHousesAddressAtItsPort(t *testing.T) {
	t.Parallel()
	server := rtcapi.New(t.Context(), http.NotFoundHandler(), slog.New(slog.DiscardHandler))
	t.Cleanup(func() { _ = server.Close() }) //nolint:errcheck // the test is over
	port := freeUDPPort(t)
	if err := server.ListenLAN(port, []netip.Addr{netip.MustParseAddr("192.0.2.55")}); err != nil {
		t.Fatal(err)
	}
	answer, err := server.Answer(t.Context(), rtcapi.Description{Type: "offer", SDP: offerSDP(t)})
	if err != nil {
		t.Fatal(err)
	}
	if want := fmt.Sprintf(" 192.0.2.55 %d typ host", port); !strings.Contains(answer.SDP, want) {
		t.Errorf("the answer holds no candidate %q:\n%s", want, answer.SDP)
	}
	ipv4, ipv6 := hostCandidatesAt(answer.SDP, port)
	if !slices.Equal(ipv4, []string{"192.0.2.55"}) || len(ipv6) > 1 {
		t.Errorf("the answer's host candidates at port %d are %v and %v; want the house's address alone and at "+
			"most one IPv6 address (D441):\n%s", port, ipv4, ipv6, answer.SDP)
	}
}

// hostCandidatesAt is the addresses of sdp's host candidates at port, IPv4 and IPv6, each once: pion
// names each candidate for both components.
func hostCandidatesAt(sdp string, port int) (ipv4, ipv6 []string) {
	for line := range strings.Lines(sdp) {
		fields := strings.Fields(strings.TrimPrefix(line, "a="))
		if len(fields) < 8 || !strings.HasPrefix(fields[0], "candidate:") || fields[1] != "1" || fields[7] != "host" ||
			fields[5] != strconv.Itoa(port) {
			continue
		}
		if address, err := netip.ParseAddr(fields[4]); err == nil && address.Is4() {
			ipv4 = append(ipv4, fields[4])
		} else {
			ipv6 = append(ipv6, fields[4])
		}
	}
	return ipv4, ipv6
}

// An offer through a discovery never makes the core check an address of its own networks, its home
// ones too: it drops every candidate naming one. A check that reaches the core from a home address is
// answered, sent by a device in the house; one from another private network is not (D300 §2; A2).
func TestRelayed_AnswersAHomeAddressOnlyAsTheSourceOfItsOwnCheck(t *testing.T) {
	t.Parallel()
	server := rtcapi.New(t.Context(), http.NotFoundHandler(), slog.New(slog.DiscardHandler))
	server.SetHome(home.Listed(netip.MustParsePrefix("192.168.1.0/24")))
	checked, err := rtcapi.CheckRelayedOffer(offerOf(sha256Fingerprint, candidate("203.0.113.1"),
		candidate("192.168.1.20"), candidate("10.0.0.7")))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(checked, "192.168.1.20") || strings.Contains(checked, "10.0.0.7") ||
		!strings.Contains(checked, "203.0.113.1") {
		t.Errorf("the offer as taken is %q; want only the candidate from outside", checked)
	}
	for address, want := range map[string]bool{"192.168.1.20": true, "10.0.0.7": false, "203.0.113.1": true} {
		if got := server.ChecksRelayed(net.ParseIP(address)); got != want {
			t.Errorf("a relayed connection answers a check from %s: %t; want %t", address, got, want)
		}
	}
}
