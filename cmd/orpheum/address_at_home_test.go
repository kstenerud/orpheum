// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"log/slog"
	"net/netip"
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/home"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// The core keeps its address at home over its settings and hands it to the home rule: a configured LAN
// address wins, and with none, one learned reaches the rule at once and is there after a restart
// (D452 §3, §5).
func TestKeepAddressAtHome_TheConfiguredAddressWinsElseTheLearnedOneReachesTheRule(t *testing.T) {
	t.Parallel()
	database := storetest.OpenMigrated(t)
	opened := openedServices{database: database, settings: settingstest.Seeded(t, database)}
	var config Config
	config.Discovery.LANAddresses = []string{"192.168.1.5:8443"}
	server, rule := newAPIServer(config, slog.New(slog.DiscardHandler))
	keeper, err := keepAddressAtHome(t.Context(), config, server, opened, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	keeper.Learn(t.Context(), netip.MustParseAddr("192.168.1.20"), "device")
	got := keeper.Known()
	if got.Origin != home.OriginConfigured || rule.AddressAtHome() != netip.MustParseAddr("192.168.1.5") {
		t.Errorf("with a LAN address configured, the address at home is %+v; want the configured one", got)
	}
	config.Discovery.LANAddresses = nil
	server, rule = newAPIServer(config, slog.New(slog.DiscardHandler))
	if keeper, err = keepAddressAtHome(t.Context(), config, server, opened, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatal(err)
	}
	keeper.Learn(t.Context(), netip.MustParseAddr("192.168.1.20"), "device")
	if rule.AddressAtHome() != netip.MustParseAddr("192.168.1.20") || server.Home() != rule {
		t.Errorf("with none configured, the rule holds %v after learning; want 192.168.1.20", rule.AddressAtHome())
	}
}

// A request naming the server's address teaches it on the plain listener's and HTTPS's ports and the
// ones a Host naming no port means, never another (D452 §3).
func TestTeachingPorts_AreThePlainAndHTTPSPorts(t *testing.T) {
	t.Parallel()
	var config Config
	config.Server.Listen, config.HTTPS.Listen, config.Server.ThisMachineListen = "0.0.0.0:8440", ":8443", "0.0.0.0:8441"
	got := teachingPorts(config)
	if want := []uint16{80, 443, 8440, 8443}; !slices.Equal(got, want) {
		t.Errorf("the teaching ports are %v; want %v", got, want)
	}
}

// A core sharing the host's network learns only an address one of its interfaces holds, so a device
// at home naming another private address in its Host does not repoint the server (D452 §3).
func TestKeepAddressAtHome_ACoreOnTheHostsNetworkLearnsNoAddressItDoesNotHold(t *testing.T) {
	t.Parallel()
	database := storetest.OpenMigrated(t)
	opened := openedServices{database: database, settings: settingstest.Seeded(t, database)}
	var config Config
	config.Server.AddressAtHomeFromDefaultRoute = true
	server, _ := newAPIServer(config, slog.New(slog.DiscardHandler))
	keeper, err := keepAddressAtHome(t.Context(), config, server, opened, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	// No machine running the tests holds this one: it is the last host of a /8 no lab hands out.
	keeper.Learn(t.Context(), netip.MustParseAddr("10.255.255.254"), "a guest's phone")
	if got := keeper.Known(); got.Origin == home.OriginLearned {
		t.Errorf("on the host's network, an address no interface holds was learned: %+v", got)
	}
}
