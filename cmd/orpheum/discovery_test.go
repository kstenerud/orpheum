// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bytes"
	"log/slog"
	"net"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/discovery/discoveryproto"
	"github.com/kstenerud/orpheum/internal/core/auth"
)

// The heartbeat's own candidates are the machine's global addresses, a private one marked as
// reachable only at home; loopback and link-local ones are left out (D273).
func TestCandidatesOf_SendsPublicAddressesMarksPrivateOnesAsHomeAndNeverSendsTheRest(t *testing.T) {
	t.Parallel()
	var addresses []net.Addr
	for _, cidr := range []string{
		"127.0.0.1/8", "::1/128", "fe80::1/64", "192.168.1.20/24", "2a0f:1000::20/64", "fd00::20/64",
		// Carrier NAT, DS-Lite's own address and documentation: global unicast to Go, public to nobody.
		"100.64.0.5/10", "192.0.0.2/29", "2001:db8::20/64",
	} {
		ip, network, err := net.ParseCIDR(cidr)
		if err != nil {
			t.Fatal(err)
		}
		addresses = append(addresses, &net.IPNet{IP: ip, Mask: network.Mask})
	}
	got := candidatesOf(addresses, 8443)
	want := []discoveryproto.Candidate{
		{Address: "192.168.1.20", Port: 8443, Source: discoveryproto.SourceInterface, Scope: discoveryproto.ScopeLAN},
		{Address: "2a0f:1000::20", Port: 8443, Source: discoveryproto.SourceInterface},
		{Address: "fd00::20", Port: 8443, Source: discoveryproto.SourceInterface, Scope: discoveryproto.ScopeLAN},
	}
	if len(got) != len(want) {
		t.Fatalf("candidates = %+v; want %+v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Errorf("candidate %d = %+v; want %+v", index, got[index], want[index])
		}
	}
}

// The core gathers through its own STUN servers and the TURN servers its discovery names, with their
// credentials; never through the discovery's STUN servers, whose IPv6 answer would name the house's
// network, from which an admin administers (the Plex-chain review's L3).
func TestICEServersFor_AreTheCoresOwnSTUNAndTheDiscoverysTURN(t *testing.T) {
	t.Parallel()
	got := iceServersFor([]string{"stun:stun.example.org:3478"}, []discoveryproto.ICEServer{
		{URLs: []string{"stun:discovery.example:3478"}},
		{
			URLs: []string{"turn:discovery.example:3478", "stun:discovery.example:3479"}, Username: "user",
			Credential: "secret",
		},
	})
	if len(got) != 2 || got[0].URLs[0] != "stun:stun.example.org:3478" || len(got[1].URLs) != 1 ||
		got[1].URLs[0] != "turn:discovery.example:3478" || got[1].Username != "user" || got[1].Credential != "secret" {
		t.Errorf("iceServersFor = %+v; want the core's STUN server, then the discovery's TURN one with its "+
			"credentials and none of its STUN", got)
	}
	if none := iceServersFor(nil, nil); len(none) != 0 {
		t.Errorf("iceServersFor with no servers = %+v; want none, gathering only the machine's own addresses", none)
	}
}

// Each device a code activates is logged with who entered the code and for whom, so a device
// someone talked a person into activating is on the record (the device code audit's M4).
func TestLogActivation_NamesWhoActivatedWhatForWhom(t *testing.T) {
	t.Parallel()
	var logged bytes.Buffer
	logActivation(slog.New(slog.NewJSONHandler(&logged, nil)))(t.Context(), auth.Activation{
		ByUserID: 1, ForUserID: 2, DeviceName: "Living room TV", Platform: "Tizen", IsThroughService: true,
	})
	for _, want := range []string{
		`"msg":"a device was activated by a code"`, `"byUser":1`, `"forUser":2`, `"device":"Living room TV"`,
		`"platform":"Tizen"`, `"throughService":true`,
	} {
		if !strings.Contains(logged.String(), want) {
			t.Errorf("logged %s; want %s in it", logged.String(), want)
		}
	}
}

// A full failed-sign-in limit is warned of, so the operator learns that sign-ins are being refused
// for want of room (c21fe368).
func TestWarnSignInLimitFull_WarnsThatSignInsAreRefused(t *testing.T) {
	t.Parallel()
	var logged bytes.Buffer
	warnSignInLimitFull(t.Context(), slog.New(slog.NewJSONHandler(&logged, nil)))()
	if !strings.Contains(logged.String(), `"level":"WARN"`) ||
		!strings.Contains(logged.String(), "the failed-sign-in limit is full") {
		t.Errorf("logged %s; want a warning that the limit is full", logged.String())
	}
}
