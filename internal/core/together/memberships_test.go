// SPDX-License-Identifier: AGPL-3.0-or-later

package together

import (
	"errors"
	"slices"
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

// joinSignedIn admits partner, signed in, by a link of their own.
func (stage sessionStage) joinSignedIn(t *testing.T, partner *Person) Joined {
	t.Helper()
	made, err := stage.hub.AddLink(stage.opened.SessionID, stage.owner.UserID, "my partner", Rights{})
	if err != nil {
		t.Fatal(err)
	}
	joined, err := stage.hub.Join(made.Secret, partner.Name, partner, Leaving{})
	if err != nil {
		t.Fatal(err)
	}
	return joined
}

// Having a joined session back on one device cuts off no other: the laptop's credential still admits
// its guest after the phone asked, and the tablet after it (D267 §3).
func TestMine_HavingTheSessionBackOnOneDeviceCutsOffNoOther(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	partner := Person{UserID: 2, Name: "partner"}
	laptop := stage.joinSignedIn(t, &partner).MembershipCredential
	phone, err := stage.hub.Mine(partner.UserID)
	if err != nil || len(phone) != 1 {
		t.Fatalf("the phone asked and had %+v, %v", phone, err)
	}
	tablet, err := stage.hub.Mine(partner.UserID)
	if err != nil || len(tablet) != 1 {
		t.Fatalf("the tablet asked and had %+v, %v", tablet, err)
	}
	for name, credential := range map[string]string{
		"the laptop's": laptop, "the phone's": phone[0].MembershipCredential,
		"the tablet's": tablet[0].MembershipCredential,
	} {
		if _, _, isMember := stage.hub.MemberOf(credential); !isMember {
			t.Errorf("%s credential no longer admits the guest", name)
		}
	}
}

// The session a person hosts stays on the device that started it (D442 §1): their other devices
// asking are told it is on, and on which of their devices, and given no credential, so the
// starting device's credential, never pushed out by theirs, still admits it however often they ask,
// as it does that device's page reloaded.
func TestMine_TheHostsSessionStaysOnTheDeviceThatStartedIt(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	signedIn := &SignedIn{Person: stage.owner, DeviceID: 7}
	if _, err := stage.hub.Attach(stage.opened.MembershipCredential, RoleBoth, true, signedIn, testTuning,
		(&inbox{}).send); err != nil {
		t.Fatal(err)
	}
	for range credentialsKeptPerMember + 1 {
		hosted, err := stage.hub.Mine(stage.owner.UserID)
		if err != nil || len(hosted) != 1 || !hosted[0].IsHost || hosted[0].MembershipCredential != "" ||
			!slices.Equal(hosted[0].HostDevices, []int64{7}) {
			t.Fatalf("another device of the host's asked and had %+v, %v; want the session, on device 7, "+
				"with no credential", hosted, err)
		}
	}
	reloaded := &SignedIn{Person: stage.owner, DeviceID: 7}
	if _, err := stage.hub.Attach(stage.opened.MembershipCredential, RoleBoth, true, reloaded, testTuning,
		(&inbox{}).send); err != nil {
		t.Errorf("the starting device's page reloaded = %v; want its credential still to admit it", err)
	}
}

// Moving the session here makes the asking device the host's one device (D442 §2, D267 §3): its new
// credential admits the host, the device the session was on is told it moved away and is cut off, a
// guest stays, and no one else moves it.
func TestHostHere_MakesTheAskingDeviceTheHostsOne(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	_, hostID, _ := stage.hub.MemberOf(stage.opened.MembershipCredential)
	_, laptop := stage.attach(t, stage.opened.MembershipCredential, RoleBoth)
	guest := stage.join(t)
	_, guestsBox := stage.attach(t, guest.MembershipCredential, RoleVideo)
	if _, err := stage.hub.HostHere(stage.opened.SessionID, 99); !errors.Is(err, ErrNotHost) {
		t.Errorf("someone else moving the session = %v; want ErrNotHost", err)
	}
	phone, err := stage.hub.HostHere(stage.opened.SessionID, stage.owner.UserID)
	if err != nil {
		t.Fatal(err)
	}
	if _, memberID, isMember := stage.hub.MemberOf(phone); !isMember || memberID != hostID {
		t.Errorf("the phone's credential is member %d (%v); want the host", memberID, isMember)
	}
	if ended := laptop.last(MessageEnded); ended.Reason != "movedAway" {
		t.Errorf("the laptop was told %+v; want ended, movedAway", ended)
	}
	if _, _, isMember := stage.hub.MemberOf(stage.opened.MembershipCredential); isMember {
		t.Error("the laptop's credential still admits the host")
	}
	if guestsBox.count(MessageEnded) != 0 {
		t.Errorf("the guest was told %+v; want them still in", guestsBox.last(MessageEnded))
	}
}

// A member keeps the newest credentialsKeptPerMember of their devices' credentials: a page reloaded
// again and again does not add without end, and the oldest device asks again.
func TestMine_KeepsAMembersNewestCredentials(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	partner := Person{UserID: 2, Name: "partner"}
	first := stage.joinSignedIn(t, &partner).MembershipCredential
	for range credentialsKeptPerMember {
		if _, err := stage.hub.Mine(partner.UserID); err != nil {
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
