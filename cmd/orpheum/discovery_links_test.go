// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/discovery/discoveryproto"
	"github.com/kstenerud/orpheum/discovery/link"
	"github.com/kstenerud/orpheum/discovery/proof"
	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/discovery"
	"github.com/kstenerud/orpheum/internal/core/secrets"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/core/together"
	"github.com/kstenerud/orpheum/internal/core/turn"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// The discovery finds the core by Watch Together's links and membership devices as by accounts'
// (discovery-core.md K7): an unused link's hash and a joined device's locator are registered, and an
// offer under either is proven under its key; a hash neither has proves nothing.
func TestLinksOf_TheDiscoveryFindsTheCoreByWatchTogethersLinksAndLocators(t *testing.T) {
	t.Parallel()
	database := storetest.OpenMigrated(t)
	fakeClock := clocktest.NewFake(time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC))
	accounts := auth.NewService(database, fakeClock, rand.Reader, settingstest.Seeded(t, database))
	sessions := together.NewHub(fakeClock, rand.Reader, nil, slog.New(slog.DiscardHandler))
	opened, err := sessions.Open(together.Person{UserID: 1, Name: "host"})
	if err != nil {
		t.Fatal(err)
	}
	unused, err := sessions.AddLink(opened.SessionID, 1, "a friend", together.Rights{})
	if err != nil {
		t.Fatal(err)
	}
	used, err := sessions.AddLink(opened.SessionID, 1, "my partner", together.Rights{})
	if err != nil {
		t.Fatal(err)
	}
	joined, err := sessions.Join(used.Secret, "Pat", nil, together.Leaving{})
	if err != nil {
		t.Fatal(err)
	}
	links, locators, err := linksOf(accounts, sessions)(t.Context())
	if err != nil || len(links) != 1 || links[0].LocateHash != link.LocateHash(unused.Secret) {
		t.Errorf("the links registered are %+v, %v; want the unused Watch Together link's", links, err)
	}
	if !slices.Equal(locators, []string{link.LocatorHash(joined.Locator)}) {
		t.Errorf("the locators registered are %v; want the joined device's", locators)
	}
	offerKey := offerKeyOf(accounts, sessions)
	for name, want := range map[string][]byte{
		link.LocateHash(unused.Secret):   link.OfferKey(unused.Secret),
		link.LocatorHash(joined.Locator): proof.SessionOfferKey(joined.MembershipCredential),
	} {
		if key, keyErr := offerKey(t.Context(), name); keyErr != nil || !bytes.Equal(key, want) {
			t.Errorf("an offer under %s is proven under %x, %v; want %x", name, key, keyErr, want)
		}
	}
	if _, err = offerKey(t.Context(), link.LocateHash(used.Secret)); !errors.Is(err, auth.ErrNoOfferKey) {
		t.Errorf("an offer under a used link's hash gave %v; want auth.ErrNoOfferKey", err)
	}
}

// fixedSecrets is a TURN secret kept in memory.
type fixedSecrets struct{ value secrets.Value }

func (kept fixedSecrets) Open(context.Context, secrets.Name) (secrets.Value, error) {
	return kept.value, nil
}

func (fixedSecrets) Set(context.Context, secrets.Name, secrets.Value, *int64) error { return nil }

// The core hands its discovery its own relay, over UDP and TCP, with credentials of the found kind
// the relay admits; a core that relays nothing hands none (the discovery protocol's §2.4).
func TestRelayForFound_HandsTheDiscoveryTheCoresRelayWithFreshCredentials(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.DiscardHandler)
	issuer := &turn.Issuer{Address: "11.22.44.146:3478", Secrets: fixedSecrets{value: secrets.NewValue("s")}}
	servers := relayForFound(issuer, logger)(t.Context())
	if len(servers) != 1 || len(servers[0].URLs) != 2 ||
		servers[0].URLs[0] != "turn:11.22.44.146:3478?transport=udp" ||
		!strings.HasSuffix(servers[0].Username, ":found") || servers[0].Credential == "" {
		t.Errorf("the relay handed the discovery is %+v; want its addresses with found credentials", servers)
	}
	if none := relayForFound(nil, logger)(t.Context()); len(none) != 0 {
		t.Errorf("a core that relays nothing handed %+v", none)
	}
}

// The relay a server runs hands members credentials that last as long as its setting says, and is
// reached at the public address and the listen's port (the constants sweep, 2026-10-20).
func TestNewICEIssuer_ReadsTheServersSettingAndIsAtThePublicAddress(t *testing.T) {
	t.Parallel()
	var config Config
	config.TURN.PublicAddress, config.TURN.Listen = "11.22.44.146", ":3478"
	var opened openedServices
	opened.settings = settingstest.Seeded(t, storetest.OpenMigrated(t))
	settingstest.Set(t, opened.settings, settings.VoiceRelayCredentialHours, "3")
	issuer := newICEIssuer(config, opened, slog.New(slog.DiscardHandler))
	if issuer == nil || issuer.Address != "11.22.44.146:3478" || issuer.Settings == nil {
		t.Fatalf("the relay's issuer is %+v; want one at 11.22.44.146:3478 with the server's settings", issuer)
	}
	if hours, err := issuer.Settings.Duration(t.Context(), settings.VoiceRelayCredentialHours); err != nil ||
		hours != 3*time.Hour {
		t.Errorf("the issuer reads %s, %v; want the 3 hours set", hours, err)
	}
}

// A server with no relay still hands Watch Together's members its STUN servers, so a listener who has
// not pressed talk offers an address someone outside the house can reach (D406).
func TestNewICEIssuer_HandsMembersTheSTUNServersWithNoRelay(t *testing.T) {
	t.Parallel()
	var config Config
	config.WebRTC.STUNServers = []string{"stun:stun.example.org:3478"}
	var opened openedServices
	opened.settings = settingstest.Seeded(t, storetest.OpenMigrated(t))
	issuer := newICEIssuer(config, opened, slog.New(slog.DiscardHandler))
	now := time.Date(2026, 11, 2, 20, 0, 0, 0, time.UTC)
	servers, err := issuer.ICEServersFor(t.Context(), turn.Membership{SessionID: 1, MemberID: 2}, now)
	if err != nil || len(servers) != 1 || strings.Join(servers[0].URLs, " ") != "stun:stun.example.org:3478" {
		t.Errorf("a server with no relay hands its members %+v (%v); want its STUN server", servers, err)
	}
	if found := relayForFound(issuer, slog.New(slog.DiscardHandler))(t.Context()); len(found) != 0 {
		t.Errorf("a server with no relay hands its discovery %+v; want nothing, STUN being the members'", found)
	}
}

// Each heartbeat sends the addresses the config names beside the router's mappings, as configured
// candidates (the protocol's §2.4).
func TestHeartbeatCandidates_SendTheConfiguredAddressesBesideTheMapped(t *testing.T) {
	t.Parallel()
	var config Config
	config.Discovery.ConfiguredAddresses = []string{"11.22.44.146:443"}
	mapped := discoveryproto.Candidate{Address: "203.0.113.9", Port: 51234, Source: discoveryproto.SourceMapped}
	mappings := func() []discoveryproto.Candidate { return []discoveryproto.Candidate{mapped} }
	got := heartbeatCandidates(config, mappings)()
	if !slices.Contains(got, mapped) || !slices.Contains(got, discoveryproto.Candidate{
		Address: "11.22.44.146", Port: 443, Source: discoveryproto.SourceConfigured,
	}) {
		t.Errorf("the heartbeat's candidates are %+v; want the mapped one and the configured one", got)
	}
}

// A core in a container sends none of the addresses its interfaces hold, only what is mapped and
// configured; one on its machine sends them too (the owner, 2026-10-14).
func TestHeartbeatCandidates_InAContainerSendNoInterfaceAddresses(t *testing.T) {
	t.Parallel()
	var config Config
	config.HTTPS.Listen = ":443"
	config.Discovery.ConfiguredAddresses = []string{"11.22.44.146:443"}
	none := func() []discoveryproto.Candidate { return nil }
	got := heartbeatCandidates(config, none)()
	want := []discoveryproto.Candidate{{Address: "11.22.44.146", Port: 443, Source: discoveryproto.SourceConfigured}}
	if !slices.Equal(got, want) {
		t.Errorf("a core told to send no interface addresses sends %+v; want the configured one alone", got)
	}
	config.Discovery.SendInterfaceAddresses = true
	if withInterfaces := heartbeatCandidates(config, none)(); len(withInterfaces) != len(interfaceCandidates(443))+1 {
		t.Errorf("a core told to send them sends %+v; want its interfaces' beside the configured", withInterfaces)
	}
}

// Each heartbeat sends a nonce for the discovery's outside probe that the core's own echo
// recognises, the one answer only this core can give from outside (the protocol's §2.6).
func TestDiscoveryParts_TheProbesNonceIsOneTheEchoRecognises(t *testing.T) {
	t.Parallel()
	var config Config
	var opened openedServices
	opened.reachability = newReachability(config, opened)
	parts := discoveryParts(config, opened, nil, nil, nil, slog.New(slog.DiscardHandler))
	nonce, err := parts.ProbeNonce()
	if err != nil || !opened.reachability.Recognises(nonce) {
		t.Errorf("the heartbeat's probe nonce is %q, %v; want one the echo recognises", nonce, err)
	}
}

// A pending core asks whether it is claimed as often as the server's setting says, as an admin set it
// (the owner, 2026-10-20: constants become settings).
func TestDiscoveryParts_APendingCoreAsksAsOftenAsTheServersSettingSays(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	var config Config
	var opened openedServices
	opened.settings = settingstest.Seeded(t, storetest.OpenMigrated(t))
	opened.reachability = newReachability(config, opened)
	key := settings.DiscoveryPendingCheckEverySeconds
	if err := opened.settings.Set(ctx, map[string]json.RawMessage{string(key): json.RawMessage("7")}, nil,
		false); err != nil {
		t.Fatal(err)
	}
	parts := discoveryParts(config, opened, nil, nil, nil, slog.New(slog.DiscardHandler))
	if parts.Settings == nil {
		t.Fatal("the discovery's parts have no settings; a pending core asks at the starting value")
	}
	if every, err := parts.Settings.Duration(ctx, key); err != nil || every != 7*time.Second {
		t.Errorf("the discovery reads %s = %s, %v; want the 7 s an admin set", key, every, err)
	}
}

// The port mapping asks for as long a lease as the server's setting says (the constants sweep,
// 2026-10-20).
func TestPortMappingParts_TheLeaseIsTheServersSetting(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	var config Config
	var opened openedServices
	opened.settings = settingstest.Seeded(t, storetest.OpenMigrated(t))
	settingstest.Set(t, opened.settings, settings.PortMappingLeaseSeconds, "900")
	parts := portMappingParts(config, opened, func() discovery.Status { return discovery.Status{} },
		slog.New(slog.DiscardHandler))
	if parts.Settings == nil {
		t.Fatal("the port mapping's parts have no settings; it asks at the starting values")
	}
	lease, err := parts.Settings.Duration(ctx, settings.PortMappingLeaseSeconds)
	if err != nil || lease != 15*time.Minute {
		t.Errorf("the port mapping reads a lease of %s, %v; want the 900 s set", lease, err)
	}
}

// The discovery's realm reaches the accounts, which make a person's realm key for it at their next
// password sign-in, and the realm sign-ins it passes on are checked by those keys (D297 §3).
func TestDiscoveryParts_TheRealmAndItsSignInsGoToTheAccounts(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	database := storetest.OpenMigrated(t)
	var config Config
	var opened openedServices
	opened.auth = auth.NewService(database, clocktest.NewFake(time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC)),
		rand.Reader, settingstest.Seeded(t, database))
	opened.reachability = newReachability(config, opened)
	parts := discoveryParts(config, opened, nil, nil, nil, slog.New(slog.DiscardHandler))
	parts.OnRealm("karlnas")
	karl := auth.NewUser{Name: "karl", Password: "a password", Role: auth.RoleUser}
	if _, err := opened.auth.CreateUser(ctx, karl); err != nil {
		t.Fatal(err)
	}
	person := proof.RealmPerson("karlnas", "karl")
	key := proof.RealmKey("karlnas", "karl", "a password")
	got, err := parts.RealmSignIn(ctx, person, proof.RealmAsk(key, "karlnas", person))
	if err != nil || !bytes.Equal(got, key) {
		t.Errorf("karl's realm sign-in through the parts = %x, %v; want his key", got, err)
	}
}

// A live hand-off's code proves the offers the discovery passes on for its person, so the client
// origin it was handed to reaches the core over WebRTC (D308 §4).
func TestDiscoveryParts_AHandOffsCodeProvesOffersForItsPerson(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	database := storetest.OpenMigrated(t)
	var config Config
	var opened openedServices
	opened.auth = auth.NewService(database, clocktest.NewFake(time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)),
		rand.Reader, settingstest.Seeded(t, database))
	opened.reachability = newReachability(config, opened)
	parts := discoveryParts(config, opened, nil, nil, nil, slog.New(slog.DiscardHandler))
	parts.OnRealm("karlnas")
	karl := auth.NewUser{Name: "karl", Password: "a password", Role: auth.RoleUser}
	if _, err := opened.auth.CreateUser(ctx, karl); err != nil {
		t.Fatal(err)
	}
	code, err := opened.auth.HandOff(ctx, "karl", "a password", "192.0.2.1",
		auth.NewDevice{Name: "Firefox on Linux", Platform: "web"})
	if err != nil {
		t.Fatal(err)
	}
	if parts.HandOffOfferKeys == nil {
		t.Fatal("the parts give the discovery no hand-off keys")
	}
	keys := parts.HandOffOfferKeys(proof.RealmPerson("karlnas", "karl"))
	if len(keys) != 1 || !bytes.Equal(keys[0], proof.HandOffOfferKey(code)) {
		t.Errorf("karl's hand-off keys through the parts are %d; want his live code's", len(keys))
	}
}
