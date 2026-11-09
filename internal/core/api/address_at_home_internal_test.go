// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"crypto/tls"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/home"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// learningServer is a server whose address at home is learned on the house's ports 80, 443 and 8440,
// with this machine's port 8441, over a fresh database.
func learningServer(t *testing.T, parts home.AddressParts) (*Server, *home.AddressAtHome) {
	t.Helper()
	database := storetest.OpenMigrated(t)
	if parts.Rule == nil {
		parts.Rule = home.NewRule(home.Networks{})
	}
	parts.Settings, parts.Database = settingstest.Seeded(t, database), database
	parts.Logger = slog.New(slog.DiscardHandler)
	keeper, err := home.NewAddressAtHome(t.Context(), parts)
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(slog.New(slog.DiscardHandler))
	server.SetHome(parts.Rule)
	server.SetThisMachinePort(8441)
	server.SetAddressAtHome(keeper, 80, 443, 8440)
	return server, keeper
}

// requestTo is a direct request from remote naming host, arriving on the core's port local.
func requestTo(t *testing.T, remote, host string, local uint16, isTLS bool) *http.Request {
	t.Helper()
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody)
	request.RemoteAddr, request.Host = remote, host
	if isTLS {
		request.TLS = &tls.ConnectionState{}
	}
	localAddress := net.TCPAddrFromAddrPort(netip.AddrPortFrom(netip.MustParseAddr("192.168.1.20"), local))
	return request.WithContext(context.WithValue(request.Context(), http.LocalAddrContextKey, localAddress))
}

// A direct request from home naming the server by its private IPv4 address on the house's plain or
// HTTPS port teaches it, with the device's user agent; loopback, this machine's port, a request from
// away, another port and a public name teach nothing (D452 §3).
func TestMarkingHome_ARequestFromHomeNamingTheServersAddressTeachesIt(t *testing.T) {
	t.Parallel()
	for _, each := range []struct {
		name, remote, host string
		local              uint16
		isTLS              bool
		want               string
	}{
		{"the plain port", "192.168.1.30:40000", "192.168.1.20", 8440, false, "192.168.1.20"},
		{"HTTPS", "192.168.1.30:40000", "192.168.1.20", 8443, true, "192.168.1.20"},
		{"a named port", "192.168.1.30:40000", "192.168.1.20:8440", 8440, false, "192.168.1.20"},
		{"loopback", "127.0.0.1:40000", "192.168.1.20", 8440, false, ""},
		{"this machine's port", "172.19.0.1:40000", "192.168.1.20:8441", 8441, false, ""},
		{"away", "203.0.113.9:40000", "192.168.1.20", 8440, false, ""},
		{"another port", "192.168.1.30:40000", "192.168.1.20:9000", 9000, false, ""},
		{"a public address", "192.168.1.30:40000", "203.0.113.5", 8440, false, ""},
	} {
		server, keeper := learningServer(t, home.AddressParts{})
		request := requestTo(t, each.remote, each.host, each.local, each.isTLS)
		request.Header.Set("User-Agent", "Firefox on Linux")
		server.markingHome(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).
			ServeHTTP(httptest.NewRecorder(), request)
		got := keeper.Known()
		if gotText := map[bool]string{true: got.Address.String(), false: ""}[got.Address.IsValid()]; gotText !=
			each.want {
			t.Errorf("by %s, the address at home is %q; want %q", each.name, gotText, each.want)
		}
		if each.want != "" && got.Device != "Firefox on Linux" {
			t.Errorf("by %s, the address was learned from %q; want the user agent", each.name, got.Device)
		}
	}
}

// The System page is told the address at home and where it came from; an admin's set reaches it, and
// one that is no private IPv4 address is refused (D452 §4).
func TestHomeAddress_TheSystemPageShowsItAndAnAdminSetsIt(t *testing.T) {
	t.Parallel()
	database := storetest.OpenMigrated(t)
	adminID := storetest.InsertUser(t, database, "owner", "admin")
	keeper, err := home.NewAddressAtHome(t.Context(), home.AddressParts{
		Rule: home.NewRule(home.Networks{}), Settings: settingstest.Seeded(t, database), Database: database,
		Logger: slog.New(slog.DiscardHandler),
	})
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(slog.New(slog.DiscardHandler))
	server.SetAddressAtHome(keeper, 80)
	if body := server.addressAtHomeBody(); body.Address != "" || body.Origin != "" {
		t.Errorf("with none known, the body is %+v; want empty", body)
	}
	if err := keeper.Set(t.Context(), netip.MustParseAddr("192.168.1.40"), adminID); err != nil {
		t.Fatal(err)
	}
	if body := server.addressAtHomeBody(); body.Address != "192.168.1.40" || body.Origin != "set" ||
		body.SetByName != "owner" || body.At == 0 {
		t.Errorf("after an admin's set, the body is %+v; want 192.168.1.40 set by owner, with when", body)
	}
}

// The setup page is offered the guess while setup is required, to a request from home alone (D452 §4).
func TestHomeAddress_TheSetupPageIsOfferedTheGuessFromHome(t *testing.T) {
	t.Parallel()
	interfaces := func() []home.Interface {
		return []home.Interface{{Name: "eth0", Addresses: []netip.Addr{netip.MustParseAddr("192.168.1.20")}}}
	}
	server, keeper := learningServer(t, home.AddressParts{Interfaces: interfaces})
	offered := func(remote string) (address string, isKnown bool) {
		server.markingHome(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
			address, isKnown = server.setupAddressAtHome(request.Context())
		})).ServeHTTP(httptest.NewRecorder(), requestTo(t, remote, "orpheum.example.com", 8440, false))
		return address, isKnown
	}
	for remote, want := range map[string]string{"192.168.1.30:40000": "192.168.1.20", "203.0.113.9:40000": ""} {
		if address, isKnown := offered(remote); address != want || isKnown {
			t.Errorf("from %s, setup offers %q (known %v); want %q, a guess", remote, address, isKnown, want)
		}
	}
	keeper.Learn(t.Context(), netip.MustParseAddr("192.168.1.25"), "device")
	if address, isKnown := offered("192.168.1.30:40000"); address != "192.168.1.25" || !isKnown {
		t.Errorf("once one is learned, setup offers %q (known %v); want 192.168.1.25, known", address, isKnown)
	}
}
