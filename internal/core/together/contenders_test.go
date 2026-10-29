// SPDX-License-Identifier: AGPL-3.0-or-later

package together

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// noPlayer says attachment's page holds no player: its film handed to a cast's receiver, or its
// player gone, its connection kept (D374 §1).
func (stage sessionStage) noPlayer(t *testing.T, attachment Attachment) {
	t.Helper()
	stage.hub.Handle(t.Context(), attachment, Inbound{Type: MessageState})
}

// A Watch Together host who casts leads through the receiver, never the sender, whose page keeps
// its connection with no player: the sender is passed over at once, the lead passing, and the
// receiver, taking the host's part, leads as the host's newest player once it plays the film; the
// sender never leads while the receiver plays, nor after it goes, the log saying why it was passed
// over (D374 §1, §2).
func TestHub_ACastingSenderNeverLeadsAndItsReceiverDoes(t *testing.T) {
	t.Parallel()
	stage := withGuests(t, 1)
	_, watchBox := stage.attach(t, stage.opened.MembershipCredential, RolePanel)
	sender, guest := stage.host, stage.guests[0]
	stage.clock.Advance(5 * time.Second)
	stage.noPlayer(t, sender) // its film handed to the receiver
	if leader := rosterLeader(watchBox); leader != guest.DeviceID {
		t.Fatalf("device %d leads, the host's film handed to a receiver; want the guest's, %d, at once",
			leader, guest.DeviceID)
	}
	if line := logLineWith(stage.logs.String(), "goneBecause=noFilm"); !strings.Contains(line,
		fmt.Sprintf("passedOver=%d:noFilm", sender.DeviceID)) {
		t.Errorf("the sender's passing logged as %q; want it gone and passed over, its player having no film", line)
	}
	credential, isHost := stage.hub.TogetherPart(stage.owner.UserID)
	if !isHost {
		t.Fatal("the host's receiver was not given the host's part")
	}
	receiver, _ := stage.attach(t, credential, RoleVideo)
	at := stage.groupPositionMs(stage.opened.SessionID)
	stage.report(t, receiver, at, false, true) // loading the film
	stage.report(t, receiver, at-20, true, false)
	if leader := rosterLeader(watchBox); leader != receiver.DeviceID {
		t.Fatalf("device %d leads, the host's receiver playing the film in step; want it, %d",
			leader, receiver.DeviceID)
	}
	for range 5 {
		stage.clock.Advance(time.Second)
		stage.noPlayer(t, sender)
		stage.report(t, receiver, stage.groupPositionMs(stage.opened.SessionID), true, false)
		if leader := rosterLeader(watchBox); leader != receiver.DeviceID {
			t.Errorf("device %d leads as the receiver plays; want the receiver, %d, never the sender, %d",
				leader, receiver.DeviceID, sender.DeviceID)
		}
	}
	stage.hub.Detach(receiver, GoneClosed)
	if leader := rosterLeader(watchBox); leader != guest.DeviceID {
		t.Errorf("device %d leads, the receiver gone; want the guest's, %d, the host's sender passed over",
			leader, guest.DeviceID)
	}
}

// A player not playing the session's film, with no film yet or another, is passed over by every
// choice of leader, however senior, the host's too; the log says each passed over and why (D374 §1).
func TestHub_APlayerNotPlayingTheFilmIsPassedOverForTheLead(t *testing.T) {
	t.Parallel()
	stage := withGuests(t, 1)
	_, watchBox := stage.attach(t, stage.opened.MembershipCredential, RolePanel)
	playingGuest := stage.guests[0]
	stage.clock.Advance(time.Second)
	another, _ := stage.attach(t, stage.join(t).MembershipCredential, RoleVideo)
	stage.hub.Handle(t.Context(), another, Inbound{Type: MessageState, Playing: true, FileID: 8})
	notYet, _ := stage.attach(t, stage.join(t).MembershipCredential, RoleVideo)
	hostsOther, _ := stage.attach(t, stage.opened.MembershipCredential, RoleVideo)
	if leader := rosterLeader(watchBox); leader != stage.host.DeviceID {
		t.Errorf("device %d leads as players attach that play no film yet; want the host's, %d",
			leader, stage.host.DeviceID)
	}
	stage.hub.Detach(stage.host, GoneClosed)
	if leader := rosterLeader(watchBox); leader != playingGuest.DeviceID {
		t.Errorf("device %d leads, the host's player gone; want the guest's playing the film, %d, the host's "+
			"other player and the rest playing none of it", leader, playingGuest.DeviceID)
	}
	if line := logLineWith(stage.logs.String(), "why=leaderGone"); !strings.Contains(line, fmt.Sprintf(
		"passedOver=\"%d:otherFilm %d:noFilm %d:noFilm\"", another.DeviceID, notYet.DeviceID, hostsOther.DeviceID)) {
		t.Errorf("the lead's passing logged as %q; want each player passed over, and why", line)
	}
	stage.report(t, hostsOther, stage.groupPositionMs(stage.opened.SessionID), true, false)
	if leader := rosterLeader(watchBox); leader != hostsOther.DeviceID {
		t.Errorf("device %d leads, the host's other player playing the film in step; want it, %d",
			leader, hostsOther.DeviceID)
	}
}

// The host's player that puts a film on leads it while it loads, before it has said a word of its
// player, so a film put on still stands for its leader alone (D373 §4, D374 §1); stopping it leaves
// nothing on and no one leading.
func TestHub_TheHostsPlayerPuttingAFilmOnLeadsItWhileItLoads(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	host, hostBox := stage.attach(t, stage.opened.MembershipCredential, RoleVideo)
	guestDevice, _ := stage.attach(t, stage.join(t).MembershipCredential, RoleVideo)
	stage.command(t, host, CommandChangeItem, 7)
	if leader := rosterLeader(hostBox); leader != host.DeviceID {
		t.Errorf("device %d leads as the host's player puts film 7 on; want it, %d", leader, host.DeviceID)
	}
	stage.report(t, host, 0, false, true)
	stage.ready(t, guestDevice, false)
	if leader := rosterLeader(hostBox); leader != host.DeviceID {
		t.Errorf("device %d leads as the host's player loads its film; want it, %d", leader, host.DeviceID)
	}
	if group := hostBox.last(MessageGroup).Group; group.State != GroupWaiting {
		t.Errorf("the group as its leader loads: %+v; want Waiting for it", group)
	}
	stage.command(t, host, CommandStop, 0)
	if leader := rosterLeader(hostBox); leader != 0 {
		t.Errorf("device %d leads, nothing on; want none", leader)
	}
}

// With one contender left, its stall is waited for, as a film played alone waits: its group stands
// where it stalled, its clock still, however long, and plays on from there; with another playing the
// film, no one's stall is waited for (D374 §3, D373 §1).
func TestHub_TheOnlyContendersStallIsWaitedFor(t *testing.T) {
	t.Parallel()
	stage := withGuests(t, 1)
	_, watchBox := stage.attach(t, stage.opened.MembershipCredential, RolePanel)
	guest := stage.guests[0]
	stage.hide(t, guest, true) // a locked phone: its film paused, it contends no more
	stage.clock.Advance(5 * time.Second)
	stage.report(t, stage.host, 6_000, true, true)
	stage.clock.Advance(4 * stalledLeaderGrace)
	stage.stall(t, stage.host)
	group := stage.pongGroup(t, stage.host, stage.hostBox)
	if group.State != GroupWaiting || group.Event != EventWaitingFor || group.PositionMs != 6_000 {
		t.Errorf("the only contender stalled at 6 s, 12 s since: %+v; want Waiting there for it", group)
	}
	if leader := rosterLeader(watchBox); leader != stage.host.DeviceID {
		t.Errorf("device %d leads, the only contender stalled; want it still, %d", leader, stage.host.DeviceID)
	}
	stage.report(t, stage.host, 6_000, true, false)
	if group := stage.hostBox.last(MessageGroup).Group; group.State != GroupPlaying ||
		group.Event != EventLeaderPlays || group.PositionMs != 6_020 {
		t.Errorf("the only contender plays again at 6 s: %+v; want Playing on from there", group)
	}

	stage.hide(t, guest, false)
	stage.ready(t, guest, false)
	seq := stage.hostBox.last(MessageGroup).Seq
	stage.stall(t, stage.host)
	stage.clock.Advance(time.Second)
	if group := stage.pongGroup(t, stage.host, stage.hostBox); group.State != GroupPlaying ||
		stage.hostBox.last(MessageGroup).Seq != seq {
		t.Errorf("the leader stalled, another playing the film: %+v; want Playing on, untold", group)
	}
}
