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
	joined, err := stage.hub.Join(made.Secret, "partner", nil, Leaving{})
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
		Type: MessageState, IsPlaying: !isBuffering, IsBuffering: isBuffering,
		PositionMs: stage.groupPositionMs(attachment.SessionID), FileID: stage.filmOn(attachment.SessionID),
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
	waiting := fmt.Sprintf("state=waiting event=changedItem byMemberId=%d fileId=7", host.MemberID)
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
	stage.hub.Detach(guestDevice, GoneClosed)
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
		stage.hub.Detach(attached, GoneClosed)
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
	for _, rights := range []Rights{{CanPlayPause: true}, {CanSeek: true}, {CanNext: true}} {
		stage := newStage(t)
		host, _ := stage.attach(t, stage.opened.MembershipCredential, RoleVideo)
		stage.command(t, host, CommandChangeItem, 7)
		made, err := stage.hub.AddLink(stage.opened.SessionID, stage.owner.UserID, "Grandma", rights)
		if err != nil {
			t.Fatal(err)
		}
		joined, err := stage.hub.Join(made.Secret, "Gran", nil, Leaving{})
		if err != nil {
			t.Fatal(err)
		}
		guest, guestBox := stage.attach(t, joined.MembershipCredential, RoleVideo)
		if told := guestBox.last(MessageWelcome).Rights; told == nil || *told != rights {
			t.Errorf("the guest given %+v was welcomed with %+v", rights, told)
		}
		for command, isAllowed := range map[CommandKind]bool{
			CommandPause: rights.CanPlayPause, CommandPlay: rights.CanPlayPause, CommandSeek: rights.CanSeek,
			CommandNext: rights.CanNext, CommandPrevious: rights.CanNext,
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

// A guest whose link gives canNext puts on the episode after or before the session's film, the one
// the host's ordering has there, from where the host is partway through it, whatever film the
// command names; one without it is refused and the film stays on; a next with none after it is not
// found; and one found from a film that has since changed is stale (D471 §2).
func TestHub_AGuestPutsOnTheNextOrPreviousEpisodeOnlyWithTheRight(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	stage.hub.MovesAlongBy(neighbourOneAlong(stage.owner.UserID))
	host, _ := stage.attach(t, stage.opened.MembershipCredential, RoleVideo)
	stage.command(t, host, CommandChangeItem, 7)
	guestOf := func(rights Rights) (Attachment, *inbox) { return stage.guestWith(t, rights) }
	refusal := refusalSince
	withheld, withheldBox := guestOf(Rights{CanPlayPause: true, CanSeek: true})
	stage.command(t, withheld, CommandNext, 0)
	if code := refusal(withheldBox, 0); code != codeForbidden || stage.filmOn(stage.opened.SessionID) != 7 {
		t.Errorf("a guest without canNext moved on: refused %q, film %d; want forbidden, 7", code,
			stage.filmOn(stage.opened.SessionID))
	}
	given, givenBox := guestOf(Rights{CanNext: true})
	stage.command(t, given, CommandNext, 99)
	if film := stage.filmOn(stage.opened.SessionID); film != 8 {
		t.Errorf("a guest's next, naming 99, put on %d; want 8, the host's next after 7", film)
	}
	stage.command(t, given, CommandPrevious, 0)
	if film, at := stage.filmOn(stage.opened.SessionID), stage.groupPositionMs(stage.opened.SessionID); film != 7 ||
		at != 120_000 {
		t.Errorf("a guest's previous put on %d at %d ms; want 7 at 120000, where the host is partway", film, at)
	}
	stage.command(t, host, CommandChangeItem, 20)
	before := len(givenBox.messages)
	stage.command(t, given, CommandNext, 0)
	if code := refusal(givenBox, before); code != codeNotFound || stage.filmOn(stage.opened.SessionID) != 20 {
		t.Errorf("a next after the last: refused %q, film %d; want not found, 20", code,
			stage.filmOn(stage.opened.SessionID))
	}
	before = len(givenBox.messages)
	stage.hub.Handle(t.Context(), given, Inbound{Type: MessageCommand, Command: CommandNext, movedFrom: 7})
	if code := refusal(givenBox, before); code != codeNotFound {
		t.Errorf("a next found afresh refused %q; want not found: its film is the session's, found again", code)
	}
	before = len(givenBox.messages)
	session := stage.hub.sessions[stage.opened.SessionID]
	stage.hub.mutex.Lock()
	session.command(stage.clock.Now(), session.members[given.MemberID],
		session.members[given.MemberID].devices[given.DeviceID],
		Inbound{Type: MessageCommand, Command: CommandNext, FileID: 8, movedFrom: 7})
	stage.hub.mutex.Unlock()
	if code := refusal(givenBox, before); code != codeStale || stage.filmOn(stage.opened.SessionID) != 20 {
		t.Errorf("a next found from 7 while 20 is on: refused %q, film %d; want stale, 20", code,
			stage.filmOn(stage.opened.SessionID))
	}
}

// neighbourOneAlong is a host's ordering in which the film after each is the next id and the one
// before the last, from two minutes in, with none after or before 20; another user has none.
func neighbourOneAlong(hostUserID int64) Neighbour {
	return func(_ context.Context, userID, fileID int64, isNext bool) (int64, int64, bool) {
		switch {
		case userID != hostUserID || fileID == 20:
			return 0, 0, false
		case isNext:
			return fileID + 1, 0, true
		default:
			return fileID - 1, 120_000, true
		}
	}
}

// guestWith is a guest joined by a link giving rights, attached with its video.
func (stage sessionStage) guestWith(t *testing.T, rights Rights) (Attachment, *inbox) {
	t.Helper()
	made, err := stage.hub.AddLink(stage.opened.SessionID, stage.owner.UserID, "a cousin", rights)
	if err != nil {
		t.Fatal(err)
	}
	joined, err := stage.hub.Join(made.Secret, "Cousin", nil, Leaving{})
	if err != nil {
		t.Fatal(err)
	}
	return stage.attach(t, joined.MembershipCredential, RoleVideo)
}

// refusalSince is the code of the first refusal box was sent after its first before messages, or "".
func refusalSince(box *inbox, before int) string {
	for _, message := range box.messages[before:] {
		if message.Type == MessageRejected {
			return message.Code
		}
	}
	return ""
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
	guest, err := stage.hub.Join(made.Secret, "Gran", nil, Leaving{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = stage.hub.Join(made.Secret, "stranger", nil, Leaving{}); !errors.Is(err, ErrNoSession) {
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
	if _, err = stage.hub.Join(unused.Secret, "cousin", nil, Leaving{}); !errors.Is(err, ErrNoSession) {
		t.Errorf("a join by a withdrawn link = %v; want ErrNoSession", err)
	}
}
