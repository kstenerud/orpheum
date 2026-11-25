// SPDX-License-Identifier: AGPL-3.0-or-later

package together

import (
	"context"
	"crypto/rand"
	"log/slog"
	"slices"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
)

// partsHeard is a hub telling each person whose part changed into told.
type partsHeard struct {
	hub   *Hub
	clock *clocktest.Fake
	told  []int64
}

func newPartsHeard() *partsHeard {
	fakeClock := clocktest.NewFake(time.Date(2026, 11, 25, 20, 0, 0, 0, time.UTC))
	heard := &partsHeard{clock: fakeClock}
	heard.hub = NewHub(fakeClock, rand.Reader, func(context.Context, int64, int64) bool { return true },
		slog.New(slog.DiscardHandler))
	heard.hub.OnPartChanged(func(userID int64) { heard.told = append(heard.told, userID) })
	return heard
}

// expect fails the test unless exactly want were told since it was last asked, after what, and
// forgets them.
func (heard *partsHeard) expect(t *testing.T, what string, want ...int64) {
	t.Helper()
	told := heard.told
	heard.told = nil
	if !slices.Equal(told, want) {
		t.Errorf("after %s the hub told the parts of %v; want %v", what, told, want)
	}
}

// Each signed-in person's pages show their part in a session, and learn it changed, whatever
// changed it, by the hub alone telling each person whose part changed as its lock is let go: their
// start, a device of theirs attached or gone, their join signed in, its move to another device, their
// leave, the host's end; never those whose part stayed as it was (the owner's staging walk,
// 2026-11-25: a page open before showed "Start a session" until reloaded).
func TestOnPartChanged_TellsEachPersonWhosePartChangedByAnyPath(t *testing.T) {
	t.Parallel()
	heard := newPartsHeard()
	hub := heard.hub
	owner, partner := Person{UserID: 1, Name: "owner"}, Person{UserID: 2, Name: "partner"}
	opened, err := hub.Open(owner)
	if err != nil {
		t.Fatal(err)
	}
	heard.expect(t, "the owner's start", owner.UserID)
	laptop, err := hub.Attach(opened.MembershipCredential, RoleBoth, false,
		&SignedIn{Person: owner, DeviceID: 10}, testTuning, (&inbox{}).send)
	if err != nil {
		t.Fatal(err)
	}
	heard.expect(t, "the owner's laptop attached", owner.UserID)
	made, err := hub.AddLink(opened.SessionID, owner.UserID, "my partner", Rights{})
	if err != nil {
		t.Fatal(err)
	}
	joined, err := hub.Join(made.Secret, partner.Name, &partner, Leaving{})
	if err != nil {
		t.Fatal(err)
	}
	heard.expect(t, "the partner's join, signed in", partner.UserID)
	if _, err = hub.Attach(joined.MembershipCredential, RoleVideo, false,
		&SignedIn{Person: partner, DeviceID: 20}, testTuning, (&inbox{}).send); err != nil {
		t.Fatal(err)
	}
	heard.expect(t, "the partner's phone attached", partner.UserID)
	moved, _, err := hub.GuestHere(opened.SessionID, partner.UserID)
	if err != nil {
		t.Fatal(err)
	}
	heard.expect(t, "the partner's part moved to another device", partner.UserID)
	if err = hub.Leave(moved.MembershipCredential); err != nil {
		t.Fatal(err)
	}
	heard.expect(t, "the partner's leave", partner.UserID)
	phone, _, err := hub.HostHere(opened.SessionID, owner.UserID)
	if err != nil {
		t.Fatal(err)
	}
	heard.expect(t, "the owner's session moved from the laptop", owner.UserID)
	hub.Detach(laptop, GoneClosed)
	heard.expect(t, "the laptop, cut off, closing")
	attached, err := hub.Attach(phone, RoleBoth, false, &SignedIn{Person: owner, DeviceID: 11}, testTuning,
		(&inbox{}).send)
	if err != nil {
		t.Fatal(err)
	}
	heard.expect(t, "the owner's phone attached", owner.UserID)
	hub.Detach(attached, GoneSilent)
	heard.expect(t, "the owner's phone gone silent", owner.UserID)
	if err := hub.End(opened.SessionID, owner.UserID); err != nil {
		t.Fatal(err)
	}
	heard.expect(t, "the host's end", owner.UserID)
}

// A session's end tells each signed-in person in it once, however it ends: the host starting another
// tells the host (whose part is now in the new one) and the guest; an idle end tells them too. A
// guest by link, signed out, has no part to tell.
func TestOnPartChanged_TellsEveryoneInASessionAsItEnds(t *testing.T) {
	t.Parallel()
	heard := newPartsHeard()
	hub := heard.hub
	owner, partner := Person{UserID: 1, Name: "owner"}, Person{UserID: 2, Name: "partner"}
	first, err := hub.Open(owner)
	if err != nil {
		t.Fatal(err)
	}
	for _, person := range []*Person{&partner, nil} {
		made, linkErr := hub.AddLink(first.SessionID, owner.UserID, "a link", Rights{})
		if linkErr != nil {
			t.Fatal(linkErr)
		}
		if _, joinErr := hub.Join(made.Secret, "a guest", person, Leaving{}); joinErr != nil {
			t.Fatal(joinErr)
		}
	}
	heard.expect(t, "the start and the joins", owner.UserID, partner.UserID)
	second, err := hub.Open(owner)
	if err != nil {
		t.Fatal(err)
	}
	heard.expect(t, "the host starting another session", owner.UserID, partner.UserID)
	made, err := hub.AddLink(second.SessionID, owner.UserID, "a link", Rights{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := hub.Join(made.Secret, "a guest", &partner, Leaving{}); err != nil {
		t.Fatal(err)
	}
	heard.expect(t, "the partner's join of the second", partner.UserID)
	heard.clock.Advance(StartingIdleEnd)
	if ended := hub.EndStale(startingLifetimes()); len(ended) != 1 {
		t.Fatalf("the idle end ended %v; want the second session", ended)
	}
	heard.expect(t, "the idle end", owner.UserID, partner.UserID)
}
