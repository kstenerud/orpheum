// SPDX-License-Identifier: AGPL-3.0-or-later

package rtcapi

import (
	"log/slog"
	"net/http"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"

	"github.com/kstenerud/orpheum/internal/core/home"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// pairOf is a selected pair from the core's end, local, to the client's, remote.
func pairOf(localType webrtc.ICECandidateType, local string, remoteType webrtc.ICECandidateType,
	remote string,
) *webrtc.ICECandidatePair {
	end := func(kind webrtc.ICECandidateType, address string) *webrtc.ICECandidate {
		addrPort := netip.MustParseAddrPort(address)
		return &webrtc.ICECandidate{Typ: kind, Address: addrPort.Addr().String(), Port: addrPort.Port()}
	}
	return &webrtc.ICECandidatePair{Local: end(localType, local), Remote: end(remoteType, remote)}
}

// watchedServer is a server on a fake clock, whose log is log, taking WebRTC at home at 192.168.1.199
// on UDP port 8443, its home network 192.168.1.0/24.
func watchedServer(t *testing.T, log *testutil.Log, fake *clocktest.Fake) *Server {
	t.Helper()
	server := New(t.Context(), http.NotFoundHandler(), slog.New(slog.NewTextHandler(log, nil)))
	server.clock = fake
	server.SetHome(home.Listed(netip.MustParsePrefix("192.168.1.0/24")))
	server.lanPort, server.lanIPs = 8443, []netip.Addr{netip.MustParseAddr("192.168.1.199")}
	return server
}

// The core logs its own end of each connection's pair, by type and address, beside the client's: a
// pair with the carrier's address at both ends said a hairpin at once, where the path and the client's
// address alone took a day (local-path-2026-10-27.md §4 item 3).
func TestRecordPath_NamesBothEndsOfThePairByTypeAndAddress(t *testing.T) {
	t.Parallel()
	log := &testutil.Log{}
	server := watchedServer(t, log, clocktest.NewFake(time.Unix(1_800_000_000, 0)))
	server.recordPath(PathPunchedIPv4, pairOf(webrtc.ICECandidateTypeSrflx, "203.0.113.9:40001",
		webrtc.ICECandidateTypePrflx, "198.51.100.4:5000"), 600*time.Millisecond)
	log.AwaitEvent(t, "clientConnected", "path=punchedIpv4", "remote=198.51.100.4:5000",
		"remoteType=prflx", "local=203.0.113.9:40001", "localType=srflx", "connectMs=600")
}

// A hairpinned connection is named, and warned of, in the log; it stands as the admin's warning, with
// where the core takes WebRTC at home, until a direct connection from the house (D354 §2, §3).
func TestHairpinWarning_StandsUntilADirectConnectionFromTheHouse(t *testing.T) {
	t.Parallel()
	log := &testutil.Log{}
	fake := clocktest.NewFake(time.Unix(1_800_000_000, 0))
	server := watchedServer(t, log, fake)
	if _, isStanding := server.HairpinWarning(); isStanding {
		t.Fatal("a warning stands before any connection")
	}
	hairpin := pairOf(webrtc.ICECandidateTypeSrflx, "11.22.33.144:40001",
		webrtc.ICECandidateTypeSrflx, "11.22.33.144:51234")
	server.recordPath(pathOf(hairpin.Local, hairpin.Remote), hairpin, time.Second)
	line := log.AwaitLine(t, "path=hairpinnedIpv4", "localType=srflx", "local=11.22.33.144:40001")
	if !strings.Contains(line, "level=WARN") {
		t.Errorf("the hairpin's line is %q; want a warning", line)
	}
	warning, isStanding := server.HairpinWarning()
	if !isStanding || warning.Client != "11.22.33.144" || warning.Count != 1 || warning.UDPPort != 8443 ||
		len(warning.LANAddresses) != 1 || warning.LANAddresses[0].String() != "192.168.1.199" {
		t.Fatalf("the warning after a hairpin is %+v, standing %t; want one from 11.22.33.144 naming "+
			"UDP 8443 at 192.168.1.199", warning, isStanding)
	}
	// Another household's direct connection from away, or a punched one, clears nothing.
	server.recordPath(PathDirectIPv4, pairOf(webrtc.ICECandidateTypeHost, "192.168.1.199:8443",
		webrtc.ICECandidateTypeSrflx, "198.51.100.4:5000"), time.Second)
	server.recordPath(PathPunchedIPv4, pairOf(webrtc.ICECandidateTypeSrflx, "11.22.33.144:40001",
		webrtc.ICECandidateTypeSrflx, "198.51.100.4:5000"), time.Second)
	if _, isStanding = server.HairpinWarning(); !isStanding {
		t.Fatal("a connection from away cleared the warning")
	}
	server.recordPath(PathDirectIPv4, pairOf(webrtc.ICECandidateTypeHost, "192.168.1.199:8443",
		webrtc.ICECandidateTypePrflx, "192.168.1.30:5000"), time.Second)
	if warning, isStanding = server.HairpinWarning(); isStanding {
		t.Errorf("the warning stands after a direct connection from the house: %+v", warning)
	}
}

// A warning no hairpin renews lapses HairpinWarningLasts after the last one: whoever hairpinned has
// gone (D354 §3).
func TestHairpinWarning_LapsesAWeekAfterTheLastHairpin(t *testing.T) {
	t.Parallel()
	fake := clocktest.NewFake(time.Unix(1_800_000_000, 0))
	server := watchedServer(t, &testutil.Log{}, fake)
	hairpin := pairOf(webrtc.ICECandidateTypeSrflx, "11.22.33.144:40001",
		webrtc.ICECandidateTypePrflx, "11.22.33.144:51234")
	server.recordPath(pathOf(hairpin.Local, hairpin.Remote), hairpin, time.Second)
	fake.Advance(HairpinWarningLasts / 2)
	server.recordPath(pathOf(hairpin.Local, hairpin.Remote), hairpin, time.Second)
	fake.Advance(HairpinWarningLasts)
	if warning, isStanding := server.HairpinWarning(); !isStanding || warning.Count != 2 {
		t.Fatalf("the warning a week after the second of two hairpins is %+v, standing %t; want it "+
			"standing, counting two", warning, isStanding)
	}
	fake.Advance(time.Minute)
	if _, isStanding := server.HairpinWarning(); isStanding {
		t.Error("the warning stands past a week after the last hairpin")
	}
}
