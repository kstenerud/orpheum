// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"

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
