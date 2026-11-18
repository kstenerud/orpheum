// SPDX-License-Identifier: AGPL-3.0-or-later

package together

import (
	"crypto/sha256"
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
	part, err := stage.hub.GuestCastPart(guest.SessionID, guest.MemberID, senderOf(guest.MembershipCredential))
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
	_, err = stage.hub.GuestCastPart(guest.SessionID, hostMemberID, senderOf(guest.MembershipCredential))
	if !errors.Is(err, ErrNoMember) {
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
	part, err := stage.hub.GuestCastPart(guest.SessionID, guest.MemberID, senderOf(guest.MembershipCredential))
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
	part, err := stage.hub.GuestCastPart(guest.SessionID, guest.MemberID, senderOf(guest.MembershipCredential))
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
	_, err = stage.hub.GuestCastPart(guest.SessionID, guest.MemberID, senderOf(guest.MembershipCredential))
	if !errors.Is(err, ErrNoMember) {
		t.Errorf("a part for a removed guest = %v; want ErrNoMember", err)
	}
	other := stage.join(t)
	if err = stage.hub.End(stage.opened.SessionID, stage.owner.UserID); err != nil {
		t.Fatal(err)
	}
	_, err = stage.hub.GuestCastPart(other.SessionID, other.MemberID, senderOf(other.MembershipCredential))
	if !errors.Is(err, ErrNoSession) {
		t.Errorf("a part in an ended session = %v; want ErrNoSession", err)
	}
}

// senderOf is the sending device a cast is asked for by, by its membership credential.
func senderOf(credential string) [sha256.Size]byte {
	return sha256.Sum256([]byte(credential))
}

// One device casts to one other at a time (D520 §1): the cast's credential is the sending device's, so
// a guest's phone and laptop each cast to a receiver of their own, and the phone's new cast replaces
// the phone's last alone, whose receiver is told its place moved away and admits nothing more, its
// locator finding no one, while the laptop's receiver and both browsers play on; and casting again and
// again never pushes a browser's credential out of the newest the member keeps.
func TestGuestCastPart_ADevicesNewCastReplacesThatDevicesLastCastAlone(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	partner := Person{UserID: 2, Name: "partner"}
	guest := stage.joinSignedIn(t, &partner)
	laptop := guest.MembershipCredential
	mine, err := stage.hub.Mine(partner.UserID)
	if err != nil || len(mine) != 1 {
		t.Fatalf("the phone asked and had %+v, %v", mine, err)
	}
	phone := mine[0].MembershipCredential
	_, phoneBox := stage.attach(t, phone, RoleVideo)
	_, laptopBox := stage.attach(t, laptop, RoleVideo)
	first, err := stage.hub.GuestCastPart(guest.SessionID, guest.MemberID, senderOf(phone))
	if err != nil {
		t.Fatal(err)
	}
	firstReceiver, firstBox := stage.attach(t, first.MembershipCredential, RoleVideo)
	laptops, err := stage.hub.GuestCastPart(guest.SessionID, guest.MemberID, senderOf(laptop))
	if err != nil {
		t.Fatal(err)
	}
	_, laptopsBox := stage.attach(t, laptops.MembershipCredential, RoleVideo)
	if firstBox.count(MessageEnded) != 0 {
		t.Errorf("the phone's receiver was told %+v as the laptop cast; want it playing on",
			firstBox.last(MessageEnded))
	}
	second, err := stage.hub.GuestCastPart(guest.SessionID, guest.MemberID, senderOf(phone))
	if err != nil {
		t.Fatal(err)
	}
	if ended := firstBox.last(MessageEnded); ended.Reason != "movedAway" {
		t.Errorf("the phone's replaced cast's receiver was told %+v; want ended, movedAway", ended)
	}
	if _, isViewer := stage.hub.ViewerOf(first.MembershipCredential); isViewer {
		t.Error("the phone's replaced cast's credential still admits the guest")
	}
	if _, _, isFound := stage.hub.MemberOfLocator(discoverylink.LocatorHash(first.Locator)); isFound {
		t.Error("the phone's replaced cast's locator still finds the guest")
	}
	if rosterLists(laptopBox, firstReceiver.DeviceID) {
		t.Error("the roster still lists the phone's replaced cast's receiver")
	}
	stage.wantAdmitted(t, "does not admit the guest", map[string]string{
		"the phone's new cast's credential": second.MembershipCredential,
		"the laptop's cast's credential":    laptops.MembershipCredential,
	})
	wantPlayingOn(t, map[string]*inbox{
		"the laptop's receiver": laptopsBox, "the phone": phoneBox, "the laptop": laptopBox,
	})
	for range credentialsKeptPerMember + 4 {
		if _, err = stage.hub.GuestCastPart(guest.SessionID, guest.MemberID, senderOf(phone)); err != nil {
			t.Fatal(err)
		}
	}
	stage.wantAdmitted(t, "was pushed out by casting again and again",
		map[string]string{"the phone's credential": phone, "the laptop's credential": laptop})
}

// rosterLists is whether the last roster box was sent lists device deviceID.
func rosterLists(box *inbox, deviceID int64) bool {
	for _, each := range box.last(MessageRoster).Roster {
		for _, attached := range each.Devices {
			if attached.ID == deviceID {
				return true
			}
		}
	}
	return false
}

// wantAdmitted fails the test, saying problem, for each named credential that admits no viewer.
func (stage sessionStage) wantAdmitted(t *testing.T, problem string, credentials map[string]string) {
	t.Helper()
	for name, credential := range credentials {
		if _, isViewer := stage.hub.ViewerOf(credential); !isViewer {
			t.Errorf("%s %s", name, problem)
		}
	}
}

// wantPlayingOn fails the test for each named device that was told its session ended.
func wantPlayingOn(t *testing.T, boxes map[string]*inbox) {
	t.Helper()
	for name, box := range boxes {
		if box.count(MessageEnded) != 0 {
			t.Errorf("%s was told %+v; want it playing on", name, box.last(MessageEnded))
		}
	}
}

// A device being cast to casts nothing itself (D520 §1): a Chromecast taking a guest's part is
// refused a Chromecast's part and an AirPlay grant's hold of its own, while the guest's browser that
// cast to it may cast again.
func TestGuestCastPart_ADeviceBeingCastToCastsNothing(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	guest := stage.join(t)
	part, err := stage.hub.GuestCastPart(guest.SessionID, guest.MemberID, senderOf(guest.MembershipCredential))
	if err != nil {
		t.Fatal(err)
	}
	if !stage.hub.IsBeingCastTo(part.MembershipCredential) {
		t.Error("the Chromecast's credential is not known to be cast to")
	}
	if stage.hub.IsBeingCastTo(guest.MembershipCredential) || stage.hub.IsBeingCastTo("no one's") {
		t.Error("the guest's browser's credential, or no one's, is known to be cast to")
	}
	receiver := senderOf(part.MembershipCredential)
	if _, err = stage.hub.GuestCastPart(guest.SessionID, guest.MemberID, receiver); !errors.Is(err, ErrBeingCastTo) {
		t.Errorf("a Chromecast's part asked for by a Chromecast cast to = %v; want ErrBeingCastTo", err)
	}
	if _, err = stage.hub.GuestAirPlayHold(guest.SessionID, guest.MemberID, receiver); !errors.Is(err, ErrBeingCastTo) {
		t.Errorf("an AirPlay hold asked for by a Chromecast cast to = %v; want ErrBeingCastTo", err)
	}
	if _, isViewer := stage.hub.ViewerOf(part.MembershipCredential); !isViewer {
		t.Error("the refused asks ended the Chromecast's own part")
	}
	_, err = stage.hub.GuestCastPart(guest.SessionID, guest.MemberID, senderOf(guest.MembershipCredential))
	if err != nil {
		t.Errorf("the guest's browser cast again = %v; want a part", err)
	}
}

// A cast ends as the credential of the device that cast goes (D520 §1): pushed out of the newest the
// member keeps, the receiver it cast to is told its place moved away and admits nothing, and that
// credential casts nothing more.
func TestGuestCastPart_EndsAsItsSendingDevicesCredentialGoes(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	partner := Person{UserID: 2, Name: "partner"}
	guest := stage.joinSignedIn(t, &partner)
	part, err := stage.hub.GuestCastPart(guest.SessionID, guest.MemberID, senderOf(guest.MembershipCredential))
	if err != nil {
		t.Fatal(err)
	}
	_, hostBox := stage.attach(t, stage.opened.MembershipCredential, RoleVideo)
	receiver, castBox := stage.attach(t, part.MembershipCredential, RoleVideo)
	for range credentialsKeptPerMember {
		if _, err = stage.hub.Mine(partner.UserID); err != nil {
			t.Fatal(err)
		}
	}
	if _, isViewer := stage.hub.ViewerOf(guest.MembershipCredential); isViewer {
		t.Fatal("the first device's credential is still kept; want it pushed out")
	}
	if ended := castBox.last(MessageEnded); ended.Reason != "movedAway" {
		t.Errorf("the receiver of a device whose credential went was told %+v; want ended, movedAway", ended)
	}
	if _, isViewer := stage.hub.ViewerOf(part.MembershipCredential); isViewer {
		t.Error("the cast of a device whose credential went still admits the guest")
	}
	if rosterLists(hostBox, receiver.DeviceID) {
		t.Error("the roster still lists the receiver of a device whose credential went")
	}
	_, err = stage.hub.GuestCastPart(guest.SessionID, guest.MemberID, senderOf(guest.MembershipCredential))
	if !errors.Is(err, ErrNoMember) {
		t.Errorf("a part asked for by a credential gone = %v; want ErrNoMember", err)
	}
}

// An AirPlay grant of a guest's holds their membership by a credential of its own while they are in
// (D519 §1): it names the guest's member, adds no locator for a discovery to find, and, as a
// Chromecast's part does, replaces the last cast's credential of the guest's device that asked and
// is replaced by its next (D520 §1); the host has none, and a removed guest's admits nothing.
func TestGuestAirPlayHold_HoldsTheGuestsMembershipAsTheirOneCast(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	guest := stage.join(t)
	locatorsBefore := len(stage.hub.LiveLocators())
	part, err := stage.hub.GuestCastPart(guest.SessionID, guest.MemberID, senderOf(guest.MembershipCredential))
	if err != nil {
		t.Fatal(err)
	}
	_, castBox := stage.attach(t, part.MembershipCredential, RoleVideo)
	hold, err := stage.hub.GuestAirPlayHold(guest.SessionID, guest.MemberID, senderOf(guest.MembershipCredential))
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
	_, err = stage.hub.GuestAirPlayHold(guest.SessionID, guest.MemberID, senderOf(guest.MembershipCredential))
	if err != nil {
		t.Fatal(err)
	}
	if _, isViewer = stage.hub.ViewerHeldBy(hold); isViewer {
		t.Error("the hold the guest's next cast replaced still admits them")
	}
	_, hostMemberID, _ := stage.hub.MemberOf(stage.opened.MembershipCredential)
	_, err = stage.hub.GuestAirPlayHold(guest.SessionID, hostMemberID, senderOf(guest.MembershipCredential))
	if !errors.Is(err, ErrNoMember) {
		t.Errorf("a hold for the host = %v; want ErrNoMember, the host AirPlaying as themselves", err)
	}
	last, err := stage.hub.GuestAirPlayHold(guest.SessionID, guest.MemberID, senderOf(guest.MembershipCredential))
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
