// SPDX-License-Identifier: GPL-3.0-or-later

package together

import (
	"context"
	"crypto/rand"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
)

// inbox keeps the messages one device was sent.
type inbox struct {
	mutex    sync.Mutex
	messages []Outbound
}

func (box *inbox) send(message Outbound) {
	box.mutex.Lock()
	defer box.mutex.Unlock()
	box.messages = append(box.messages, message)
}

// last is the last message of a type the device was sent, or the zero one.
func (box *inbox) last(kind MessageType) Outbound {
	box.mutex.Lock()
	defer box.mutex.Unlock()
	for _, message := range slices.Backward(box.messages) {
		if message.Type == kind {
			return message
		}
	}
	return Outbound{}
}

// stage is a hub with a session the owner hosts, playing file 7, holding DefaultCap people, and a
// clock.
type stage struct {
	hub     *Hub
	clock   *clocktest.Fake
	created Created
	owner   Person
}

func newStage(t *testing.T, transport Transport) stage {
	t.Helper()
	fakeClock := clocktest.NewFake(time.Date(2026, 10, 5, 20, 0, 0, 0, time.UTC))
	hub := NewHub(fakeClock, rand.Reader, func(_ context.Context, _, fileID int64) bool { return fileID != 13 })
	owner := Person{UserID: 1, Name: "owner"}
	created, err := hub.Create(owner, 7, Abilities{Transport: transport}, DefaultCap)
	if err != nil {
		t.Fatal(err)
	}
	return stage{hub: hub, clock: fakeClock, created: created, owner: owner}
}

// attach attaches a device with credential and role, and returns it and its inbox.
func (stage stage) attach(t *testing.T, credential string, role DeviceRole) (Attachment, *inbox) {
	t.Helper()
	box := &inbox{}
	attachment, err := stage.hub.Attach(credential, role, true, nil, box.send)
	if err != nil {
		t.Fatal(err)
	}
	return attachment, box
}

// join admits the guest, "partner", by the session's link.
func (stage stage) join(t *testing.T) Joined {
	t.Helper()
	joined, err := stage.hub.Join(stage.created.JoinSecret, "partner")
	if err != nil {
		t.Fatal(err)
	}
	return joined
}

// A play waits until every device that plays is ready, then plays; one that buffers makes the group
// wait for it and name it, and when it is ready again the group plays on from where it waited
// (watch-together.md, Sync).
func TestHub_TheGroupPlaysWhenEveryPlayerIsReadyAndWaitsForOneThatBuffers(t *testing.T) {
	t.Parallel()
	stage := newStage(t, TransportGroup)
	host, hostBox := stage.attach(t, stage.created.MembershipCredential, RolePlayback)
	guest := stage.join(t)
	guestDevice, guestBox := stage.attach(t, guest.MembershipCredential, RolePlayback)
	if welcome := guestBox.last(MessageWelcome); welcome.SessionID != stage.created.SessionID ||
		welcome.Group == nil || welcome.Group.FileID != 7 || len(welcome.Roster) != 2 {
		t.Fatalf("the guest's welcome = %+v; want the session, its film and both members", welcome)
	}
	stage.hub.Handle(t.Context(), guestDevice, Inbound{Type: MessageState, Buffering: true})
	stage.hub.Handle(t.Context(), host, Inbound{Type: MessageCommand, Command: CommandPlay})
	if state := hostBox.last(MessageGroup).Group.State; state != GroupWaiting {
		t.Errorf("a play with the guest buffering: %s, want Waiting", state)
	}
	stage.hub.Handle(t.Context(), guestDevice, Inbound{Type: MessageState})
	if state := guestBox.last(MessageGroup).Group.State; state != GroupPlaying {
		t.Errorf("once the guest is ready: %s, want Playing", state)
	}
	stage.clock.Advance(10 * time.Second)
	stage.hub.Handle(t.Context(), guestDevice, Inbound{Type: MessageState, Buffering: true})
	waiting := hostBox.last(MessageGroup).Group
	if waiting.State != GroupWaiting || waiting.ByMemberID != guest.MemberID || waiting.PositionMs != 10_000 {
		t.Errorf("the guest buffering 10 s in: %+v; want Waiting at 10 s, for the guest", waiting)
	}
	stage.clock.Advance(5 * time.Second)
	stage.hub.Handle(t.Context(), guestDevice, Inbound{Type: MessageState})
	if resumed := hostBox.last(MessageGroup).Group; resumed.State != GroupPlaying || resumed.PositionMs != 10_000 {
		t.Errorf("the guest ready again: %+v; want Playing on from 10 s", resumed)
	}
}

// A drop is a state, not an ending: a playing group waits for the device that left, and plays on
// when it is back and ready (watch-together.md, What each person sees).
func TestHub_ADroppedPlayerMakesTheGroupWaitUntilItIsBack(t *testing.T) {
	t.Parallel()
	stage := newStage(t, TransportGroup)
	host, hostBox := stage.attach(t, stage.created.MembershipCredential, RolePlayback)
	guest := stage.join(t)
	guestDevice, _ := stage.attach(t, guest.MembershipCredential, RolePlayback)
	stage.hub.Handle(t.Context(), host, Inbound{Type: MessageCommand, Command: CommandPlay})
	stage.hub.Detach(guestDevice)
	if dropped := hostBox.last(MessageGroup).Group; dropped.State != GroupWaiting || dropped.Event != "left" {
		t.Errorf("the guest's device dropped: %+v; want Waiting, for the one who left", dropped)
	}
	stage.attach(t, guest.MembershipCredential, RolePlayback)
	if back := hostBox.last(MessageGroup).Group; back.State != GroupPlaying {
		t.Errorf("the guest back and ready: %s; want Playing", back.State)
	}
}

// Transport is the host's alone when the host said so; what plays is always the host's (D121).
func TestHub_OnlyTheHostDoesWhatTheAbilitiesKeepForTheHost(t *testing.T) {
	t.Parallel()
	stage := newStage(t, TransportHost)
	stage.attach(t, stage.created.MembershipCredential, RolePlayback)
	guest := stage.join(t)
	guestDevice, guestBox := stage.attach(t, guest.MembershipCredential, RolePlayback)
	for _, command := range []CommandKind{CommandPlay, CommandChangeItem} {
		stage.hub.Handle(t.Context(), guestDevice, Inbound{Type: MessageCommand, Command: command, FileID: 9})
		if refused := guestBox.last(MessageRejected); refused.Code != codeForbidden {
			t.Errorf("the guest's %s with host-only transport = %+v; want it refused as forbidden", command, refused)
		}
	}
}

// Of two seeks issued against the same group, the one that arrives second is the stale one and is
// refused; the first stands (cut-a §5.1).
func TestHub_ASeekAgainstAnotherSeekIsRefusedAsStale(t *testing.T) {
	t.Parallel()
	stage := newStage(t, TransportGroup)
	host, hostBox := stage.attach(t, stage.created.MembershipCredential, RolePlayback)
	guest := stage.join(t)
	guestDevice, guestBox := stage.attach(t, guest.MembershipCredential, RolePlayback)
	seq := hostBox.last(MessageRoster).Seq
	stage.hub.Handle(t.Context(), host, Inbound{
		Type: MessageCommand, Command: CommandSeek, PositionMs: 60_000, BasedOnSeq: seq,
	})
	stage.hub.Handle(t.Context(), guestDevice, Inbound{
		Type: MessageCommand, Command: CommandSeek, PositionMs: 90_000, BasedOnSeq: seq,
	})
	if refused := guestBox.last(MessageRejected); refused.Code != codeStale {
		t.Errorf("the second seek = %+v; want it refused as stale", refused)
	}
	if group := guestBox.last(MessageGroup).Group; group.PositionMs != 60_000 {
		t.Errorf("the group is at %d ms; want the first seek's 60000", group.PositionMs)
	}
}

// A join link admits up to the cap, counting people, and a refusal is counted for the host; a new
// link admits no one by the old, and leaves everyone in; an ejected member is told and their
// credential admits nothing more (D119, D120).
func TestHub_TheLinkAdmitsToTheCapRotatesAndAnEjectedMemberIsGone(t *testing.T) {
	t.Parallel()
	stage := newStage(t, TransportGroup)
	_, hostBox := stage.attach(t, stage.created.MembershipCredential, RolePlayback)
	guest := stage.join(t)
	_, guestPhone := stage.attach(t, guest.MembershipCredential, RoleVoice)
	stage.attach(t, guest.MembershipCredential, RolePlayback) // a second device is the same person
	if _, err := stage.hub.Join(stage.created.JoinSecret, "stranger"); !errors.Is(err, ErrFull) {
		t.Errorf("a third person's join at a cap of two = %v; want ErrFull", err)
	}
	if refused := hostBox.last(MessageRoster).RefusedJoins; refused != 1 {
		t.Errorf("the host's roster counts %d refused joins; want 1", refused)
	}
	fresh, err := stage.hub.Rotate(stage.created.SessionID, stage.owner.UserID)
	if err != nil {
		t.Fatal(err)
	}
	if err = stage.hub.SetCap(stage.created.SessionID, stage.owner.UserID, 3); err != nil {
		t.Fatal(err)
	}
	if _, err = stage.hub.Join(stage.created.JoinSecret, "stranger"); !errors.Is(err, ErrNoSession) {
		t.Errorf("a join by the old link = %v; want ErrNoSession", err)
	}
	if _, err = stage.hub.Join(fresh, "cousin"); err != nil {
		t.Errorf("a join by the new link = %v", err)
	}
	if err = stage.hub.Eject(stage.created.SessionID, guest.MemberID+100, guest.MemberID); !errors.Is(err, ErrNotHost) {
		t.Errorf("an ejection by someone not the host = %v; want ErrNotHost", err)
	}
	if err = stage.hub.Eject(stage.created.SessionID, stage.owner.UserID, guest.MemberID); err != nil {
		t.Fatal(err)
	}
	if ended := guestPhone.last(MessageEnded); ended.Reason != "ejected" {
		t.Errorf("the ejected member's phone was told %+v; want ended, ejected", ended)
	}
	if _, err = stage.hub.Attach(guest.MembershipCredential, RolePlayback, true, nil, (&inbox{}).send); !errors.Is(err,
		ErrNoSession) {
		t.Errorf("the ejected member's credential = %v; want it to admit nothing", err)
	}
}

// Voice signalling is relayed to the device it names, said to be from the sender (D193 §4).
func TestHub_RelaysVoiceSignallingToTheDeviceItNames(t *testing.T) {
	t.Parallel()
	stage := newStage(t, TransportGroup)
	host, _ := stage.attach(t, stage.created.MembershipCredential, RoleBoth)
	guest := stage.join(t)
	guestDevice, guestBox := stage.attach(t, guest.MembershipCredential, RoleVoice)
	stage.hub.Handle(t.Context(), host, Inbound{Type: MessageRTC, ToDeviceID: guestDevice.DeviceID, SDP: "offer"})
	if relayed := guestBox.last(MessageRTC); relayed.SDP != "offer" || relayed.FromDeviceID != host.DeviceID {
		t.Errorf("relayed %+v; want the host's offer", relayed)
	}
}

// The host changes what plays, to a film they may play; one they may not is refused as not found,
// and the group plays on as it was (D122, D243 §4).
func TestHub_TheHostChangesWhatPlaysOnlyToAFilmTheyMayPlay(t *testing.T) {
	t.Parallel()
	stage := newStage(t, TransportGroup)
	host, hostBox := stage.attach(t, stage.created.MembershipCredential, RolePlayback)
	stage.hub.Handle(t.Context(), host, Inbound{Type: MessageCommand, Command: CommandChangeItem, FileID: 13})
	if refused := hostBox.last(MessageRejected); refused.Code != codeNotFound {
		t.Errorf("a change to a film the host may not play = %+v; want it refused as not found", refused)
	}
	stage.hub.Handle(t.Context(), host, Inbound{Type: MessageCommand, Command: CommandChangeItem, FileID: 9})
	if changed := hostBox.last(MessageGroup).Group; changed.FileID != 9 || changed.PositionMs != 0 {
		t.Errorf("a change to film 9 = %+v; want it playing from the start", changed)
	}
}

// A device's state report carries the shortest round trip its pings measured, which the roster
// shows the others; a report without one keeps the last (cut-a §5.1, D66).
func TestHub_TheRosterShowsEachDevicesRoundTripItReported(t *testing.T) {
	t.Parallel()
	stage := newStage(t, TransportGroup)
	host, hostBox := stage.attach(t, stage.created.MembershipCredential, RolePlayback)
	guest := stage.join(t)
	guestDevice, _ := stage.attach(t, guest.MembershipCredential, RolePlayback)
	stage.hub.Handle(t.Context(), guestDevice, Inbound{Type: MessageState, RTTMs: 42})
	stage.hub.Handle(t.Context(), guestDevice, Inbound{Type: MessageState, Buffering: true})
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
