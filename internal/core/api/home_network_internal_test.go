// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/discovery/discoveryproto"
	"github.com/kstenerud/orpheum/internal/core/discovery"
	"github.com/kstenerud/orpheum/internal/core/home"
)

// The System page is told the server's home addresses and whether their certificate is held, as the
// discovery status holds them (D292 §6, D297 §6).
func TestDiscoveryStatusBody_CarriesTheHomeAddressesAndWhetherTheyAreReady(t *testing.T) {
	t.Parallel()
	homeAddress := "https://192-168-1-250.k7xq2.orpheum-core.com:8443"
	body := discoveryStatusBody(discovery.Status{HomeAddresses: []string{homeAddress}, IsHomeAddressReady: true})
	if len(body.HomeAddresses) != 1 || body.HomeAddresses[0] != homeAddress || !body.IsHomeAddressReady {
		t.Errorf("the body = %+v; want the home address, ready", body)
	}
	if none := discoveryStatusBody(discovery.Status{}); none.HomeAddresses == nil || len(none.HomeAddresses) != 0 {
		t.Errorf("with none, the body's home addresses = %#v; want an empty list, never null", none.HomeAddresses)
	}
}

// The System page is told the house's networks, how each was had, and for a learnt one by which
// device and when (D356 §10); none is an empty list.
func TestDiscoveryStatusBody_CarriesTheHouseNetworks(t *testing.T) {
	t.Parallel()
	witnessed := time.UnixMilli(1_800_000_000_000)
	body := discoveryStatusBody(discovery.Status{HouseNetworks: []discovery.HouseNetwork{
		{
			Network: netip.MustParsePrefix("2a0f:5000:1:2::/64"), Source: discoveryproto.HouseNetworkLearnt,
			Device: netip.MustParseAddr("192.168.1.30"), WitnessedAt: witnessed,
		},
		{Network: netip.MustParsePrefix("2a0f:3000:9:8::/64"), Source: discoveryproto.HouseNetworkConfigured},
	}})
	want := []HouseNetworkBody{
		{Network: "2a0f:5000:1:2::/64", Source: "learnt", Device: "192.168.1.30", WitnessedAt: 1_800_000_000_000},
		{Network: "2a0f:3000:9:8::/64", Source: "configured"},
	}
	if !slices.Equal(body.HouseNetworks, want) {
		t.Errorf("the body's house networks = %+v; want %+v", body.HouseNetworks, want)
	}
	if none := discoveryStatusBody(discovery.Status{}); none.HouseNetworks == nil || len(none.HouseNetworks) != 0 {
		t.Errorf("with none, the body's house networks = %#v; want an empty list, never null", none.HouseNetworks)
	}
}

// The System page is told the last week's dead ends by cause, as the discovery counted them, the
// commonest first; a discovery that keeps no causes gives an empty list, never null (D398 §7).
func TestDiscoveryStatusBody_CarriesTheDeadEndsCausesCommonestFirst(t *testing.T) {
	t.Parallel()
	body := discoveryStatusBody(discovery.Status{DeadEndsLastWeek: 6, DeadEndCausesLastWeek: map[string]int{
		"none": 1, discoveryproto.DeadEndCauseNoIPv6: 3, discoveryproto.DeadEndCauseUnknown: 1, "other": 0,
	}})
	want := []DeadEndCauseBody{{Cause: "noIpv6", Count: 3}, {Cause: "none", Count: 1}, {Cause: "unknown", Count: 1}}
	if !slices.Equal(body.DeadEndCauses, want) {
		t.Errorf("the body's dead-end causes = %+v; want %+v", body.DeadEndCauses, want)
	}
	if none := discoveryStatusBody(discovery.Status{}); none.DeadEndCauses == nil || len(none.DeadEndCauses) != 0 {
		t.Errorf("with none, the body's dead-end causes = %#v; want an empty list, never null", none.DeadEndCauses)
	}
}

// A house network grants nothing but the LAN name's place among a discovery's paths: a request from
// an address in one is from home only when that address is in a home network itself (D356 §7, D297 §6).
func TestMarkingHome_AHouseNetworkGrantsNoAdmin(t *testing.T) {
	t.Parallel()
	server := NewServer(slog.New(slog.DiscardHandler))
	server.SetHome(home.Listed(netip.MustParsePrefix("192.168.1.0/24")))
	var marked bool
	handler := server.markingHome(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		marked = isFromHome(request.Context())
	}))
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody)
	request.RemoteAddr = "[2a0f:5000:1:2::30]:40000" // in the house network the discovery was told
	handler.ServeHTTP(httptest.NewRecorder(), request)
	if marked {
		t.Error("a request from the house's learnt IPv6 network is from home; want away")
	}
}

// The server's own machine reaches the core from Docker's gateway, so a browser there was refused
// admin as away (staging, 2026-10-23); the port only that machine reaches is home (D334 §4).
func TestMarkingHome_ThisMachinesPortIsHome(t *testing.T) {
	t.Parallel()
	server := NewServer(slog.New(slog.DiscardHandler))
	server.SetHome(home.Listed(netip.MustParsePrefix("192.168.1.0/24")))
	isHome := func(remote, local string) bool {
		var marked bool
		handler := server.markingHome(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
			marked = isFromHome(request.Context())
		}))
		request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody)
		request.RemoteAddr = remote
		localAddress := net.TCPAddrFromAddrPort(netip.MustParseAddrPort(local))
		request = request.WithContext(context.WithValue(request.Context(), http.LocalAddrContextKey, localAddress))
		handler.ServeHTTP(httptest.NewRecorder(), request)
		return marked
	}
	if isHome("172.19.0.1:40000", "172.19.0.2:8441") {
		t.Error("with no port named for this machine, the gateway on 8441 is home; want away")
	}
	if server.thisMachineHint() != "" {
		t.Errorf("with no port named, the refusal adds %q; want nothing", server.thisMachineHint())
	}
	server.SetThisMachinePort(8441)
	for _, each := range []struct {
		remote, local string
		want          bool
	}{
		{"172.19.0.1:40000", "172.19.0.2:8441", true},
		{"172.19.0.1:40000", "172.19.0.2:8440", false},
		{"203.0.113.9:40000", "172.19.0.2:8440", false},
		{"192.168.1.20:40000", "172.19.0.2:8440", true},
	} {
		if got := isHome(each.remote, each.local); got != each.want {
			t.Errorf("a request from %s on %s is home: %v; want %v", each.remote, each.local, got, each.want)
		}
	}
	if hint := server.thisMachineHint(); !strings.Contains(hint, "http://localhost:8441") {
		t.Errorf("the refusal adds %q; want it to name http://localhost:8441", hint)
	}
}

// A direct request is home by the one rule rtcapi judges WebRTC by (D452 §1, §2): any private
// address but Docker's bridge, and the core's own public IPv6 /64 as its connections last found it,
// never another /64.
func TestMarkingHome_ADirectRequestIsHomeByTheSharedRule(t *testing.T) {
	t.Parallel()
	server := NewServer(slog.New(slog.DiscardHandler))
	rule := home.NewRule(home.Networks{
		PrivateIsHome: true, Interfaces: []netip.Prefix{netip.MustParsePrefix("172.17.0.0/16")},
	})
	rule.CountOwnIPv6Networks(func() []netip.Prefix {
		return []netip.Prefix{netip.MustParsePrefix("2a0f:5000:1:2::/64")}
	})
	server.SetHome(rule)
	handler := server.markingHome(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if isFromHome(request.Context()) {
			writer.WriteHeader(http.StatusNoContent)
		}
	}))
	for remote, want := range map[string]bool{
		"192.168.7.20:40000": true, "[2a0f:5000:1:2::30]:40000": true,
		"172.17.0.1:40000": false, "[2a0f:5000:1:3::30]:40000": false, "100.64.0.9:40000": false,
	} {
		request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody)
		request.RemoteAddr = remote
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if got := recorder.Code == http.StatusNoContent; got != want {
			t.Errorf("a direct request from %s is home: %v; want %v", remote, got, want)
		}
	}
}
