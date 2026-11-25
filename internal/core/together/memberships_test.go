// SPDX-License-Identifier: AGPL-3.0-or-later

package together

import (
	"errors"
	"slices"
	"testing"
)

// A person's sessions are listed on any device they sign in on: the host's, and one they joined
// signed in, each with no credential while their part is on a device (D267 §2; D555, the owner's
// point 3); a guest who joined signed out has none.
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
	if err != nil || len(guest) != 1 || guest[0].IsHost || guest[0].MemberID != joined.MemberID ||
		guest[0].MembershipCredential != "" {
		t.Fatalf("the signed-in guest's sessions are %+v, %v; want the one they joined, as member %d, "+
			"with no credential: their part is on the device that joined", guest, err, joined.MemberID)
	}
	if none, err := stage.hub.Mine(99); err != nil || len(none) != 0 {
		t.Errorf("someone in no session is in %+v, %v; want none", none, err)
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

// A signed-in guest's part stays on the device it is on (D555, the owner's point 3): their other
// devices asking, however often, are told it is on, on which of their devices, and given no
// credential, so loading a page on a second device takes nothing from the first, whose credential
// still admits it.
func TestMine_AGuestsPartStaysOnTheDeviceItIsOn(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	partner := Person{UserID: 2, Name: "partner"}
	laptop := stage.joinSignedIn(t, &partner).MembershipCredential
	if _, err := stage.hub.Attach(laptop, RoleBoth, false, &SignedIn{Person: partner, DeviceID: 7}, testTuning,
		(&inbox{}).send); err != nil {
		t.Fatal(err)
	}
	for range credentialsKeptPerMember + 1 {
		phone, err := stage.hub.Mine(partner.UserID)
		if err != nil || len(phone) != 1 || phone[0].MembershipCredential != "" ||
			!slices.Equal(phone[0].Devices, []int64{7}) {
			t.Fatalf("the phone asked and had %+v, %v; want the session, on device 7, with no credential",
				phone, err)
		}
	}
	if _, _, isMember := stage.hub.MemberOf(laptop); !isMember {
		t.Error("the laptop's credential no longer admits the guest")
	}
}

// A signed-in guest whose part holds no credential of their own, on no device, has one made for the
// device that asks, which takes it from none (D267 §2).
func TestMine_AGuestsPartOnNoDeviceIsHadBackByTheDeviceThatAsks(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	partner := Person{UserID: 2, Name: "partner"}
	joined := stage.joinSignedIn(t, &partner)
	stage.hub.mutex.Lock()
	stage.hub.sessions[stage.opened.SessionID].members[joined.MemberID].credentials = nil
	stage.hub.mutex.Unlock()
	phone, err := stage.hub.Mine(partner.UserID)
	if err != nil || len(phone) != 1 || phone[0].MembershipCredential == "" || phone[0].Locator == "" {
		t.Fatalf("the phone asked and had %+v, %v; want a credential and a locator", phone, err)
	}
	if _, memberID, isMember := stage.hub.MemberOf(phone[0].MembershipCredential); !isMember ||
		memberID != joined.MemberID {
		t.Errorf("the phone's credential is member %d (%v), want %d", memberID, isMember, joined.MemberID)
	}
}

// Taking their part over here makes the asking device a signed-in guest's one device, as HostHere does
// a host's (D555, the owner's point 3): its credential admits them, the device it was on is told it
// moved away and cut off, their cast ends, movedAway, its castee told once and its credential taken
// back (the owner, 2026-11-25), and no one else, nor one not their guest, moves it.
func TestGuestHere_MakesTheAskingDeviceTheGuestsOneAndEndsTheirCast(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	partner := Person{UserID: 2, Name: "partner"}
	joined := stage.joinSignedIn(t, &partner)
	_, laptop := stage.attach(t, joined.MembershipCredential, RoleBoth)
	cast, err := stage.hub.OpenCast(partner, joined.MembershipCredential)
	if err != nil {
		t.Fatal(err)
	}
	_, castee := stage.attach(t, cast.ReceiverCredential, RoleVideo)
	if _, _, err = stage.hub.GuestHere(stage.opened.SessionID, 99); !errors.Is(err, ErrNoMember) {
		t.Errorf("someone not in the session taking the part = %v; want ErrNoMember", err)
	}
	if _, _, err = stage.hub.GuestHere(stage.opened.SessionID, stage.owner.UserID); !errors.Is(err, ErrNoMember) {
		t.Errorf("the host taking a guest's part = %v; want ErrNoMember", err)
	}
	phone, ended, err := stage.hub.GuestHere(stage.opened.SessionID, partner.UserID)
	if err != nil {
		t.Fatal(err)
	}
	if want := []EndedCast{{CastID: cast.CastID, UserID: partner.UserID}}; !slices.Equal(ended, want) {
		t.Errorf("taking the part over ended the casts %+v; want %+v", ended, want)
	}
	if _, memberID, isMember := stage.hub.MemberOf(phone.MembershipCredential); !isMember ||
		memberID != joined.MemberID || phone.Locator == "" {
		t.Errorf("the phone's credential is member %d (%v), locator %q; want the guest's, with a locator",
			memberID, isMember, phone.Locator)
	}
	if ended := laptop.last(MessageEnded); ended.Reason != "movedAway" {
		t.Errorf("the laptop was told %+v; want ended, movedAway", ended)
	}
	if _, _, isMember := stage.hub.MemberOf(joined.MembershipCredential); isMember {
		t.Error("the laptop's credential still admits the guest")
	}
	stage.assertCastEndedMovedAway(t, castee, cast)
}

// assertCastEndedMovedAway checks that cast ended as its caster's seat moved: its castee told once,
// movedAway, and its credential admitting nothing.
func (stage sessionStage) assertCastEndedMovedAway(t *testing.T, castee *inbox, opened CastOpened) {
	t.Helper()
	if castee.count(MessageEnded) != 1 || castee.last(MessageEnded).Reason != CastMovedAway {
		t.Errorf("the castee was told %d ends, the last %+v; want one, movedAway", castee.count(MessageEnded),
			castee.last(MessageEnded))
	}
	if _, _, isMember := stage.hub.MemberOf(opened.ReceiverCredential); isMember {
		t.Error("the castee's credential still admits it")
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
			!slices.Equal(hosted[0].Devices, []int64{7}) {
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
	if _, _, err := stage.hub.HostHere(stage.opened.SessionID, 99); !errors.Is(err, ErrNotHost) {
		t.Errorf("someone else moving the session = %v; want ErrNotHost", err)
	}
	phone, _, err := stage.hub.HostHere(stage.opened.SessionID, stage.owner.UserID)
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

// Moving the session here ends the host's cast: a castee carried over could be left with no remote
// that works it, as a Chromecast cast from Chrome is from Firefox, so the seat runs on the new device
// alone, the cast ended on the one end path, movedAway, its castee told and its credential taken
// back, while the host's other devices are told they moved away (the owner, 2026-11-25).
func TestHostHere_EndsTheHostsCast(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	cast, err := stage.hub.OpenCast(stage.owner, "")
	if err != nil {
		t.Fatal(err)
	}
	_, castee := stage.attach(t, cast.ReceiverCredential, RoleVideo)
	_, laptop := stage.attach(t, stage.opened.MembershipCredential, RoleBoth)
	_, ended, err := stage.hub.HostHere(stage.opened.SessionID, stage.owner.UserID)
	if err != nil {
		t.Fatal(err)
	}
	if want := []EndedCast{{CastID: cast.CastID, UserID: stage.owner.UserID}}; !slices.Equal(ended, want) {
		t.Errorf("moving the session ended the casts %+v; want %+v", ended, want)
	}
	if ended := laptop.last(MessageEnded); ended.Reason != "movedAway" {
		t.Errorf("the laptop was told %+v; want ended, movedAway", ended)
	}
	stage.assertCastEndedMovedAway(t, castee, cast)
	if _, _, err = stage.hub.CastPosition(cast.CastID, stage.owner.UserID); err == nil {
		t.Error("the hub still holds the cast")
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
