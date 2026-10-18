// SPDX-License-Identifier: AGPL-3.0-or-later

package together

import (
	"bytes"
	"cmp"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
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

// testTuning is how the tests' devices are welcomed to follow their group.
var testTuning = Tuning{
	LeaveDriftUpToMs: 500, NudgeMostPercent: 10, SeekDriftBeyondMs: 3000, CatchUpBufferPercent: 80, SyncWaitMs: 20_000,
}

// testSyncWait is testTuning's sync wait.
const testSyncWait = 20 * time.Second

// sessionStage is a hub with a session the owner hosts, nothing playing yet, and a clock.
type sessionStage struct {
	hub   *Hub
	clock *clocktest.Fake
	// logs is what the hub logged.
	logs   *bytes.Buffer
	opened Opened
	owner  Person
}

func newStage(t *testing.T) sessionStage {
	t.Helper()
	fakeClock := clocktest.NewFake(time.Date(2026, 10, 5, 20, 0, 0, 0, time.UTC))
	logs := &bytes.Buffer{}
	hub := NewHub(fakeClock, rand.Reader, func(_ context.Context, _, fileID int64) bool { return fileID != 13 },
		slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	owner := Person{UserID: 1, Name: "owner"}
	opened, err := hub.Open(owner)
	if err != nil {
		t.Fatal(err)
	}
	return sessionStage{hub: hub, clock: fakeClock, logs: logs, opened: opened, owner: owner}
}

// attach attaches a device with credential and role, and returns it and its inbox.
func (stage sessionStage) attach(t *testing.T, credential string, role DeviceRole) (Attachment, *inbox) {
	t.Helper()
	box := &inbox{}
	attachment, err := stage.hub.Attach(credential, role, true, nil, testTuning, box.send)
	if err != nil {
		t.Fatal(err)
	}
	return attachment, box
}

// join admits the guest, "partner", by a link of their own.
func (stage sessionStage) join(t *testing.T) Joined {
	t.Helper()
	made, err := stage.hub.AddLink(stage.opened.SessionID, stage.owner.UserID, "my partner", Rights{})
	if err != nil {
		t.Fatal(err)
	}
	joined, err := stage.hub.Join(made.Secret, "partner", nil)
	if err != nil {
		t.Fatal(err)
	}
	return joined
}

// command sends the host's command from attachment.
func (stage sessionStage) command(t *testing.T, attachment Attachment, command CommandKind, fileID int64) {
	t.Helper()
	stage.hub.Handle(t.Context(), attachment, Inbound{Type: MessageCommand, Command: command, FileID: fileID})
}

// ready says attachment is ready to play along, or buffering.
func (stage sessionStage) ready(t *testing.T, attachment Attachment, isBuffering bool) {
	t.Helper()
	stage.hub.Handle(t.Context(), attachment, Inbound{Type: MessageState, Buffering: isBuffering})
}

// tick is a ready player's report while the group waits: any message ends a wait whose time is up.
func (stage sessionStage) tick(t *testing.T, attachment Attachment) {
	t.Helper()
	stage.hub.Handle(t.Context(), attachment, Inbound{Type: MessagePing})
}

// A pong says where the group is as the server answers, so a follower places the group by the pong
// whose round trip it measured shortest (D264).
func TestHub_APongSaysWhereTheGroupIsAsItAnswers(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	host, box := stage.attach(t, stage.opened.MembershipCredential, RolePlayback)
	stage.ready(t, host, false)
	stage.command(t, host, CommandChangeItem, 7)
	stage.clock.Advance(12 * time.Second)
	stage.hub.Handle(t.Context(), host, Inbound{Type: MessagePing, TMono: 4321})
	pong := box.last(MessagePong)
	if pong.TMono != 4321 || pong.Group == nil || pong.Group.State != GroupPlaying ||
		pong.Group.PositionMs != 12_000 || pong.Seq != box.last(MessageGroup).Seq {
		t.Errorf("the pong is %+v (group %+v); want tMono 4321 and the group playing at 12 s, at its seq",
			pong, pong.Group)
	}
}

// A session starts with nothing playing; what the host puts on waits until every player that has
// been ready is, then plays; one that buffers then stops no one: the group plays on, naming it, and
// it catches up alone (watch-together.md, Sync; D246 §5; D250 §2).
func TestHub_TheGroupPlaysWhenEveryPlayerIsReadyAndPlaysOnWhenOneBuffers(t *testing.T) {
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
	playingOn := hostBox.last(MessageGroup).Group
	if playingOn.State != GroupPlaying || playingOn.Event != EventContinuedWithout ||
		playingOn.ByMemberID != guest.MemberID || playingOn.PositionMs != 10_000 {
		t.Errorf("the guest buffering 10 s in: %+v; want Playing on from 10 s without the guest", playingOn)
	}
	seq := hostBox.last(MessageGroup).Seq
	stage.clock.Advance(5 * time.Second)
	stage.ready(t, guestDevice, false)
	if hostBox.last(MessageGroup).Seq != seq {
		t.Errorf("the guest ready again changed the group: %+v", hostBox.last(MessageGroup).Group)
	}
}

// Every change of the group is logged: what plays, the event and who caused it, and while it waits,
// whose player it waits for and until when. A group that would not play on staging could only be
// guessed at (2026-10-06).
func TestHub_LogsEachChangeOfTheGroupAndWhoItWaitsFor(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	host, _ := stage.attach(t, stage.opened.MembershipCredential, RolePlayback)
	guest := stage.join(t)
	guestDevice, _ := stage.attach(t, guest.MembershipCredential, RolePlayback)
	stage.ready(t, host, false)
	stage.ready(t, guestDevice, false)
	stage.ready(t, guestDevice, true)
	stage.command(t, host, CommandChangeItem, 7)
	waiting := fmt.Sprintf("state=Waiting event=changedItem byMemberId=%d fileId=7", host.MemberID)
	wantFor := fmt.Sprintf("waitingForMemberIds=[%d]", guest.MemberID)
	for line := range strings.Lines(stage.logs.String()) {
		if strings.Contains(line, `msg="watch together: the group changed"`) && strings.Contains(line, waiting) &&
			strings.Contains(line, wantFor) && strings.Contains(line, "waitUntil=") {
			return
		}
	}
	t.Errorf("no log line says the group waits on film 7 for member %d:\n%s", guest.MemberID, stage.logs.String())
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
	if group := hostBox.last(MessageGroup).Group; group.State != GroupPlaying || group.ByMemberID != guest.MemberID {
		t.Errorf("the viewer, once ready, buffering: %+v, want Playing on without them (D250 §2)", group)
	}
}

// A drop stops no one (D250 §2): a playing group plays on while the device is gone, and a sync point
// waiting for it plays once the rest are ready.
func TestHub_ADroppedPlayerStopsNoOne(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	host, hostBox := stage.attach(t, stage.opened.MembershipCredential, RolePlayback)
	guest := stage.join(t)
	guestDevice, _ := stage.attach(t, guest.MembershipCredential, RolePlayback)
	stage.ready(t, host, false)
	stage.ready(t, guestDevice, false)
	stage.command(t, host, CommandChangeItem, 7)
	seq := hostBox.last(MessageGroup).Seq
	stage.hub.Detach(guestDevice)
	if hostBox.last(MessageGroup).Seq != seq || hostBox.last(MessageGroup).Group.State != GroupPlaying {
		t.Errorf("the guest's device dropped: %+v; want the group playing on, unchanged", hostBox.last(MessageGroup))
	}
	back, _ := stage.attach(t, guest.MembershipCredential, RolePlayback)
	stage.ready(t, back, false)
	stage.ready(t, back, true)
	stage.command(t, host, CommandPause, 0)
	stage.command(t, host, CommandPlay, 0)
	if group := hostBox.last(MessageGroup).Group; group.State != GroupWaiting {
		t.Fatalf("the host's play with the guest loading: %+v; want Waiting for the guest", group)
	}
	stage.hub.Detach(back)
	if group := hostBox.last(MessageGroup).Group; group.State != GroupPlaying {
		t.Errorf("the guest it waited for dropped: %+v; want Playing", group)
	}
}

// The roster lists the members in the order they joined, the host first, every time it is sent:
// kept by id in a map, its order was chance, and the names swapped on everyone's screen.
func TestHub_TheRosterListsTheMembersInTheOrderTheyJoined(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	_, hostBox := stage.attach(t, stage.opened.MembershipCredential, RolePlayback)
	var credentials []string
	for range 3 {
		credentials = append(credentials, stage.join(t).MembershipCredential)
	}
	for range 20 {
		attached, _ := stage.attach(t, credentials[2], RolePlayback)
		roster := hostBox.last(MessageRoster).Roster
		if len(roster) != 4 || !roster[0].IsHost ||
			!slices.IsSortedFunc(roster, func(one, other MemberView) int { return cmp.Compare(one.ID, other.ID) }) {
			t.Fatalf("the roster %+v; want the host and then each member in the order they joined", roster)
		}
		stage.hub.Detach(attached)
	}
}

// A guest whose link gives them no rights controls nothing: their every command is refused (D246 §8,
// D271 §4).
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

// A guest's link says what they may do to playback: a guest given play and pause alone pauses the
// group and is refused a seek; one given the position alone seeks and is refused a pause; neither
// changes what plays, stops it or changes its rate, which stay the host's (D271 §4). Each is told
// their rights as they attach, so their player offers only those.
func TestHub_AGuestDoesToPlaybackWhatTheirLinkAllows(t *testing.T) {
	t.Parallel()
	for _, rights := range []Rights{{CanPlayPause: true}, {CanSeek: true}} {
		stage := newStage(t)
		host, _ := stage.attach(t, stage.opened.MembershipCredential, RolePlayback)
		stage.command(t, host, CommandChangeItem, 7)
		made, err := stage.hub.AddLink(stage.opened.SessionID, stage.owner.UserID, "Grandma", rights)
		if err != nil {
			t.Fatal(err)
		}
		joined, err := stage.hub.Join(made.Secret, "Gran", nil)
		if err != nil {
			t.Fatal(err)
		}
		guest, guestBox := stage.attach(t, joined.MembershipCredential, RolePlayback)
		if told := guestBox.last(MessageWelcome).Rights; told == nil || *told != rights {
			t.Errorf("the guest given %+v was welcomed with %+v", rights, told)
		}
		for command, isAllowed := range map[CommandKind]bool{
			CommandPause: rights.CanPlayPause, CommandPlay: rights.CanPlayPause, CommandSeek: rights.CanSeek,
			CommandChangeItem: false, CommandStop: false, CommandRate: false,
		} {
			before := len(guestBox.messages)
			stage.hub.Handle(t.Context(), guest, Inbound{
				Type: MessageCommand, Command: command, FileID: 9, PositionMs: 60_000, Rate: 1.1,
				BasedOnSeq: stage.hub.sessions[stage.opened.SessionID].group.seq,
			})
			isRefused := slices.ContainsFunc(guestBox.messages[before:], func(message Outbound) bool {
				return message.Type == MessageRejected && message.Code == codeForbidden
			})
			if isRefused == isAllowed {
				t.Errorf("given %+v, the guest's %s was refused: %v; want refused: %v",
					rights, command, isRefused, !isAllowed)
			}
		}
	}
}

// Each link starts with the rights of the last one made, the first with every right (D271 §4).
func TestHub_ANewLinkStartsWithTheLastOnesRights(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	details, err := stage.hub.DetailsOf(stage.opened.SessionID, stage.owner.UserID)
	if err != nil {
		t.Fatal(err)
	}
	if details.NextRights != everyRight() {
		t.Errorf("the first link starts with %+v; want every right", details.NextRights)
	}
	_, err = stage.hub.AddLink(stage.opened.SessionID, stage.owner.UserID, "a cousin", Rights{CanSeek: true})
	if err != nil {
		t.Fatal(err)
	}
	if details, err = stage.hub.DetailsOf(stage.opened.SessionID, stage.owner.UserID); err != nil {
		t.Fatal(err)
	}
	if details.NextRights != (Rights{CanSeek: true}) || details.Links[0].Rights != (Rights{CanSeek: true}) {
		t.Errorf("after a link given the position alone: next %+v, link %+v; want both that", details.NextRights,
			details.Links[0].Rights)
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
	_, err := stage.hub.AddLink(stage.opened.SessionID, stage.owner.UserID, "", Rights{})
	if !errors.Is(err, ErrNoNote) {
		t.Errorf("a link with no note = %v; want ErrNoNote", err)
	}
	made, err := stage.hub.AddLink(stage.opened.SessionID, stage.owner.UserID, "Grandma", Rights{})
	if err != nil {
		t.Fatal(err)
	}
	guest, err := stage.hub.Join(made.Secret, "Gran", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = stage.hub.Join(made.Secret, "stranger", nil); !errors.Is(err, ErrNoSession) {
		t.Errorf("a second join by one link = %v; want ErrNoSession", err)
	}
	for _, member := range hostBox.last(MessageRoster).Roster {
		if member.ID == guest.MemberID && member.Name != "Gran" {
			t.Errorf("the roster names the guest %q; want the name they chose", member.Name)
		}
	}
	details, err := stage.hub.DetailsOf(stage.opened.SessionID, stage.owner.UserID)
	if err != nil || len(details.Links) != 1 || details.Links[0].UsedBy != guest.MemberID ||
		!slices.ContainsFunc(details.Members, func(each MemberDetail) bool { return each.HostNote == "Grandma" }) {
		t.Errorf("the host's view = %+v (%v); want the link used by Gran, noted Grandma", details, err)
	}
	unused, err := stage.hub.AddLink(stage.opened.SessionID, stage.owner.UserID, "a cousin", Rights{})
	if err != nil {
		t.Fatal(err)
	}
	if err = stage.hub.RemoveLink(stage.opened.SessionID, stage.owner.UserID, unused.LinkID); err != nil {
		t.Fatal(err)
	}
	if _, err = stage.hub.Join(unused.Secret, "cousin", nil); !errors.Is(err, ErrNoSession) {
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
	_, err = stage.hub.Attach(guest.MembershipCredential, RolePlayback, true, nil, testTuning, (&inbox{}).send)
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
	_, err = stage.hub.Attach(again.MembershipCredential, RolePlayback, true, nil, testTuning, (&inbox{}).send)
	if err != nil {
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

// A device that says even its lowest quality is beyond its link is shown so in everyone's roster,
// with both numbers, until it says it keeps up again (D247 §8).
func TestHub_TheRosterTellsEveryoneADeviceCannotKeepUp(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	_, hostBox := stage.attach(t, stage.opened.MembershipCredential, RolePlayback)
	guest := stage.join(t)
	guestDevice, _ := stage.attach(t, guest.MembershipCredential, RolePlayback)
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
	host, hostBox := stage.attach(t, stage.opened.MembershipCredential, RolePlayback)
	stage.ready(t, host, false)
	stage.command(t, host, CommandChangeItem, 7)
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

// playingStage is a stage with the host's and a guest's players both ready and film 7 playing.
type playingStage struct {
	sessionStage
	host, guestDevice Attachment
	hostBox           *inbox
	guest             Joined
}

func playing(t *testing.T) playingStage {
	t.Helper()
	stage := newStage(t)
	host, hostBox := stage.attach(t, stage.opened.MembershipCredential, RolePlayback)
	guest := stage.join(t)
	guestDevice, _ := stage.attach(t, guest.MembershipCredential, RolePlayback)
	stage.ready(t, host, false)
	stage.ready(t, guestDevice, false)
	stage.command(t, host, CommandChangeItem, 7)
	if state := hostBox.last(MessageGroup).Group.State; state != GroupPlaying {
		t.Fatalf("film 7 put on with both ready: %s, want Playing", state)
	}
	return playingStage{sessionStage: stage, host: host, guestDevice: guestDevice, hostBox: hostBox, guest: guest}
}

// Everyone's roster says where each player stands: what its page waits on, its person's speakers or
// a tap; once a film is on, loading until it is first ready, then watching. Nothing of it moves whom
// the group waits for (D246 §6). A guest still choosing her speakers was shown as nothing at all
// while the group played on without her (the owner's Watch Together session, 2026-10-18).
func TestHub_TheRosterSaysWhereEachPlayerStands(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	host, hostBox := stage.attach(t, stage.opened.MembershipCredential, RolePlayback)
	guest := stage.join(t)
	guestDevice, guestBox := stage.attach(t, guest.MembershipCredential, RolePlayback)
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
	for _, standing := range []Standing{StandingNeedsTap, StandingChoosingSound} {
		stage.hub.Handle(t.Context(), guestDevice, Inbound{Type: MessageStanding, Standing: standing})
		if got := guestStatus(); got != DeviceStatus(standing) {
			t.Errorf("its page waiting on %q: %q; want the same", standing, got)
		}
	}
	stage.ready(t, host, false)
	stage.command(t, host, CommandChangeItem, 7)
	if got := hostBox.last(MessageGroup).Group.State; got != GroupPlaying {
		t.Errorf("the group with the guest choosing her speakers is %q; want it playing, waiting for no one", got)
	}
	stage.command(t, host, CommandStop, 0)
	stage.hub.Handle(t.Context(), guestDevice, Inbound{Type: MessageStanding, Standing: StandingNone})
	if got := guestStatus(); got != "" {
		t.Errorf("speakers chosen, nothing on: %q; want none", got)
	}
	stage.command(t, host, CommandChangeItem, 8) // the roster comes with the group's change
	if got := guestStatus(); got != StatusLoading {
		t.Errorf("another film on: %q; want loading", got)
	}
	stage.ready(t, guestDevice, false)
	if got := guestStatus(); got != StatusWatching {
		t.Errorf("once ready: %q; want watching", got)
	}
	stage.hub.Handle(t.Context(), guestDevice, Inbound{Type: MessageStanding, Standing: "dancing"})
	if got := guestBox.last(MessageRejected).Code; got != codeInvalid {
		t.Errorf("a standing no page says was answered %q; want it refused", got)
	}
	stage.hub.Handle(t.Context(), guestDevice, Inbound{Type: MessageStanding, Standing: StandingNeedsTap})
	if !strings.Contains(stage.logs.String(), "standing=needsTap") {
		t.Error("a page's standing was not logged")
	}
}
