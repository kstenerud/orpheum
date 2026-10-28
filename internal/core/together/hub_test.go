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

// count is how many messages of a type the device was sent.
func (box *inbox) count(kind MessageType) int {
	box.mutex.Lock()
	defer box.mutex.Unlock()
	count := 0
	for _, message := range box.messages {
		if message.Type == kind {
			count++
		}
	}
	return count
}

// testTuning is how the tests' devices are welcomed to follow their group.
var testTuning = Tuning{
	LeaveDriftUpToMs: 500, NudgeMostPercent: 10, SeekDriftBeyondMs: 3000, CatchUpBufferPercent: 80,
}

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

// ready says attachment's player plays where the group is, or buffers there.
func (stage sessionStage) ready(t *testing.T, attachment Attachment, isBuffering bool) {
	t.Helper()
	stage.hub.Handle(t.Context(), attachment, Inbound{
		Type: MessageState, Playing: !isBuffering, Buffering: isBuffering,
		PositionMs: stage.groupPositionMs(attachment.SessionID),
	})
}

// groupPositionMs is where the session's group is now.
func (stage sessionStage) groupPositionMs(sessionID int64) int64 {
	stage.hub.mutex.Lock()
	defer stage.hub.mutex.Unlock()
	if held, found := stage.hub.sessions[sessionID]; found {
		return held.positionAt(stage.clock.Now()).Milliseconds()
	}
	return 0
}

// A pong says where the group is as the server answers, so a follower places the group by the pong
// whose round trip it measured shortest (D264).
func TestHub_APongSaysWhereTheGroupIsAsItAnswers(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	host, box := stage.attach(t, stage.opened.MembershipCredential, RoleVideo)
	stage.command(t, host, CommandChangeItem, 7)
	stage.ready(t, host, false)
	stage.clock.Advance(12 * time.Second)
	stage.hub.Handle(t.Context(), host, Inbound{Type: MessagePing, TMono: 4321})
	pong := box.last(MessagePong)
	if pong.TMono != 4321 || pong.Group == nil || pong.Group.State != GroupPlaying ||
		pong.Group.PositionMs != 12_000 || pong.Seq != box.last(MessageGroup).Seq {
		t.Errorf("the pong is %+v (group %+v); want tMono 4321 and the group playing at 12 s, at its seq",
			pong, pong.Group)
	}
}

// Every change of the group is logged: what plays, the event and who caused it, and who leads it, so
// a report can be read from the log (D371). A group that would not play on staging could only be
// guessed at (2026-10-06).
func TestHub_LogsEachChangeOfTheGroupAndWhoLeadsIt(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	host, _ := stage.attach(t, stage.opened.MembershipCredential, RoleVideo)
	guest := stage.join(t)
	guestDevice, _ := stage.attach(t, guest.MembershipCredential, RoleVideo)
	stage.ready(t, guestDevice, true)
	stage.command(t, host, CommandChangeItem, 7)
	waiting := fmt.Sprintf("state=Waiting event=changedItem byMemberId=%d fileId=7", host.MemberID)
	leads := fmt.Sprintf("leaderDeviceId=%d", host.DeviceID)
	for line := range strings.Lines(stage.logs.String()) {
		if strings.Contains(line, `msg="watch together: the group changed"`) && strings.Contains(line, waiting) &&
			strings.Contains(line, leads) {
			return
		}
	}
	t.Errorf("no log line says the group stands on film 7 for its leader, device %d:\n%s", host.DeviceID,
		stage.logs.String())
}

// Someone who joins while the film plays holds no one up while they choose their speakers and
// load, nor once ready, buffering: a follower's trouble is theirs to catch up on (D246 §6, D371 §2).
func TestHub_AJoiningViewerHoldsNoOneUp(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	host, hostBox := stage.attach(t, stage.opened.MembershipCredential, RoleVideo)
	stage.command(t, host, CommandChangeItem, 7)
	stage.ready(t, host, false)
	if state := hostBox.last(MessageGroup).Group.State; state != GroupPlaying {
		t.Fatalf("film 7 put on with only the host, ready: %s, want Playing", state)
	}
	guest := stage.join(t)
	guestDevice, guestBox := stage.attach(t, guest.MembershipCredential, RoleVideo)
	stage.ready(t, guestDevice, true)
	if state := hostBox.last(MessageGroup).Group.State; state != GroupPlaying {
		t.Errorf("a viewer joining and loading: %s, want Playing on without them", state)
	}
	if welcome := guestBox.last(MessageWelcome); welcome.Group.FileID != 7 || welcome.Group.State != GroupPlaying {
		t.Errorf("the joining viewer's welcome = %+v; want film 7 playing, to start where it is", welcome.Group)
	}
	seq := hostBox.last(MessageGroup).Seq
	stage.ready(t, guestDevice, false)
	stage.ready(t, guestDevice, true)
	if hostBox.last(MessageGroup).Seq != seq {
		t.Errorf("the viewer, once ready, buffering: %+v, want the group playing on, unchanged (D371 §2)",
			hostBox.last(MessageGroup).Group)
	}
}

// A drop stops no one (D371 §2): a playing group plays on while a follower is gone, and the host's
// play, with a follower back and loading, stands for the leader alone.
func TestHub_ADroppedPlayerStopsNoOne(t *testing.T) {
	t.Parallel()
	stage := playing(t)
	host, hostBox, guestDevice := stage.host, stage.hostBox, stage.guestDevice
	seq := hostBox.last(MessageGroup).Seq
	stage.hub.Detach(guestDevice)
	if hostBox.last(MessageGroup).Seq != seq || hostBox.last(MessageGroup).Group.State != GroupPlaying {
		t.Errorf("the guest's device dropped: %+v; want the group playing on, unchanged", hostBox.last(MessageGroup))
	}
	back, _ := stage.attach(t, stage.guest.MembershipCredential, RoleVideo)
	stage.ready(t, back, true)
	stage.command(t, host, CommandPause, 0)
	stage.command(t, host, CommandPlay, 0)
	stage.ready(t, host, false)
	if group := hostBox.last(MessageGroup).Group; group.State != GroupPlaying {
		t.Errorf("the host's play, its player playing, the guest loading: %+v; want Playing", group)
	}
}

// The roster lists the members in the order they joined, the host first, every time it is sent:
// kept by id in a map, its order was chance, and the names swapped on everyone's screen.
func TestHub_TheRosterListsTheMembersInTheOrderTheyJoined(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	_, hostBox := stage.attach(t, stage.opened.MembershipCredential, RoleVideo)
	var credentials []string
	for range 3 {
		credentials = append(credentials, stage.join(t).MembershipCredential)
	}
	for range 20 {
		attached, _ := stage.attach(t, credentials[2], RoleVideo)
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
	host, _ := stage.attach(t, stage.opened.MembershipCredential, RoleVideo)
	stage.command(t, host, CommandChangeItem, 7)
	guest := stage.join(t)
	guestDevice, guestBox := stage.attach(t, guest.MembershipCredential, RoleVideo)
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
		host, _ := stage.attach(t, stage.opened.MembershipCredential, RoleVideo)
		stage.command(t, host, CommandChangeItem, 7)
		made, err := stage.hub.AddLink(stage.opened.SessionID, stage.owner.UserID, "Grandma", rights)
		if err != nil {
			t.Fatal(err)
		}
		joined, err := stage.hub.Join(made.Secret, "Gran", nil)
		if err != nil {
			t.Fatal(err)
		}
		guest, guestBox := stage.attach(t, joined.MembershipCredential, RoleVideo)
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
	host, _ := stage.attach(t, stage.opened.MembershipCredential, RoleVideo)
	guest := stage.join(t)
	_, guestBox := stage.attach(t, guest.MembershipCredential, RoleVideo)
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
	host, hostBox := stage.attach(t, stage.opened.MembershipCredential, RoleVideo)
	hostPhone, phoneBox := stage.attach(t, stage.opened.MembershipCredential, RoleVideo)
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
	_, hostBox := stage.attach(t, stage.opened.MembershipCredential, RoleVideo)
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
	guest, err := stage.hub.Join(first.Secret, "partner", nil)
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
	chrome, err := stage.hub.Join(moved.Secret, "partner on chrome", nil)
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
	if _, err = stage.hub.Join(again.Secret, "someone", nil); !errors.Is(err, ErrNoSession) {
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
	_, err = stage.hub.Attach(again.MembershipCredential, RoleVideo, true, nil, testTuning, (&inbox{}).send)
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
	stage.hub.Handle(t.Context(), receiver, Inbound{Type: MessageState, Playing: true, Playback: &converted})
	if got := receiverInRoster(); !got.IsLeader || got.Playback == nil || *got.Playback != converted {
		t.Errorf("the receiver in the remote's roster = %+v (%+v); want the leader, converted at 4200 kb/s "+
			"on 18000", got, got.Playback)
	}
	stage.hub.Handle(t.Context(), receiver, Inbound{Type: MessageState, Playing: true})
	if got := receiverInRoster().Playback; got == nil || *got != converted {
		t.Errorf("after a state that says none, the receiver plays %+v; want the last said", got)
	}
	stage.hub.Handle(t.Context(), receiver, Inbound{
		Type: MessageState, Playing: true, Playback: &Playback{Delivery: "teleported"},
	})
	if got := receiverInRoster().Playback; got == nil || *got != converted {
		t.Errorf("after a delivery no player says, the receiver plays %+v; want the last known", got)
	}
}
