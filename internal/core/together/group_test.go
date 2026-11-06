// SPDX-License-Identifier: AGPL-3.0-or-later

package together

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/testutil"
)

// report is attachment's state: where its player, which has the session's film, is, whether it
// plays, and whether it buffers, its round trip 40 ms.
func (stage sessionStage) report(t *testing.T, attachment Attachment, positionMs int64, isPlaying, isBuffering bool) {
	t.Helper()
	stage.hub.Handle(t.Context(), attachment, Inbound{
		Type: MessageState, PositionMs: positionMs, Playing: isPlaying, Buffering: isBuffering, RTTMs: 40,
		FileID: stage.filmOn(attachment.SessionID),
	})
}

// hide says attachment's page is hidden by its browser, its film paused, or shown again (D372 §5).
func (stage sessionStage) hide(t *testing.T, attachment Attachment, isHidden bool) {
	t.Helper()
	stage.hub.Handle(t.Context(), attachment, Inbound{
		Type: MessageState, PositionMs: stage.groupPositionMs(attachment.SessionID), Buffering: true,
		PageHidden: isHidden, FileID: stage.filmOn(attachment.SessionID),
	})
}

// filmOn is the film the session has on, which a player following it has loaded; 0 for none.
func (stage sessionStage) filmOn(sessionID int64) int64 {
	stage.hub.mutex.Lock()
	defer stage.hub.mutex.Unlock()
	if held, found := stage.hub.sessions[sessionID]; found {
		return held.group.fileID
	}
	return 0
}

// pongGroup is the group as a pong to attachment says it is now.
func (stage sessionStage) pongGroup(t *testing.T, attachment Attachment, box *inbox) *GroupView {
	t.Helper()
	stage.hub.Handle(t.Context(), attachment, Inbound{Type: MessagePing, TMono: 1})
	return box.last(MessagePong).Group
}

// A film the host's player puts on stands where it is put on for that player, its leader, alone:
// a follower still loading holds no one up, and the group plays from where the leader plays once
// it does, at once, with no lead to start on (D371 §1, §2, §6).
func TestHub_AFilmPutOnStandsForItsLeaderAloneAndPlaysAsTheLeaderPlays(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	host, hostBox := stage.attach(t, stage.opened.MembershipCredential, RoleVideo)
	guest := stage.join(t)
	guestDevice, _ := stage.attach(t, guest.MembershipCredential, RoleVideo)
	stage.report(t, guestDevice, 0, false, true)
	stage.hub.Handle(t.Context(), host, Inbound{
		Type: MessageCommand, Command: CommandChangeItem, FileID: 7, PositionMs: 30_000,
	})
	if group := hostBox.last(MessageGroup).Group; group.State != GroupWaiting || group.Event != EventChangedItem ||
		group.PositionMs != 30_000 {
		t.Fatalf("film 7 put on at 30 s: %+v; want Waiting there for its leader", group)
	}
	stage.report(t, guestDevice, 30_000, true, false)
	if group := hostBox.last(MessageGroup).Group; group.State != GroupWaiting {
		t.Errorf("the guest ready, the leader not yet playing: %+v; want still Waiting, for the leader alone", group)
	}
	stage.clock.Advance(3 * time.Second)
	stage.report(t, host, 30_500, true, false)
	group := hostBox.last(MessageGroup).Group
	if group.State != GroupPlaying || group.Event != EventLeaderPlays || group.ByMemberID != host.MemberID ||
		group.PositionMs != 30_520 {
		t.Errorf("the leader plays at 30.5 s, its round trip 40 ms: %+v; want Playing from 30.52 s", group)
	}
	stage.clock.Advance(time.Second)
	if moving := stage.pongGroup(t, host, hostBox); moving.PositionMs != 31_520 {
		t.Errorf("a second on, the group is at %d ms; want 31.52 s, moving at once with no lead", moving.PositionMs)
	}
	stage.report(t, guestDevice, 30_000, true, true)
	stage.hub.Detach(guestDevice, GoneClosed)
	if changed := hostBox.last(MessageGroup).Group; changed.Event != EventLeaderPlays {
		t.Errorf("the guest stalled and dropped: %+v; want the group as it was, waiting for no follower", changed)
	}
}

// The group's position is its leader's: each state the leader reports while the group plays places
// the group where it is, with or without followers, and nothing corrects the leader (D371 §1); a
// leader's stall holds no one: the group plays on, and the leader playing again within its grace
// catches up to it, the group never stepping back (D373 §1, §2, D374). A follower's stall changes
// nothing: it catches up (D371 §2).
func TestHub_TheGroupIsWhereItsLeaderIsAndAFollowersStallChangesNothing(t *testing.T) {
	t.Parallel()
	stage := playing(t)
	host, hostBox := stage.host, stage.hostBox
	stage.clock.Advance(10 * time.Second)
	stage.report(t, host, 9_200, true, false)
	stage.clock.Advance(time.Second)
	if group := stage.pongGroup(t, host, hostBox); group.State != GroupPlaying || group.PositionMs != 10_220 {
		t.Errorf("a second after the leader said it plays at 9.2 s, 40 ms its round trip: %+v; "+
			"want Playing at 10.22 s, where it is, with a follower along", group)
	}
	seq := hostBox.last(MessageGroup).Seq
	stage.report(t, stage.guestDevice, 4_000, true, true)
	if hostBox.last(MessageGroup).Seq != seq {
		t.Errorf("a follower's stall changed the group: %+v", hostBox.last(MessageGroup).Group)
	}
	stage.report(t, host, 10_300, true, true)
	if hostBox.last(MessageGroup).Seq != seq {
		t.Fatalf("the leader's stall at 10.3 s changed the group: %+v; want it playing on, untold (D373)",
			hostBox.last(MessageGroup).Group)
	}
	stage.clock.Advance(2 * time.Second)
	if group := stage.pongGroup(t, host, hostBox); group.State != GroupPlaying || group.PositionMs != 12_220 {
		t.Errorf("2 s into its leader's stall, the group: %+v; want Playing on at 12.22 s", group)
	}
	stage.report(t, host, 10_300, true, false)
	if at := stage.groupPositionMs(stage.opened.SessionID); at != 12_220 || hostBox.last(MessageGroup).Seq != seq {
		t.Errorf("the leader plays again at 10.3 s: the group at %d ms, %+v; want it playing on at 12.22 s, "+
			"untold, for the leader to catch up to (D374)", at, hostBox.last(MessageGroup).Group)
	}
	line := testutil.LineWith(t, stage.logs.String(), eventFollowedLeader)
	if !strings.Contains(line, "steppedMs=-780") {
		t.Errorf("the group's first step to its leader logged as %q; want steppedMs=-780", line)
	}
}

// Another device's play or seek, a cast's remote's or a guest's, stands for the leader, which carries
// it out as its own press, and the group plays from where the leader then plays; a pause pauses the
// group at once. Without a leader, a play or a seek plays at once, everyone catching up (D371 §2, §4).
func TestHub_ACommandOfAnotherDeviceStandsForTheLeaderToCarryItOut(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	cast, err := stage.hub.OpenCast(stage.owner)
	if err != nil {
		t.Fatal(err)
	}
	receiver, _ := stage.attach(t, cast.ReceiverCredential, RoleVideo)
	remote, remoteBox := stage.attach(t, cast.SenderCredential, RolePanel)
	stage.command(t, receiver, CommandChangeItem, 12)
	stage.report(t, receiver, 0, true, false)
	stage.command(t, remote, CommandPause, 0)
	if group := remoteBox.last(MessageGroup).Group; group.State != GroupPaused {
		t.Errorf("the remote's pause: %+v; want Paused", group)
	}
	stage.command(t, remote, CommandPlay, 0)
	if group := remoteBox.last(MessageGroup).Group; group.State != GroupWaiting || group.Event != EventPlayed {
		t.Errorf("the remote's play: %+v; want Waiting for the receiver to play", group)
	}
	stage.report(t, receiver, 0, true, false)
	stage.hub.Handle(t.Context(), remote, Inbound{
		Type: MessageCommand, Command: CommandSeek, PositionMs: 600_000, BasedOnSeq: remoteBox.last(MessageGroup).Seq,
	})
	if group := remoteBox.last(MessageGroup).Group; group.State != GroupWaiting || group.PositionMs != 600_000 {
		t.Errorf("the remote's seek to 10 min: %+v; want Waiting there for the receiver", group)
	}
	stage.report(t, receiver, 600_000, true, false)
	if group := remoteBox.last(MessageGroup).Group; group.State != GroupPlaying {
		t.Errorf("the receiver plays where it was sought: %+v; want Playing", group)
	}

	leaderless := newStage(t)
	panel, panelBox := leaderless.attach(t, leaderless.opened.MembershipCredential, RolePanel)
	guest := leaderless.join(t)
	guestDevice, _ := leaderless.attach(t, guest.MembershipCredential, RoleVideo)
	leaderless.hide(t, guestDevice, true)
	leaderless.command(t, panel, CommandChangeItem, 7)
	if group := panelBox.last(MessageGroup).Group; group.State != GroupPlaying {
		t.Errorf("a film put on from a panel, no one leading, a follower loading: %+v; want Playing at once", group)
	}
}

// The host's device that puts a film on leads, as it does, and the host's newest player leads as it
// first plays the film; the leader leaving hands the lead to the host's newest other player, and
// with none of the host's, to the player present longest, a guest's, which the group then stands for
// (D371 §3, D372 §1, D374 §1). Each change is logged with why, and the roster says who leads.
func TestHub_TheHostsPlayerLeadsAndAnotherTakesTheLeadAsItLeaves(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	_, watchBox := stage.attach(t, stage.opened.MembershipCredential, RolePanel)
	older, _ := stage.attach(t, stage.opened.MembershipCredential, RoleVideo)
	guest := stage.join(t)
	guestDevice, _ := stage.attach(t, guest.MembershipCredential, RoleVideo)
	leaderOf := func() int64 { return rosterLeader(watchBox) }
	stage.command(t, older, CommandChangeItem, 7)
	if leaderOf() != older.DeviceID {
		t.Errorf("the roster's leader is device %d; want the player that put the film on, %d",
			leaderOf(), older.DeviceID)
	}
	newer, _ := stage.attach(t, stage.opened.MembershipCredential, RoleBoth)
	stage.ready(t, newer, true)
	stage.ready(t, guestDevice, true)
	if leaderOf() != newer.DeviceID {
		t.Errorf("the roster's leader is device %d; want the newest of the host's players, %d",
			leaderOf(), newer.DeviceID)
	}
	stage.hub.Detach(newer, GoneClosed)
	if leaderOf() != older.DeviceID {
		t.Errorf("the roster's leader is device %d; want the host's other player, %d", leaderOf(), older.DeviceID)
	}
	stage.hub.Detach(older, GoneSilent)
	if leaderOf() != guestDevice.DeviceID {
		t.Errorf("the roster's leader is device %d; want the guest's player, the one left, %d",
			leaderOf(), guestDevice.DeviceID)
	}
	if group := watchBox.last(MessageGroup).Group; group.State != GroupWaiting || group.Event != EventChangedItem {
		t.Errorf("the group as the host's players left, a guest's leading: %+v; want Waiting for it as before", group)
	}
	logs := stage.logs.String()
	for _, want := range []string{
		fmt.Sprintf("leaderDeviceId=%d leaderMemberId=%d why=attached", newer.DeviceID, newer.MemberID),
		fmt.Sprintf("leaderDeviceId=%d leaderMemberId=%d why=putOn", older.DeviceID, older.MemberID),
		fmt.Sprintf("leaderDeviceId=%d leaderMemberId=%d why=leaderGone leaderIsHost=true previousLeaderDeviceId=%d",
			older.DeviceID, older.MemberID, newer.DeviceID),
		"goneBecause=closed goneLeaderWasHost=true",
		fmt.Sprintf("leaderDeviceId=%d leaderMemberId=%d why=leaderGone leaderIsHost=false previousLeaderDeviceId=%d",
			guestDevice.DeviceID, guestDevice.MemberID, older.DeviceID),
		"goneBecause=silent goneLeaderWasHost=true",
	} {
		if !strings.Contains(logs, want) {
			t.Errorf("no log line says %q:\n%s", want, logs)
		}
	}
}

// rosterLeader is the device the roster box was last told leads, or 0.
func rosterLeader(box *inbox) int64 {
	for _, each := range box.last(MessageRoster).Roster {
		for _, attached := range each.Devices {
			if attached.IsLeader {
				return attached.ID
			}
		}
	}
	return 0
}

// A panel never leads, nor, with nothing on, any player: there is no film to play (D374 §1); while
// none of the host's devices can lead, a guest's playing the film does, and the host's player that
// comes to play it while a guest's leads follows until in step (D372 §1, §3).
func TestHub_NoPanelLeadsAndAGuestsLeadsWhileNoneOfTheHostsCan(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	panel, panelBox := stage.attach(t, stage.opened.MembershipCredential, RolePanel)
	if leader := rosterLeader(panelBox); leader != 0 {
		t.Errorf("device %d leads, the host's panel alone attached; want none", leader)
	}
	guest := stage.join(t)
	guestDevice, _ := stage.attach(t, guest.MembershipCredential, RoleBoth)
	stage.ready(t, guestDevice, false)
	if leader := rosterLeader(panelBox); leader != 0 {
		t.Errorf("device %d leads, nothing on; want none", leader)
	}
	stage.command(t, panel, CommandChangeItem, 7)
	stage.ready(t, guestDevice, false)
	if leader := rosterLeader(panelBox); leader != guestDevice.DeviceID {
		t.Errorf("device %d leads; want the guest's player, %d, the only one", leader, guestDevice.DeviceID)
	}
	host, _ := stage.attach(t, stage.opened.MembershipCredential, RoleVideo)
	stage.report(t, host, 0, false, true)
	if leader := rosterLeader(panelBox); leader != guestDevice.DeviceID {
		t.Errorf("device %d leads, the host's player loading the film; want the guest's still, %d, until "+
			"the host's plays in step", leader, guestDevice.DeviceID)
	}
	if !strings.Contains(stage.logs.String(), fmt.Sprintf(
		"leaderDeviceId=%d leaderMemberId=%d why=noneLed leaderIsHost=false", guestDevice.DeviceID, guest.MemberID)) {
		t.Errorf("the guest's lead not logged as noneLed:\n%s", stage.logs.String())
	}
}
