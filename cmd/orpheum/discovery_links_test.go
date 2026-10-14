// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bytes"
	"crypto/rand"
	"errors"
	"log/slog"
	"slices"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/discovery/link"
	"github.com/kstenerud/orpheum/discovery/proof"
	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/core/together"
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
