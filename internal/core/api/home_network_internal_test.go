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
	"github.com/kstenerud/orpheum/internal/core/rtcapi"
)

// A request over WebRTC is from home when its client's address is in a home network or in the core's
// own IPv6 /64 on that connection, the house's IPv6 network, and no relay is between (D297 §6, D300
// §3): the owner's Mac reached nas.example over IPv6 from the house's /64 and was refused admin
// (2026-10-19).
func TestIsHomeOverWebRTC_CountsTheCoresOwnIPv6NetworkAsTheHouses(t *testing.T) {
	t.Parallel()
	own := []netip.Prefix{netip.MustParsePrefix("2a0f:2000:4753:8afc::/64")}
	inHouse, elsewhere := "[2a0f:2000:4753:8afc:9d1c:6dac:8031:6613]:61333", "[2a0f:2000:4753:9999::1]:61333"
	for _, each := range []struct {
		isHomeAddress bool
		remote        string
		own           []netip.Prefix
		path          rtcapi.Path
		want          bool
	}{
		{false, inHouse, own, rtcapi.PathDirectIPv6, true},
		{false, inHouse, own, rtcapi.PathPunchedIPv6, true},
		{false, inHouse, own, rtcapi.PathRelayed, false},
		{false, inHouse, nil, rtcapi.PathDirectIPv6, false},
		{false, elsewhere, own, rtcapi.PathDirectIPv6, false},
		{true, "192.168.1.20:50000", nil, rtcapi.PathDirectIPv4, true},
		{true, "192.168.1.20:50000", nil, rtcapi.PathRelayed, false},
		// A hairpin's carrier address is many households', even one named a home network (D354 §4).
		{true, "11.22.33.144:51234", nil, rtcapi.PathHairpinnedIPv4, false},
		{false, "11.22.33.144:51234", nil, rtcapi.PathPunchedIPv4, false},
	} {
		if got := isHomeOverWebRTC(each.isHomeAddress, each.remote, each.own, each.path); got != each.want {
			t.Errorf("isHomeOverWebRTC(%v, %s, %v, %s) = %v; want %v",
				each.isHomeAddress, each.remote, each.own, each.path, got, each.want)
		}
	}
}

// The System page is told the server's home addresses and whether their certificate is held, as the
// discovery status holds them (D292 §6, D297 §6).
func TestDiscoveryStatusBody_CarriesTheHomeAddressesAndWhetherTheyAreReady(t *testing.T) {
	t.Parallel()
	home := "https://192-168-1-250.k7xq2.orpheum-core.com:8443"
	body := discoveryStatusBody(discovery.Status{HomeAddresses: []string{home}, IsHomeAddressReady: true})
	if len(body.HomeAddresses) != 1 || body.HomeAddresses[0] != home || !body.IsHomeAddressReady {
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
			Network: netip.MustParsePrefix("2a0f:1000:1:2::/64"), Source: discoveryproto.HouseNetworkLearnt,
			Device: netip.MustParseAddr("192.168.1.30"), WitnessedAt: witnessed,
		},
		{Network: netip.MustParsePrefix("2a0f:3000:9:8::/64"), Source: discoveryproto.HouseNetworkConfigured},
	}})
	want := []HouseNetworkBody{
		{Network: "2a0f:1000:1:2::/64", Source: "learnt", Device: "192.168.1.30", WitnessedAt: 1_800_000_000_000},
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
	server.SetHomeNetworks([]netip.Prefix{netip.MustParsePrefix("192.168.1.0/24")})
	var marked bool
	handler := server.markingHome(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		marked = isFromHome(request.Context())
	}))
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody)
	request.RemoteAddr = "[2a0f:1000:1:2::30]:40000" // in the house network the discovery was told
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
	server.SetHomeNetworks([]netip.Prefix{netip.MustParsePrefix("192.168.1.0/24")})
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
