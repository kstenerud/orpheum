// SPDX-License-Identifier: AGPL-3.0-or-later

package home_test

import (
	"context"
	"errors"
	"log/slog"
	"net/netip"
	"sync/atomic"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/home"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// keeper is a core's address at home over database, as one starting with parts would read it.
type keeper struct {
	*home.AddressAtHome

	settings *settings.Service
}

// keeperOver is the address at home kept in database, as a core starting with parts would read it.
func keeperOver(t *testing.T, database *store.Database, parts home.AddressParts) keeper {
	t.Helper()
	service := settingstest.Seeded(t, database)
	parts.Settings, parts.Database = service, database
	if parts.Rule == nil {
		parts.Rule = home.NewRule(home.Networks{})
	}
	parts.Logger = slog.New(slog.DiscardHandler)
	made, err := home.NewAddressAtHome(t.Context(), parts)
	if err != nil {
		t.Fatal(err)
	}
	return keeper{AddressAtHome: made, settings: service}
}

// entryOf is the setting key as the settings keep it.
func entryOf(t *testing.T, service *settings.Service, key settings.Key) settings.Entry {
	t.Helper()
	entries, err := service.List(t.Context(), false)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Definition.Key == key {
			return entry
		}
	}
	t.Fatalf("no setting %s", key)
	return settings.Entry{}
}

// A request from home that names the server by a private IPv4 address teaches it: the rule and every
// reaction take it at once, and the core started again keeps it, with the device (D452 §3).
func TestHomeAddress_ARequestFromHomeTeachesItAndARestartKeepsIt(t *testing.T) {
	t.Parallel()
	database := storetest.OpenMigrated(t)
	rule := home.NewRule(home.Networks{})
	running := keeperOver(t, database, home.AddressParts{Rule: rule})
	var told atomic.Pointer[netip.Addr]
	running.OnChange(func(address netip.Addr) { told.Store(&address) })
	if told.Load() != nil {
		t.Fatal("a reaction was told an address before any was known")
	}
	learned := netip.MustParseAddr("192.168.1.20")
	running.Learn(t.Context(), learned, "Firefox on Linux")
	if rule.AddressAtHome() != learned || told.Load() == nil || *told.Load() != learned {
		t.Fatalf("after learning, the rule holds %v and the reaction was told %v; want %v", rule.AddressAtHome(),
			told.Load(), learned)
	}
	restartedRule := home.NewRule(home.Networks{})
	restarted := keeperOver(t, database, home.AddressParts{Rule: restartedRule}).Known()
	if restarted.Address != learned || restarted.Origin != home.OriginLearned ||
		restarted.Device != "Firefox on Linux" || restarted.At.IsZero() || restartedRule.AddressAtHome() != learned {
		t.Errorf("after a restart, the address at home is %+v and the rule holds %v; want %v learned from Firefox",
			restarted, restartedRule.AddressAtHome(), learned)
	}
}

// A request that names the address known writes nothing; a new address is written with its device.
func TestHomeAddress_LearnsOnlyOnChange(t *testing.T) {
	t.Parallel()
	running := keeperOver(t, storetest.OpenMigrated(t), home.AddressParts{})
	running.Learn(t.Context(), netip.MustParseAddr("192.168.1.20"), "first")
	before := entryOf(t, running.settings, settings.AddressAtHomeLearnedFrom)
	running.Learn(t.Context(), netip.MustParseAddr("192.168.1.20"), "second")
	if after := entryOf(t, running.settings, settings.AddressAtHomeLearnedFrom); string(after.Value) != `"first"` ||
		string(before.Value) != `"first"` {
		t.Errorf("the address was learned from %s; want the first device alone", after.Value)
	}
	running.Learn(t.Context(), netip.MustParseAddr("192.168.1.30"), "third")
	if got := running.Known(); got.Address != netip.MustParseAddr("192.168.1.30") || got.Device != "third" {
		t.Errorf("after a new address, the address at home is %+v; want 192.168.1.30 from the third", got)
	}
}

// Loopback, a public address, carrier-grade NAT's and an IPv6 address never teach (D452 §3).
func TestHomeAddress_OnlyAPrivateIPv4AddressTeaches(t *testing.T) {
	t.Parallel()
	running := keeperOver(t, storetest.OpenMigrated(t), home.AddressParts{})
	for _, each := range []string{"127.0.0.1", "203.0.113.5", "fd00::5", "100.64.0.5"} {
		running.Learn(t.Context(), netip.MustParseAddr(each), "device")
	}
	if got := running.Known(); got.Address.IsValid() {
		t.Errorf("the address at home is %+v; want none learned", got)
	}
}

// A configured address wins: nothing is learned, and an admin's set is refused (D452 §5).
func TestHomeAddress_AConfiguredAddressWinsAndNothingIsLearned(t *testing.T) {
	t.Parallel()
	configured := netip.MustParseAddr("192.168.1.5")
	rule := home.NewRule(home.Networks{})
	running := keeperOver(t, storetest.OpenMigrated(t), home.AddressParts{Rule: rule, Configured: configured})
	running.Learn(t.Context(), netip.MustParseAddr("192.168.1.20"), "device")
	if got := running.Known(); got.Address != configured || got.Origin != home.OriginConfigured ||
		rule.AddressAtHome() != configured {
		t.Errorf("the address at home is %+v, the rule's %v; want the configured %v", got, rule.AddressAtHome(),
			configured)
	}
	if err := running.Set(t.Context(), netip.MustParseAddr("192.168.1.30"), 1); !errors.Is(err, home.ErrConfigured) {
		t.Errorf("an admin's set = %v; want ErrConfigured", err)
	}
}

// A name is looked up once, while no address is known, and its private IPv4 answer learned (D452 §4).
func TestHomeAddress_ANameIsLookedUpForItsPrivateIPv4Answer(t *testing.T) {
	t.Parallel()
	var lookups atomic.Int32
	resolve := func(_ context.Context, name string) ([]netip.Addr, error) {
		lookups.Add(1)
		if name != "openmediavault.local" {
			return nil, errors.New("no such name")
		}
		return []netip.Addr{netip.MustParseAddr("203.0.113.5"), netip.MustParseAddr("192.168.1.20")}, nil
	}
	running := keeperOver(t, storetest.OpenMigrated(t), home.AddressParts{Resolve: resolve})
	running.LearnName(t.Context(), "elsewhere.local", "device")
	running.LearnName(t.Context(), "elsewhere.local", "device")
	if lookups.Load() != 1 || running.Address().IsValid() {
		t.Fatalf("after a name with no answer twice, %d lookups and the address %v; want one and none",
			lookups.Load(), running.Address())
	}
	running.LearnName(t.Context(), "OpenMediaVault.local.", "device")
	if got := running.Address(); got != netip.MustParseAddr("192.168.1.20") {
		t.Errorf("the address at home is %v; want the name's private answer", got)
	}
	running.LearnName(t.Context(), "another.local", "device")
	if lookups.Load() != 2 {
		t.Errorf("%d lookups; want none once an address is known", lookups.Load())
	}
}

// An admin's set is kept as a learned address is, naming them; their removal forgets who (D452 §4,
// D305).
func TestHomeAddress_AnAdminsSetIsKeptWithTheirName(t *testing.T) {
	t.Parallel()
	database := storetest.OpenMigrated(t)
	adminID := storetest.InsertUser(t, database, "owner", "admin")
	running := keeperOver(t, database, home.AddressParts{})
	if err := running.Set(t.Context(), netip.MustParseAddr("203.0.113.5"), adminID); !errors.Is(err,
		home.ErrNotAtHome) {
		t.Errorf("a public address set = %v; want ErrNotAtHome", err)
	}
	if err := running.Set(t.Context(), netip.MustParseAddr("192.168.1.40"), adminID); err != nil {
		t.Fatal(err)
	}
	got := keeperOver(t, database, home.AddressParts{}).Known()
	if got.Address != netip.MustParseAddr("192.168.1.40") || got.Origin != home.OriginSet || got.SetByName != "owner" {
		t.Errorf("after a restart, the address at home is %+v; want 192.168.1.40 set by owner", got)
	}
	err := database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
		return queries.ClearSettingsSetByRemovedUser(t.Context(), &adminID)
	})
	if err != nil {
		t.Fatal(err)
	}
	got = keeperOver(t, database, home.AddressParts{}).Known()
	if got.Origin != home.OriginSet || got.SetByName != "" || !got.Address.IsValid() {
		t.Errorf("after the admin's removal, the address at home is %+v; want it kept, set by no one named", got)
	}
}

// A change on the settings page reaches the rule and the reactions as a learned one does; a reset
// forgets the address (D452 §3).
func TestHomeAddress_TheSettingsPageChangesItLive(t *testing.T) {
	t.Parallel()
	rule := home.NewRule(home.Networks{})
	running := keeperOver(t, storetest.OpenMigrated(t), home.AddressParts{Rule: rule})
	var told atomic.Pointer[netip.Addr]
	running.OnChange(func(address netip.Addr) { told.Store(&address) })
	settingstest.Set(t, running.settings, settings.AddressAtHome, `"10.0.0.9"`)
	if rule.AddressAtHome() != netip.MustParseAddr("10.0.0.9") || told.Load() == nil ||
		*told.Load() != netip.MustParseAddr("10.0.0.9") {
		t.Fatalf("after the settings page's change, the rule holds %v and the reaction was told %v; want 10.0.0.9",
			rule.AddressAtHome(), told.Load())
	}
	if err := running.settings.Reset(t.Context(), settings.AddressAtHome, nil); err != nil {
		t.Fatal(err)
	}
	if rule.AddressAtHome().IsValid() || told.Load().IsValid() {
		t.Errorf("after a reset, the rule holds %v and the reaction was told %v; want none", rule.AddressAtHome(),
			told.Load())
	}
}

// The guess is a private IPv4 address of an interface no container made, outside the tunnel's (D452 §4).
func TestHomeAddress_TheGuessIsAPrivateIPv4AddressOfNoContainerInterface(t *testing.T) {
	t.Parallel()
	interfaces := func() []home.Interface {
		return []home.Interface{
			{Name: "lo", Addresses: []netip.Addr{netip.MustParseAddr("127.0.0.1")}},
			{Name: "docker0", Addresses: []netip.Addr{netip.MustParseAddr("172.17.0.1")}},
			{Name: "orpheum0", Addresses: []netip.Addr{netip.MustParseAddr("10.77.0.2")}},
			{Name: "eth0", Addresses: []netip.Addr{
				netip.MustParseAddr("fd00::5"), netip.MustParseAddr("192.168.1.20"),
			}},
		}
	}
	rule := home.NewRule(home.Networks{NotHome: []netip.Prefix{netip.MustParsePrefix("10.77.0.0/24")}})
	running := keeperOver(t, storetest.OpenMigrated(t), home.AddressParts{Rule: rule, Interfaces: interfaces})
	if got := running.Guess(); got != netip.MustParseAddr("192.168.1.20") {
		t.Errorf("the guess is %v; want eth0's 192.168.1.20", got)
	}
}

// A core sharing the host's network (D454 §5) takes the default route's address as the address at
// home until one is learned or set, so the house's network, one of its own interfaces', is home and
// its first browser can teach; Docker's bridges stay not home, a proxy's container forwarding the
// internet arriving from one. A reset falls back to the guess (D452 §3).
func TestHomeAddress_ACoreOnTheHostsNetworkGuessesTheDefaultRoutesAddressUntilOneIsLearned(t *testing.T) {
	t.Parallel()
	rule := home.NewRule(home.Networks{Interfaces: []netip.Prefix{
		netip.MustParsePrefix("192.168.1.0/24"), netip.MustParsePrefix("172.17.0.0/16"),
	}})
	browser, proxy := netip.MustParseAddr("192.168.1.30"), netip.MustParseAddr("172.17.0.5")
	if rule.Contains(browser) {
		t.Fatal("before any address at home, the house's network, one of the core's own, was home")
	}
	guess := func() netip.Addr { return netip.MustParseAddr("192.168.1.20") }
	running := keeperOver(t, storetest.OpenMigrated(t), home.AddressParts{Rule: rule, DefaultRoute: guess})
	got := running.Known()
	if got.Address != guess() || got.Origin != home.OriginGuessed || !rule.Contains(browser) ||
		rule.Contains(proxy) || !running.IsLearning() {
		t.Fatalf("with the guess, the address at home is %+v, the browser home %v, the proxy's bridge home %v; "+
			"want the guess, the browser alone home, and still learning", got, rule.Contains(browser),
			rule.Contains(proxy))
	}
	running.Learn(t.Context(), guess(), "Firefox on Linux")
	got = running.Known()
	if got.Origin != home.OriginLearned || got.Device != "Firefox on Linux" || running.IsLearning() {
		t.Errorf("after a browser named the guessed address, the address at home is %+v; want it learned", got)
	}
	if err := running.settings.Reset(t.Context(), settings.AddressAtHome, nil); err != nil {
		t.Fatal(err)
	}
	if got := running.Known(); got.Address != guess() || got.Origin != home.OriginGuessed {
		t.Errorf("after a reset, the address at home is %+v; want the guess again", got)
	}
}

// A core in a container's own network guesses nothing: its default route is Docker's bridge, and
// taking it would make the internet, arriving from that bridge, home (D452 §3).
func TestHomeAddress_ACoreInAContainersNetworkGuessesNothing(t *testing.T) {
	t.Parallel()
	rule := home.NewRule(home.Networks{Interfaces: []netip.Prefix{netip.MustParsePrefix("172.18.0.0/16")}})
	running := keeperOver(t, storetest.OpenMigrated(t), home.AddressParts{Rule: rule})
	if got := running.Known(); got.Address.IsValid() || rule.Contains(netip.MustParseAddr("172.18.0.1")) {
		t.Errorf("the address at home is %+v and the bridge's gateway home %v; want none and not home", got,
			rule.Contains(netip.MustParseAddr("172.18.0.1")))
	}
}

// The default route's address is its interface's private IPv4 address, never a container interface's.
func TestDefaultRouteAddress_IsTheDefaultRoutesInterfacesPrivateIPv4AddressButNoContainers(t *testing.T) {
	t.Parallel()
	header := "Iface\tDestination\tGateway \tFlags\tRefCnt\tUse\tMetric\tMask\t\tMTU\tWindow\tIRTT\n"
	interfaces := []home.Interface{
		{Name: "docker0", Addresses: []netip.Addr{netip.MustParseAddr("172.17.0.1")}},
		{Name: "enp3s0", Addresses: []netip.Addr{netip.MustParseAddr("fd00::5"), netip.MustParseAddr("192.168.1.20")}},
	}
	for _, each := range []struct {
		name, table string
		want        netip.Addr
	}{
		{"the host's", header + "enp3s0\t0001A8C0\t00000000\t0001\t0\t0\t0\t00FFFFFF\t0\t0\t0\n" +
			"enp3s0\t00000000\t0101A8C0\t0003\t0\t0\t100\t00000000\t0\t0\t0\n", netip.MustParseAddr("192.168.1.20")},
		{"Docker's bridge", header + "docker0\t00000000\t010011AC\t0003\t0\t0\t0\t00000000\t0\t0\t0\n", netip.Addr{}},
		{"none", header + "enp3s0\t0001A8C0\t00000000\t0001\t0\t0\t0\t00FFFFFF\t0\t0\t0\n", netip.Addr{}},
	} {
		if got := home.DefaultRouteAddress(each.table, interfaces); got != each.want {
			t.Errorf("with %s default route, the address is %v; want %v", each.name, got, each.want)
		}
	}
}
