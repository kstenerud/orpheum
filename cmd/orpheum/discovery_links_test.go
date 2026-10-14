// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bytes"
	"context"
	"crypto/rand"
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
	"github.com/kstenerud/orpheum/internal/core/secrets"
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
	joined, err := sessions.Join(used.Secret, "Pat", nil)
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
