// SPDX-License-Identifier: AGPL-3.0-or-later

package together

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"
)

// Only the host ejects; an ejected member is told and their credential admits nothing more (D119).
func TestHub_AnEjectedMemberIsGone(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	guest := stage.join(t)
	_, guestPhone := stage.attach(t, guest.MembershipCredential, RolePanel)
	err := stage.hub.Eject(stage.opened.SessionID, guest.MemberID+100, guest.MemberID)
	if !errors.Is(err, ErrNotHost) {
		t.Errorf("an ejection by someone not the host = %v; want ErrNotHost", err)
	}
	if err = stage.hub.Eject(stage.opened.SessionID, stage.owner.UserID, guest.MemberID); err != nil {
		t.Fatal(err)
	}
	if ended := guestPhone.last(MessageEnded); ended.Reason != "ejected" {
		t.Errorf("the ejected member's phone was told %+v; want ended, ejected", ended)
	}
	_, err = stage.hub.Attach(guest.MembershipCredential, RoleVideo, true, nil, testTuning, (&inbox{}).send)
	if !errors.Is(err, ErrNoSession) {
		t.Errorf("the ejected member's credential = %v; want it to admit nothing", err)
	}
}

// A guest given a new link keeps their place on the devices they came in on until it is used; whoever
// joins by it takes the place over, with its rights and the name they give, those devices cut off and
// the place's other links ended; once they are removed, a new link for them admits no one (D311 §5,
// D312 §3, D318; the owner, 2026-10-19).
func TestHub_ANewLinkForAGuestMovesTheirPlaceOnlyOnceItIsUsed(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	session, host := stage.opened.SessionID, stage.owner.UserID
	first, err := stage.hub.AddLink(session, host, "my partner", Rights{CanSeek: true})
	if err != nil {
		t.Fatal(err)
	}
	guest, err := stage.hub.Join(first.Secret, "partner", nil, Leaving{})
	if err != nil {
		t.Fatal(err)
	}
	_, brave := stage.attach(t, guest.MembershipCredential, RoleVideo)
	moved, err := stage.hub.AnotherLinkFor(session, host, guest.MemberID)
	if err != nil {
		t.Fatal(err)
	}
	// Until the new link is used, the guest is still in on the browser they came in on.
	_, err = stage.hub.Attach(guest.MembershipCredential, RolePanel, true, nil, testTuning, (&inbox{}).send)
	if err != nil {
		t.Errorf("the guest's credential before the new link is used = %v; want it still to admit them", err)
	}
	chrome, err := stage.hub.Join(moved.Secret, "partner on chrome", nil, Leaving{})
	if err != nil {
		t.Fatal(err)
	}
	if chrome.MemberID != guest.MemberID {
		t.Errorf("the new link joined as member %d; want the guest's place, %d", chrome.MemberID, guest.MemberID)
	}
	stage.wantMovedAway(t, guest.MembershipCredential, brave, moved.LinkID)
	again, err := stage.hub.AnotherLinkFor(session, host, guest.MemberID)
	if err != nil {
		t.Fatal(err)
	}
	if err = stage.hub.Eject(session, host, guest.MemberID); err != nil {
		t.Fatal(err)
	}
	if _, err = stage.hub.Join(again.Secret, "someone", nil, Leaving{}); !errors.Is(err, ErrNoSession) {
		t.Errorf("a new link for a guest since removed = %v; want it to admit no one", err)
	}
}

// wantMovedAway fails unless the old browser, on inbox, was told its place moved away and its
// credential admits nothing, and the session holds only the new link, linkID, with the place's
// rights, its guest renamed "partner on chrome".
func (stage sessionStage) wantMovedAway(t *testing.T, credential string, old *inbox, linkID int64) {
	t.Helper()
	if ended := old.last(MessageEnded); ended.Reason != "movedAway" {
		t.Errorf("the old browser was told %+v; want ended, movedAway", ended)
	}
	_, err := stage.hub.Attach(credential, RoleVideo, true, nil, testTuning, (&inbox{}).send)
	if !errors.Is(err, ErrNoSession) {
		t.Errorf("the old browser's credential = %v; want it to admit nothing", err)
	}
	details, err := stage.hub.DetailsOf(stage.opened.SessionID, stage.owner.UserID)
	if err != nil {
		t.Fatal(err)
	}
	isRenamed := slices.ContainsFunc(details.Members, func(each MemberDetail) bool {
		return each.Name == "partner on chrome"
	})
	if len(details.Links) != 1 || details.Links[0].ID != linkID || !details.Links[0].Rights.CanSeek || !isRenamed {
		t.Errorf("the session = %+v; want the new link alone, with the place's rights, and the guest renamed", details)
	}
}

// A viewer plays what the session plays now, in the host's libraries, while their membership
// lasts; leaving ends it (D246 §3, §5).
func TestHub_AViewerPlaysWhatTheSessionPlaysWhileTheyAreIn(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	host, _ := stage.attach(t, stage.opened.MembershipCredential, RoleVideo)
	guest := stage.join(t)
	stage.command(t, host, CommandChangeItem, 7)
	viewer, isViewer := stage.hub.ViewerOf(guest.MembershipCredential)
	if !isViewer || viewer.FileID != 7 || viewer.HostUserID != stage.owner.UserID || viewer.MemberID != guest.MemberID {
		t.Errorf("the viewer = %+v (%v); want film 7, the owner's", viewer, isViewer)
	}
	if err := stage.hub.Leave(guest.MembershipCredential); err != nil {
		t.Fatal(err)
	}
	if _, isViewer = stage.hub.ViewerOf(guest.MembershipCredential); isViewer {
		t.Error("a viewer who left still plays")
	}
}

// Voice signalling is relayed to the device it names, said to be from the sender (D193 §4).
func TestHub_RelaysVoiceSignallingToTheDeviceItNames(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	host, _ := stage.attach(t, stage.opened.MembershipCredential, RoleBoth)
	guest := stage.join(t)
	guestDevice, guestBox := stage.attach(t, guest.MembershipCredential, RolePanel)
	stage.hub.Handle(t.Context(), host, Inbound{Type: MessageRTC, ToDeviceID: guestDevice.DeviceID, SDP: "offer"})
	if relayed := guestBox.last(MessageRTC); relayed.SDP != "offer" || relayed.FromDeviceID != host.DeviceID {
		t.Errorf("relayed %+v; want the host's offer", relayed)
	}
}

// The host changes what plays, to a film they may play; one they may not is refused as not found,
// and the group plays on as it was (D122, D243 §4).
func TestHub_TheHostChangesWhatPlaysOnlyToAFilmTheyMayPlay(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	host, hostBox := stage.attach(t, stage.opened.MembershipCredential, RoleVideo)
	stage.command(t, host, CommandChangeItem, 13)
	if refused := hostBox.last(MessageRejected); refused.Code != codeNotFound {
		t.Errorf("a change to a film the host may not play = %+v; want it refused as not found", refused)
	}
	stage.hub.Handle(t.Context(), host, Inbound{
		Type: MessageCommand, Command: CommandChangeItem, FileID: 9, PositionMs: 30_000,
	})
	if changed := hostBox.last(MessageGroup).Group; changed.FileID != 9 || changed.PositionMs != 30_000 {
		t.Errorf("a change to film 9 at 30 s = %+v; want it on from where the host played it", changed)
	}
}

// A device's state report carries the shortest round trip its pings measured, which the roster
// shows the others; a report without one keeps the last (cut-a §5.1, D66).
func TestHub_TheRosterShowsEachDevicesRoundTripItReported(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	host, hostBox := stage.attach(t, stage.opened.MembershipCredential, RoleVideo)
	guest := stage.join(t)
	guestDevice, _ := stage.attach(t, guest.MembershipCredential, RoleVideo)
	stage.hub.Handle(t.Context(), guestDevice, Inbound{Type: MessageState, RTTMs: 42})
	stage.ready(t, guestDevice, true)
	for _, each := range hostBox.last(MessageRoster).Roster {
		if each.ID != guest.MemberID {
			continue
		}
		if len(each.Devices) != 1 || each.Devices[0].RTTMs != 42 {
			t.Errorf("the guest in the host's roster = %+v; want its one device at 42 ms", each)
		}
		return
	}
	t.Errorf("the host's roster has no guest; host %d", host.MemberID)
}

// A device that says even its lowest quality is beyond its link is shown so in everyone's roster,
// with both numbers, until it says it keeps up again (D247 §8).
func TestHub_TheRosterTellsEveryoneADeviceCannotKeepUp(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	_, hostBox := stage.attach(t, stage.opened.MembershipCredential, RoleVideo)
	guest := stage.join(t)
	guestDevice, _ := stage.attach(t, guest.MembershipCredential, RoleVideo)
	guestInHostsRoster := func() DeviceView {
		for _, each := range hostBox.last(MessageRoster).Roster {
			if each.ID == guest.MemberID && len(each.Devices) == 1 {
				return each.Devices[0]
			}
		}
		t.Fatal("the host's roster has no guest with one device")
		return DeviceView{}
	}
	stage.hub.Handle(t.Context(), guestDevice, Inbound{
		Type: MessageState, Buffering: true, CannotKeepUp: &CannotKeepUp{LinkKbps: 600, LowestKbps: 750},
	})
	if got := guestInHostsRoster().CannotKeepUp; got == nil || *got != (CannotKeepUp{LinkKbps: 600, LowestKbps: 750}) {
		t.Errorf("the guest's device in the host's roster cannot keep up = %+v; want 600 kb/s against 750", got)
	}
	stage.hub.Handle(t.Context(), guestDevice, Inbound{Type: MessageState})
	if got := guestInHostsRoster().CannotKeepUp; got != nil {
		t.Errorf("after a report without it, the guest still cannot keep up: %+v", got)
	}
}

// A group's change is named by a code a client words, never by words; a seek says where it jumped
// from as well as to (D188).
func TestHub_AnEventIsACodeAndASeekSaysWhereItJumpedFrom(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	host, hostBox := stage.attach(t, stage.opened.MembershipCredential, RoleVideo)
	stage.command(t, host, CommandChangeItem, 7)
	stage.ready(t, host, false)
	stage.clock.Advance(30 * time.Second)
	stage.hub.Handle(t.Context(), host, Inbound{
		Type: MessageCommand, Command: CommandSeek, PositionMs: 90_000, BasedOnSeq: hostBox.last(MessageGroup).Seq,
	})
	var seeked *GroupView
	for _, message := range slices.Backward(hostBox.messages) {
		if message.Group != nil && message.Group.Event == EventSeeked {
			seeked = message.Group
			break
		}
	}
	if seeked == nil || seeked.Event != EventSeeked || seeked.SeekedFromMs != 30_000 || seeked.PositionMs != 90_000 {
		t.Errorf("the seek's group = %+v; want seeked, from 30 s to 90 s", seeked)
	}
	stage.command(t, host, CommandPause, 0)
	if paused := hostBox.last(MessageGroup).Group; paused.Event != EventPaused || paused.SeekedFromMs != 0 {
		t.Errorf("the pause after = %+v; want paused, with no seek's origin", paused)
	}
}

// playingStage is a stage with film 7 playing, the host's player leading it and a guest's following.
type playingStage struct {
	sessionStage
	host, guestDevice Attachment
	hostBox           *inbox
	guest             Joined
}

func playing(t *testing.T) playingStage {
	t.Helper()
	stage := newStage(t)
	host, hostBox := stage.attach(t, stage.opened.MembershipCredential, RoleVideo)
	guest := stage.join(t)
	guestDevice, _ := stage.attach(t, guest.MembershipCredential, RoleVideo)
	stage.command(t, host, CommandChangeItem, 7)
	stage.ready(t, host, false)
	stage.ready(t, guestDevice, false)
	if state := hostBox.last(MessageGroup).Group.State; state != GroupPlaying {
		t.Fatalf("film 7 put on, its leader playing: %s, want Playing", state)
	}
	return playingStage{sessionStage: stage, host: host, guestDevice: guestDevice, hostBox: hostBox, guest: guest}
}

// Everyone's roster says where each player stands: what its page waits on, a tap; once a film is
// on, loading until it is first ready, then watching, and a follower buffering catching up. Nothing
// of it moves the group, which stands for its leader alone (D246 §6, D371 §2). A guest still
// choosing her speakers was shown as nothing at all while the group played on without her (the
// owner's Watch Together session, 2026-10-18).
func TestHub_TheRosterSaysWhereEachPlayerStands(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	host, hostBox := stage.attach(t, stage.opened.MembershipCredential, RoleVideo)
	guest := stage.join(t)
	guestDevice, guestBox := stage.attach(t, guest.MembershipCredential, RoleVideo)
	guestStatus := func() DeviceStatus {
		for _, each := range hostBox.last(MessageRoster).Roster {
			if each.ID == guest.MemberID && len(each.Devices) == 1 {
				return each.Devices[0].Status
			}
		}
		t.Fatal("the host's roster has no guest with one device")
		return ""
	}
	if got := guestStatus(); got != "" {
		t.Errorf("before any film, waiting on nothing: %q; want none", got)
	}
	for _, standing := range []Standing{StandingNeedsTap} {
		stage.hub.Handle(t.Context(), guestDevice, Inbound{Type: MessageStanding, Standing: standing})
		if got := guestStatus(); got != DeviceStatus(standing) {
			t.Errorf("its page waiting on %q: %q; want the same", standing, got)
		}
	}
	stage.command(t, host, CommandChangeItem, 7)
	stage.ready(t, host, false)
	if got := hostBox.last(MessageGroup).Group.State; got != GroupPlaying {
		t.Errorf("the group with the guest waiting for her tap is %q; want it playing, waiting for no one", got)
	}
	stage.command(t, host, CommandStop, 0)
	stage.hub.Handle(t.Context(), guestDevice, Inbound{Type: MessageStanding, Standing: StandingNone})
	if got := guestStatus(); got != "" {
		t.Errorf("tapped, nothing on: %q; want none", got)
	}
	stage.command(t, host, CommandChangeItem, 8) // the roster comes with the group's change
	if got := guestStatus(); got != StatusLoading {
		t.Errorf("another film on: %q; want loading", got)
	}
	stage.ready(t, guestDevice, false)
	if got := guestStatus(); got != StatusWatching {
		t.Errorf("once ready: %q; want watching", got)
	}
	stage.ready(t, guestDevice, true)
	if got := guestStatus(); got != StatusCatchingUp {
		t.Errorf("a follower buffering: %q; want catching up", got)
	}
	stage.hub.Handle(t.Context(), guestDevice, Inbound{Type: MessageStanding, Standing: "dancing"})
	if got := guestBox.last(MessageRejected).Code; got != codeInvalid {
		t.Errorf("a standing no page says was answered %q; want it refused", got)
	}
	refusedBefore := guestBox.count(MessageRejected)
	stage.hub.Handle(t.Context(), guestDevice, Inbound{Type: MessageStanding, Standing: "choosingSound"})
	if guestBox.count(MessageRejected) != refusedBefore+1 {
		t.Error("a page still choosing its sound, which no page asks since D336, was not refused")
	}
	stage.hub.Handle(t.Context(), guestDevice, Inbound{Type: MessageStanding, Standing: StandingNeedsTap})
	if !strings.Contains(stage.logs.String(), "standing=needsTap") {
		t.Error("a page's standing was not logged")
	}
}

// A player's state says how it plays the film, delivered as it is or converted, at which quality, on
// what link, and everyone's roster shows it, a cast's remote among them; a delivery no player says is
// left out, and a state that says none keeps the last (D371 §4: "information about playback, such as
// the transcoding and bandwidth choices").
func TestHub_TheRosterSaysHowEachPlayerPlaysTheFilm(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	cast, err := stage.hub.OpenCast(stage.owner)
	if err != nil {
		t.Fatal(err)
	}
	receiver, _ := stage.attach(t, cast.ReceiverCredential, RoleVideo)
	_, remoteBox := stage.attach(t, cast.SenderCredential, RolePanel)
	receiverInRoster := func() DeviceView {
		for _, each := range remoteBox.last(MessageRoster).Roster {
			for _, attached := range each.Devices {
				if attached.ID == receiver.DeviceID {
					return attached
				}
			}
		}
		t.Fatal("the remote's roster has no receiver")
		return DeviceView{}
	}
	converted := Playback{Delivery: DeliveryConverted, QualityKbps: 4200, LinkKbps: 18_000}
	stage.command(t, receiver, CommandChangeItem, 12)
	stage.hub.Handle(t.Context(), receiver, Inbound{
		Type: MessageState, Playing: true, Playback: &converted, FileID: 12,
	})
	if got := receiverInRoster(); !got.IsLeader || got.Playback == nil || *got.Playback != converted {
		t.Errorf("the receiver in the remote's roster = %+v (%+v); want the leader, converted at 4200 kb/s "+
			"on 18000", got, got.Playback)
	}
	stage.hub.Handle(t.Context(), receiver, Inbound{Type: MessageState, Playing: true, FileID: 12})
	if got := receiverInRoster().Playback; got == nil || *got != converted {
		t.Errorf("after a state that says none, the receiver plays %+v; want the last said", got)
	}
	stage.hub.Handle(t.Context(), receiver, Inbound{
		Type: MessageState, Playing: true, Playback: &Playback{Delivery: "teleported"}, FileID: 12,
	})
	if got := receiverInRoster().Playback; got == nil || *got != converted {
		t.Errorf("after a delivery no player says, the receiver plays %+v; want the last known", got)
	}
}
