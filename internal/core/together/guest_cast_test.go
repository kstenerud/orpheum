// SPDX-License-Identifier: AGPL-3.0-or-later

package together

import (
	"errors"
	"testing"

	discoverylink "github.com/kstenerud/orpheum/discovery/link"
)

// A Google Cast device a Watch Together guest by link cast to takes their part in the session as
// another device of theirs: its own credential names the guest's member, viewing with their rights
// alone, its locator finds that member, and it follows the group, never leading it (D518 §1).
func TestGuestCastPart_IsAnotherDeviceOfTheGuestsWithTheirRights(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	host, hostBox := stage.attach(t, stage.opened.MembershipCredential, RoleVideo)
	made, err := stage.hub.AddLink(stage.opened.SessionID, stage.owner.UserID, "a friend", Rights{CanSeek: true})
	if err != nil {
		t.Fatal(err)
	}
	guest, err := stage.hub.Join(made.Secret, "Pat", nil, Leaving{})
	if err != nil {
		t.Fatal(err)
	}
	part, err := stage.hub.GuestCastPart(guest.SessionID, guest.MemberID)
	if err != nil {
		t.Fatal(err)
	}
	if part.MembershipCredential == guest.MembershipCredential || part.Locator == guest.Locator {
		t.Error("the Cast device was given the guest's own credential or locator; want its own")
	}
	viewer, isViewer := stage.hub.ViewerOf(part.MembershipCredential)
	if !isViewer || viewer.SessionID != guest.SessionID || viewer.MemberID != guest.MemberID ||
		viewer.Rights != (Rights{CanSeek: true}) {
		t.Errorf("the Cast device views as %+v (%v); want the guest's member with their rights", viewer, isViewer)
	}
	_, hostMemberID, _ := stage.hub.MemberOf(stage.opened.MembershipCredential)
	if _, err = stage.hub.GuestCastPart(guest.SessionID, hostMemberID); !errors.Is(err, ErrNoMember) {
		t.Errorf("a part for the host = %v; want ErrNoMember, the host casting as themselves", err)
	}
	stage.command(t, host, CommandChangeItem, 7)
	stage.ready(t, host, false)
	receiver, _ := stage.attach(t, part.MembershipCredential, RoleVideo)
	stage.report(t, receiver, 0, false, true)
	for _, each := range hostBox.last(MessageRoster).Roster {
		for _, attached := range each.Devices {
			if attached.ID == receiver.DeviceID && attached.IsLeader {
				t.Error("the guest's Cast device leads the session; want it a follower")
			}
		}
	}
}

// The Cast device's locator finds the guest's member, whose waiting grants its offers are proven by;
// no member's locator finds no one (D518 §1).
func TestGuestCastPart_ItsLocatorFindsTheGuestsMember(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	guest := stage.join(t)
	part, err := stage.hub.GuestCastPart(guest.SessionID, guest.MemberID)
	if err != nil {
		t.Fatal(err)
	}
	sessionID, memberID, isFound := stage.hub.MemberOfLocator(discoverylink.LocatorHash(part.Locator))
	if !isFound || sessionID != guest.SessionID || memberID != guest.MemberID {
		t.Errorf("the Cast device's locator finds session %d, member %d (%v); want the guest's",
			sessionID, memberID, isFound)
	}
	if _, _, isFound = stage.hub.MemberOfLocator(discoverylink.LocatorHash("no one's")); isFound {
		t.Error("no member's locator found a member")
	}
}

// The Cast device's part ends as the guest's membership does: removed, the device is told it was, its
// credential admits nothing and its locator finds no one, and no part is given for them again; once
// the session ends, none is given in it (D518 §1, D389).
func TestGuestCastPart_EndsAsTheMembershipEnds(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	guest := stage.join(t)
	part, err := stage.hub.GuestCastPart(guest.SessionID, guest.MemberID)
	if err != nil {
		t.Fatal(err)
	}
	_, castBox := stage.attach(t, part.MembershipCredential, RoleVideo)
	if err = stage.hub.Eject(stage.opened.SessionID, stage.owner.UserID, guest.MemberID); err != nil {
		t.Fatal(err)
	}
	if ended := castBox.last(MessageEnded); ended.Reason != "ejected" {
		t.Errorf("the removed guest's Cast device was told %+v; want ended, ejected", ended)
	}
	if _, isViewer := stage.hub.ViewerOf(part.MembershipCredential); isViewer {
		t.Error("the removed guest's Cast device's credential still admits them")
	}
	if _, _, isFound := stage.hub.MemberOfLocator(discoverylink.LocatorHash(part.Locator)); isFound {
		t.Error("the removed guest's Cast device's locator still finds them")
	}
	if _, err = stage.hub.GuestCastPart(guest.SessionID, guest.MemberID); !errors.Is(err, ErrNoMember) {
		t.Errorf("a part for a removed guest = %v; want ErrNoMember", err)
	}
	other := stage.join(t)
	if err = stage.hub.End(stage.opened.SessionID, stage.owner.UserID); err != nil {
		t.Fatal(err)
	}
	if _, err = stage.hub.GuestCastPart(other.SessionID, other.MemberID); !errors.Is(err, ErrNoSession) {
		t.Errorf("a part in an ended session = %v; want ErrNoSession", err)
	}
}

// A guest has one cast's credential at a time (D519 §2): a new cast's replaces the last's, whose
// receiver is told its place moved away and admits nothing more, its locator finding no one, while the
// guest's own browser plays on; and casting again and again never pushes the browser's credential out
// of the newest the member keeps.
func TestGuestCastPart_ANewCastReplacesTheGuestsLastCastsCredential(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	guest := stage.join(t)
	_, browserBox := stage.attach(t, guest.MembershipCredential, RoleVideo)
	first, err := stage.hub.GuestCastPart(guest.SessionID, guest.MemberID)
	if err != nil {
		t.Fatal(err)
	}
	firstReceiver, firstBox := stage.attach(t, first.MembershipCredential, RoleVideo)
	second, err := stage.hub.GuestCastPart(guest.SessionID, guest.MemberID)
	if err != nil {
		t.Fatal(err)
	}
	if ended := firstBox.last(MessageEnded); ended.Reason != "movedAway" {
		t.Errorf("the replaced cast's receiver was told %+v; want ended, movedAway", ended)
	}
	if _, isViewer := stage.hub.ViewerOf(first.MembershipCredential); isViewer {
		t.Error("the replaced cast's credential still admits the guest")
	}
	if _, _, isFound := stage.hub.MemberOfLocator(discoverylink.LocatorHash(first.Locator)); isFound {
		t.Error("the replaced cast's locator still finds the guest")
	}
	for _, each := range browserBox.last(MessageRoster).Roster {
		for _, attached := range each.Devices {
			if attached.ID == firstReceiver.DeviceID {
				t.Error("the roster still lists the replaced cast's receiver")
			}
		}
	}
	if _, isViewer := stage.hub.ViewerOf(second.MembershipCredential); !isViewer {
		t.Error("the new cast's credential does not admit the guest")
	}
	if browserBox.count(MessageEnded) != 0 {
		t.Errorf("the guest's own browser was told %+v; want it playing on", browserBox.last(MessageEnded))
	}
	for range credentialsKeptPerMember + 4 {
		if _, err = stage.hub.GuestCastPart(guest.SessionID, guest.MemberID); err != nil {
			t.Fatal(err)
		}
	}
	if _, isViewer := stage.hub.ViewerOf(guest.MembershipCredential); !isViewer {
		t.Error("casting again and again pushed the guest's own browser's credential out")
	}
}

// An AirPlay grant of a guest's holds their membership by a credential of its own while they are in
// (D519 §1): it names the guest's member, adds no locator for a discovery to find, and, as a
// Chromecast's part does, replaces the guest's last cast's credential and is replaced by their next
// (D519 §2); the host has none, and a removed guest's admits nothing.
func TestGuestAirPlayHold_HoldsTheGuestsMembershipAsTheirOneCast(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	guest := stage.join(t)
	locatorsBefore := len(stage.hub.LiveLocators())
	part, err := stage.hub.GuestCastPart(guest.SessionID, guest.MemberID)
	if err != nil {
		t.Fatal(err)
	}
	_, castBox := stage.attach(t, part.MembershipCredential, RoleVideo)
	hold, err := stage.hub.GuestAirPlayHold(guest.SessionID, guest.MemberID)
	if err != nil {
		t.Fatal(err)
	}
	viewer, isViewer := stage.hub.ViewerHeldBy(hold)
	if !isViewer || viewer.SessionID != guest.SessionID || viewer.MemberID != guest.MemberID {
		t.Errorf("the hold views as %+v (%v); want the guest's member", viewer, isViewer)
	}
	if ended := castBox.last(MessageEnded); ended.Reason != "movedAway" {
		t.Errorf("the Chromecast the AirPlay grant replaced was told %+v; want ended, movedAway", ended)
	}
	if locators := len(stage.hub.LiveLocators()); locators != locatorsBefore {
		t.Errorf("the hold left %d locators; want the %d before the casts, it adding none", locators, locatorsBefore)
	}
	if _, err = stage.hub.GuestAirPlayHold(guest.SessionID, guest.MemberID); err != nil {
		t.Fatal(err)
	}
	if _, isViewer = stage.hub.ViewerHeldBy(hold); isViewer {
		t.Error("the hold the guest's next cast replaced still admits them")
	}
	_, hostMemberID, _ := stage.hub.MemberOf(stage.opened.MembershipCredential)
	if _, err = stage.hub.GuestAirPlayHold(guest.SessionID, hostMemberID); !errors.Is(err, ErrNoMember) {
		t.Errorf("a hold for the host = %v; want ErrNoMember, the host AirPlaying as themselves", err)
	}
	last, err := stage.hub.GuestAirPlayHold(guest.SessionID, guest.MemberID)
	if err != nil {
		t.Fatal(err)
	}
	if err = stage.hub.Eject(stage.opened.SessionID, stage.owner.UserID, guest.MemberID); err != nil {
		t.Fatal(err)
	}
	if _, isViewer = stage.hub.ViewerHeldBy(last); isViewer {
		t.Error("the removed guest's hold still admits them")
	}
}
