// SPDX-License-Identifier: AGPL-3.0-or-later

package together

import (
	"testing"
)

// A person's sessions follow them to any device they sign in on: the host's, and one they joined
// signed in, each with a credential for the device that asks; a guest who joined signed out has none
// (D267 §2).
func TestMine_IsEverySessionAPersonIsInAsHostOrSignedInMember(t *testing.T) {
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
	stage.join(t) // another guest, signed out
	hosted, err := stage.hub.Mine(stage.owner.UserID)
	if err != nil || len(hosted) != 1 || !hosted[0].IsHost || hosted[0].SessionID != stage.opened.SessionID {
		t.Fatalf("the host's sessions are %+v, %v; want theirs, as host", hosted, err)
	}
	guest, err := stage.hub.Mine(partner.UserID)
	if err != nil || len(guest) != 1 || guest[0].IsHost || guest[0].MemberID != joined.MemberID {
		t.Fatalf("the signed-in guest's sessions are %+v, %v; want the one they joined, as member %d",
			guest, err, joined.MemberID)
	}
	if none, err := stage.hub.Mine(99); err != nil || len(none) != 0 {
		t.Errorf("someone in no session is in %+v, %v; want none", none, err)
	}
	// The credential is this device's: it says hello, as the member.
	if _, memberID, isMember := stage.hub.MemberOf(guest[0].MembershipCredential); !isMember ||
		memberID != joined.MemberID {
		t.Errorf("the device's credential is member %d (%v), want %d", memberID, isMember, joined.MemberID)
	}
}

// Having the session back on one device cuts off no other: the laptop's credential still admits it
// after the phone asked, and the tablet after it (D267 §3).
func TestMine_HavingTheSessionBackOnOneDeviceCutsOffNoOther(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	laptop := stage.opened.MembershipCredential
	phone, err := stage.hub.Mine(stage.owner.UserID)
	if err != nil || len(phone) != 1 {
		t.Fatalf("the phone asked and had %+v, %v", phone, err)
	}
	tablet, err := stage.hub.Mine(stage.owner.UserID)
	if err != nil || len(tablet) != 1 {
		t.Fatalf("the tablet asked and had %+v, %v", tablet, err)
	}
	for name, credential := range map[string]string{
		"the laptop's": laptop, "the phone's": phone[0].MembershipCredential,
		"the tablet's": tablet[0].MembershipCredential,
	} {
		if _, _, isMember := stage.hub.MemberOf(credential); !isMember {
			t.Errorf("%s credential no longer admits the host", name)
		}
	}
}

// A member keeps the newest credentialsKeptPerMember of their devices' credentials: a page reloaded
// again and again does not add without end, and the oldest device asks again.
func TestMine_KeepsAMembersNewestCredentials(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	first := stage.opened.MembershipCredential
	for range credentialsKeptPerMember {
		if _, err := stage.hub.Mine(stage.owner.UserID); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, isMember := stage.hub.MemberOf(first); isMember {
		t.Errorf("the oldest of %d credentials still admits", credentialsKeptPerMember+1)
	}
}

// The host sees each link's secret, waiting or used, to pass it on again: a page reloaded before
// it was shared lost it, and a guest comes back by it on the browser that joined (D267 §5).
func TestDetailsOf_ShowsTheHostEachLinkToPassOnAgain(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	waiting, err := stage.hub.AddLink(stage.opened.SessionID, stage.owner.UserID, "a cousin", Rights{})
	if err != nil {
		t.Fatal(err)
	}
	used, err := stage.hub.AddLink(stage.opened.SessionID, stage.owner.UserID, "Grandma", Rights{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = stage.hub.Join(used.Secret, "Gran", nil, Leaving{}); err != nil {
		t.Fatal(err)
	}
	details, err := stage.hub.DetailsOf(stage.opened.SessionID, stage.owner.UserID)
	if err != nil {
		t.Fatal(err)
	}
	secrets := map[int64]string{}
	for _, each := range details.Links {
		secrets[each.ID] = each.Secret
	}
	if secrets[waiting.LinkID] != waiting.Secret || secrets[used.LinkID] != used.Secret {
		t.Errorf("the host sees the links' secrets %v; want the waiting %q and the used %q",
			secrets, waiting.Secret, used.Secret)
	}
}
