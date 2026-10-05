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

// stage is a hub with a session the owner hosts, nothing playing yet, and a clock.
type stage struct {
	hub    *Hub
	clock  *clocktest.Fake
	opened Opened
	owner  Person
}

func newStage(t *testing.T) stage {
	t.Helper()
	fakeClock := clocktest.NewFake(time.Date(2026, 10, 5, 20, 0, 0, 0, time.UTC))
	hub := NewHub(fakeClock, rand.Reader, func(_ context.Context, _, fileID int64) bool { return fileID != 13 })
	owner := Person{UserID: 1, Name: "owner"}
	opened, err := hub.Open(owner)
	if err != nil {
		t.Fatal(err)
	}
	return stage{hub: hub, clock: fakeClock, opened: opened, owner: owner}
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

// join admits the guest, "partner", by a link of their own.
func (stage stage) join(t *testing.T) Joined {
	t.Helper()
	made, err := stage.hub.AddLink(stage.opened.SessionID, stage.owner.UserID, "my partner")
	if err != nil {
		t.Fatal(err)
	}
	joined, err := stage.hub.Join(made.Secret, "partner")
	if err != nil {
		t.Fatal(err)
	}
	return joined
}

// command sends the host's command from attachment.
func (stage stage) command(t *testing.T, attachment Attachment, command CommandKind, fileID int64) {
	t.Helper()
	stage.hub.Handle(t.Context(), attachment, Inbound{Type: MessageCommand, Command: command, FileID: fileID})
}

// ready says attachment is ready to play along, or buffering.
func (stage stage) ready(t *testing.T, attachment Attachment, isBuffering bool) {
	t.Helper()
	stage.hub.Handle(t.Context(), attachment, Inbound{Type: MessageState, Buffering: isBuffering})
}

// A session starts with nothing playing; what the host puts on waits until every player that has
// been ready is, then plays; one that buffers makes the group wait for it and name it, and when it
// is ready again the group plays on from where it waited (watch-together.md, Sync; D246 §5, §9).
func TestHub_TheGroupPlaysWhenEveryPlayerIsReadyAndWaitsForOneThatBuffers(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	host, hostBox := stage.attach(t, stage.opened.MembershipCredential, RolePlayback)
	guest := stage.join(t)
	guestDevice, guestBox := stage.attach(t, guest.MembershipCredential, RolePlayback)
	if welcome := guestBox.last(MessageWelcome); welcome.SessionID != stage.opened.SessionID ||
		welcome.Group == nil || welcome.Group.State != GroupIdle || welcome.Group.FileID != 0 ||
		len(welcome.Roster) != 2 || welcome.IsHost {
		t.Fatalf("the guest's welcome = %+v; want the session, nothing playing, and both members", welcome)
	}
	stage.ready(t, host, false)
	stage.ready(t, guestDevice, false)
	stage.ready(t, guestDevice, true)
	stage.command(t, host, CommandChangeItem, 7)
	if group := hostBox.last(MessageGroup).Group; group.State != GroupWaiting || group.FileID != 7 {
		t.Errorf("film 7 put on with the guest buffering: %+v, want Waiting on 7", group)
	}
	stage.ready(t, guestDevice, false)
	if state := guestBox.last(MessageGroup).Group.State; state != GroupPlaying {
		t.Errorf("once the guest is ready: %s, want Playing", state)
	}
	stage.clock.Advance(10 * time.Second)
	stage.ready(t, guestDevice, true)
	waiting := hostBox.last(MessageGroup).Group
	if waiting.State != GroupWaiting || waiting.ByMemberID != guest.MemberID || waiting.PositionMs != 10_000 {
		t.Errorf("the guest buffering 10 s in: %+v; want Waiting at 10 s, for the guest", waiting)
	}
	stage.clock.Advance(5 * time.Second)
	stage.ready(t, guestDevice, false)
	if resumed := hostBox.last(MessageGroup).Group; resumed.State != GroupPlaying || resumed.PositionMs != 10_000 {
		t.Errorf("the guest ready again: %+v; want Playing on from 10 s", resumed)
	}
}

// Someone who joins while the film plays holds no one up while they choose their speakers and
// load; once ready they play along, and are waited for from then on (D246 §6).
func TestHub_AJoiningViewerHoldsNoOneUpUntilReady(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	host, hostBox := stage.attach(t, stage.opened.MembershipCredential, RolePlayback)
	stage.ready(t, host, false)
	stage.command(t, host, CommandChangeItem, 7)
	if state := hostBox.last(MessageGroup).Group.State; state != GroupPlaying {
		t.Fatalf("film 7 put on with only the host, ready: %s, want Playing", state)
	}
	guest := stage.join(t)
	guestDevice, guestBox := stage.attach(t, guest.MembershipCredential, RolePlayback)
	stage.ready(t, guestDevice, true)
	if state := hostBox.last(MessageGroup).Group.State; state != GroupPlaying {
		t.Errorf("a viewer joining and loading: %s, want Playing on without them", state)
	}
	if welcome := guestBox.last(MessageWelcome); welcome.Group.FileID != 7 || welcome.Group.State != GroupPlaying {
		t.Errorf("the joining viewer's welcome = %+v; want film 7 playing, to start where it is", welcome.Group)
	}
	stage.ready(t, guestDevice, false)
	stage.ready(t, guestDevice, true)
	if state := hostBox.last(MessageGroup).Group.State; state != GroupWaiting {
		t.Errorf("the viewer, once ready, buffering: %s, want Waiting for them", state)
	}
}

// A drop is a state, not an ending: a playing group waits for the device that left, and plays on
// when it is back and ready (watch-together.md, What each person sees).
func TestHub_ADroppedPlayerMakesTheGroupWaitUntilItIsBack(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	host, hostBox := stage.attach(t, stage.opened.MembershipCredential, RolePlayback)
	guest := stage.join(t)
	guestDevice, _ := stage.attach(t, guest.MembershipCredential, RolePlayback)
	stage.ready(t, guestDevice, false)
	stage.command(t, host, CommandChangeItem, 7)
	stage.hub.Detach(guestDevice)
	if dropped := hostBox.last(MessageGroup).Group; dropped.State != GroupWaiting || dropped.Event != "left" {
		t.Errorf("the guest's device dropped: %+v; want Waiting, for the one who left", dropped)
	}
	back, _ := stage.attach(t, guest.MembershipCredential, RolePlayback)
	stage.ready(t, back, false)
	if group := hostBox.last(MessageGroup).Group; group.State != GroupPlaying {
		t.Errorf("the guest back and ready: %s; want Playing", group.State)
	}
}

// In Cut A only the host controls playback: a viewer's every command is refused (D246 §8).
func TestHub_OnlyTheHostControlsPlayback(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	host, _ := stage.attach(t, stage.opened.MembershipCredential, RolePlayback)
	stage.command(t, host, CommandChangeItem, 7)
	guest := stage.join(t)
	guestDevice, guestBox := stage.attach(t, guest.MembershipCredential, RolePlayback)
	for _, command := range []CommandKind{CommandPlay, CommandPause, CommandSeek, CommandChangeItem, CommandStop} {
		stage.command(t, guestDevice, command, 9)
		if refused := guestBox.last(MessageRejected); refused.Code != codeForbidden {
			t.Errorf("the viewer's %s = %+v; want it refused as forbidden", command, refused)
		}
	}
}

// When the host stops, every screen goes black: nothing plays until the host puts something on
// (D246 §5).
func TestHub_TheHostStoppingLeavesNothingPlaying(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	host, _ := stage.attach(t, stage.opened.MembershipCredential, RolePlayback)
	guest := stage.join(t)
	_, guestBox := stage.attach(t, guest.MembershipCredential, RolePlayback)
	stage.command(t, host, CommandChangeItem, 7)
	stage.command(t, host, CommandStop, 0)
	if group := guestBox.last(MessageGroup).Group; group.State != GroupIdle || group.FileID != 0 {
		t.Errorf("after the host stopped: %+v; want Idle, nothing on", group)
	}
	if viewer, isViewer := stage.hub.ViewerOf(guest.MembershipCredential); !isViewer || viewer.FileID != 0 {
		t.Errorf("the viewer after the stop = %+v (%v); want nothing to play", viewer, isViewer)
	}
}

// Of two seeks issued against the same group, the one that arrives second is the stale one and is
// refused; the first stands (cut-a §5.1).
func TestHub_ASeekAgainstAnotherSeekIsRefusedAsStale(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	host, hostBox := stage.attach(t, stage.opened.MembershipCredential, RolePlayback)
	hostPhone, phoneBox := stage.attach(t, stage.opened.MembershipCredential, RolePlayback)
	stage.command(t, host, CommandChangeItem, 7)
	seq := hostBox.last(MessageGroup).Seq
	stage.hub.Handle(t.Context(), host, Inbound{
		Type: MessageCommand, Command: CommandSeek, PositionMs: 60_000, BasedOnSeq: seq,
	})
	stage.hub.Handle(t.Context(), hostPhone, Inbound{
		Type: MessageCommand, Command: CommandSeek, PositionMs: 90_000, BasedOnSeq: seq,
	})
	if refused := phoneBox.last(MessageRejected); refused.Code != codeStale {
		t.Errorf("the second seek = %+v; want it refused as stale", refused)
	}
	if group := phoneBox.last(MessageGroup).Group; group.PositionMs != 60_000 {
		t.Errorf("the group is at %d ms; want the first seek's 60000", group.PositionMs)
	}
}

// Each link admits the one person it was made for, once, keeping the host's note for the host
// alone, and a withdrawn link admits no one (D246 §3).
func TestHub_EachLinkAdmitsOnePersonOnce(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	_, hostBox := stage.attach(t, stage.opened.MembershipCredential, RolePlayback)
	if _, err := stage.hub.AddLink(stage.opened.SessionID, stage.owner.UserID, ""); !errors.Is(err, ErrNoNote) {
		t.Errorf("a link with no note = %v; want ErrNoNote", err)
	}
	made, err := stage.hub.AddLink(stage.opened.SessionID, stage.owner.UserID, "Grandma")
	if err != nil {
		t.Fatal(err)
	}
	guest, err := stage.hub.Join(made.Secret, "Gran")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = stage.hub.Join(made.Secret, "stranger"); !errors.Is(err, ErrNoSession) {
		t.Errorf("a second join by one link = %v; want ErrNoSession", err)
	}
	for _, member := range hostBox.last(MessageRoster).Roster {
		if member.ID == guest.MemberID && member.Name != "Gran" {
			t.Errorf("the roster names the guest %q; want the name they chose", member.Name)
		}
	}
	details, err := stage.hub.DetailsOf(stage.opened.SessionID, stage.owner.UserID)
	if err != nil || len(details.Links) != 1 || details.Links[0].UsedBy != guest.MemberID ||
		!slices.ContainsFunc(details.Members, func(each MemberDetail) bool { return each.AdminNote == "Grandma" }) {
		t.Errorf("the host's view = %+v (%v); want the link used by Gran, noted Grandma", details, err)
	}
	unused, err := stage.hub.AddLink(stage.opened.SessionID, stage.owner.UserID, "a cousin")
	if err != nil {
		t.Fatal(err)
	}
	if err = stage.hub.RemoveLink(stage.opened.SessionID, stage.owner.UserID, unused.LinkID); err != nil {
		t.Fatal(err)
	}
	if _, err = stage.hub.Join(unused.Secret, "cousin"); !errors.Is(err, ErrNoSession) {
		t.Errorf("a join by a withdrawn link = %v; want ErrNoSession", err)
	}
}

// Only the host ejects; an ejected member is told and their credential admits nothing more (D119).
func TestHub_AnEjectedMemberIsGone(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	guest := stage.join(t)
	_, guestPhone := stage.attach(t, guest.MembershipCredential, RoleVoice)
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
	_, err = stage.hub.Attach(guest.MembershipCredential, RolePlayback, true, nil, (&inbox{}).send)
	if !errors.Is(err, ErrNoSession) {
		t.Errorf("the ejected member's credential = %v; want it to admit nothing", err)
	}
}

// A host has one session: opening again gives it back, with a new credential for a reloaded page,
// and the old one still names the host's devices already attached (D246 §5).
func TestHub_OpeningAgainGivesTheHostTheirSession(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	again, err := stage.hub.Open(stage.owner)
	if err != nil {
		t.Fatal(err)
	}
	if again.SessionID != stage.opened.SessionID || again.IsNew ||
		again.MembershipCredential == stage.opened.MembershipCredential {
		t.Errorf("opening again = %+v; want the same session, not new, with a new credential", again)
	}
	if _, err = stage.hub.Attach(again.MembershipCredential, RolePlayback, true, nil, (&inbox{}).send); err != nil {
		t.Errorf("the new credential = %v; want it to admit the host", err)
	}
	if _, isViewer := stage.hub.ViewerOf(again.MembershipCredential); isViewer {
		t.Error("the host's credential reads as a viewer's; the host plays as themselves")
	}
}

// A viewer plays what the session plays now, in the host's libraries, while their membership
// lasts; leaving ends it (D246 §3, §5).
func TestHub_AViewerPlaysWhatTheSessionPlaysWhileTheyAreIn(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	host, _ := stage.attach(t, stage.opened.MembershipCredential, RolePlayback)
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
	stage := newStage(t)
	host, hostBox := stage.attach(t, stage.opened.MembershipCredential, RolePlayback)
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
	host, hostBox := stage.attach(t, stage.opened.MembershipCredential, RolePlayback)
	guest := stage.join(t)
	guestDevice, _ := stage.attach(t, guest.MembershipCredential, RolePlayback)
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
