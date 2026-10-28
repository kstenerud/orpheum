// SPDX-License-Identifier: AGPL-3.0-or-later

package together

import (
	"bytes"
	"slices"
	"testing"
	"time"

	discoverylink "github.com/kstenerud/orpheum/discovery/link"
	"github.com/kstenerud/orpheum/discovery/proof"
)

// An unused link is found by its locate hash, its offers proven under its key, and lives at the
// discovery to the end of tomorrow; once used it locates nothing, and its person's device is found by
// the locator it was handed, its offers proven under its credential (the protocol's §3.1, §2.5; A12).
func TestLiveLinks_AreTheUnusedLinksAndEachMembershipDeviceIsFoundByItsLocator(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	changes := 0
	stage.hub.OnLinksChanged(func() { changes++ })
	made, err := stage.hub.AddLink(stage.opened.SessionID, stage.owner.UserID, "my partner", Rights{})
	if err != nil {
		t.Fatal(err)
	}
	hash := discoverylink.LocateHash(made.Secret)
	tomorrowsEnd := time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC) // the stage's clock is 2026-10-05 20:00
	if live := stage.hub.LiveLinks(); !slices.Equal(live, []LiveLink{{LocateHash: hash, ExpiresAt: tomorrowsEnd}}) {
		t.Errorf("the live links are %+v; want the unused link's hash, until the end of tomorrow", live)
	}
	if key, found := stage.hub.OfferKey(hash); !found || !bytes.Equal(key, discoverylink.OfferKey(made.Secret)) {
		t.Errorf("an offer by the link is proven under %x (%v); want the link's offer key", key, found)
	}
	if locators := stage.hub.LiveLocators(); len(locators) != 0 {
		t.Errorf("before anyone joined the locators are %v; want none, the host's device having its own", locators)
	}
	joined, err := stage.hub.Join(made.Secret, "Pat", nil, Leaving{})
	if err != nil {
		t.Fatal(err)
	}
	if live := stage.hub.LiveLinks(); len(live) != 0 {
		t.Errorf("a used link is still live: %+v", live)
	}
	if _, found := stage.hub.OfferKey(hash); found {
		t.Error("an offer by a used link is still proven")
	}
	locator := discoverylink.LocatorHash(joined.Locator)
	if locators := stage.hub.LiveLocators(); !slices.Equal(locators, []string{locator}) {
		t.Errorf("the locators are %v; want the joined device's, %s", locators, locator)
	}
	key, found := stage.hub.OfferKey(locator)
	if !found || !bytes.Equal(key, proof.SessionOfferKey(joined.MembershipCredential)) {
		t.Errorf("an offer by the device's locator is proven under %x (%v); want its credential's hash", key, found)
	}
	if changes != 2 {
		t.Errorf("the discovery was told of %d changes; want 2, the link made and the locator handed", changes)
	}
}

// A member who leaves takes their devices' locators with them: an offer by one proves nothing.
func TestLiveLocators_GoWithAMemberWhoLeaves(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	joined := stage.join(t)
	if err := stage.hub.Leave(joined.MembershipCredential); err != nil {
		t.Fatal(err)
	}
	if locators := stage.hub.LiveLocators(); len(locators) != 0 {
		t.Errorf("a member who left still has locators: %v", locators)
	}
	if _, found := stage.hub.OfferKey(discoverylink.LocatorHash(joined.Locator)); found {
		t.Error("an offer by the locator of a member who left is still proven")
	}
}

// Ending a session drops its links and its members' locators; a signed-in member's other device
// has a locator of its own with the membership back.
func TestLiveLocators_EndWithTheSessionAndEachDeviceHasItsOwn(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	made, err := stage.hub.AddLink(stage.opened.SessionID, stage.owner.UserID, "my partner", Rights{})
	if err != nil {
		t.Fatal(err)
	}
	partner := Person{UserID: 2, Name: "partner"}
	joined, err := stage.hub.Join(made.Secret, "Pat", &partner, Leaving{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = stage.hub.AddLink(stage.opened.SessionID, stage.owner.UserID, "a friend", Rights{}); err != nil {
		t.Fatal(err)
	}
	other, err := stage.hub.Mine(partner.UserID)
	if err != nil || len(other) != 1 || other[0].Locator == "" || other[0].Locator == joined.Locator {
		t.Fatalf("the member's other device has %+v, %v; want a locator of its own", other, err)
	}
	if hosted, mineErr := stage.hub.Mine(stage.owner.UserID); mineErr != nil || hosted[0].Locator != "" {
		t.Errorf("the host's device was handed a locator, %+v, %v; want none, it has its session's", hosted, mineErr)
	}
	if locators := stage.hub.LiveLocators(); len(locators) != 2 {
		t.Errorf("the locators are %v; want the member's two devices'", locators)
	}
	if err = stage.hub.End(stage.opened.SessionID, stage.owner.UserID); err != nil {
		t.Fatal(err)
	}
	if live, locators := stage.hub.LiveLinks(), stage.hub.LiveLocators(); len(live) != 0 || len(locators) != 0 {
		t.Errorf("an ended session left links %+v and locators %v", live, locators)
	}
}
