// SPDX-License-Identifier: AGPL-3.0-or-later

package together

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// tellPlace says attachment's player is at positionMs, playing or paused, and whether it has a
// position of its own: it was playing the film with its session on its page (D377 §4).
func (stage sessionStage) tellPlace(t *testing.T, attachment Attachment, positionMs int64, isPlaying, isOwn bool) {
	t.Helper()
	stage.hub.Handle(t.Context(), attachment, Inbound{
		Type: MessageState, PositionMs: positionMs, Playing: isPlaying, RemembersPlace: isOwn,
		FileID: stage.filmOn(attachment.SessionID),
	})
}

// A follower that takes over from a live leader, gone as it leads, moves no one: it is in step with
// the leader it follows, so the group goes on where the core has it, whatever position the follower
// last told (D377 §4). The log says the position is the core's.
func TestHub_AFollowerTakingOverFromALiveLeaderMovesNoOne(t *testing.T) {
	t.Parallel()
	stage := withGuests(t, 1)
	stage.clock.Advance(10 * time.Second)
	guest := stage.guests[0]
	stage.tellPlace(t, guest, stage.groupPositionMs(stage.opened.SessionID)+800, true, true)
	stage.clock.Advance(2 * time.Second)
	coreMs := stage.groupPositionMs(stage.opened.SessionID)
	stage.hub.Detach(stage.host, GoneClosed)
	if now := stage.groupPositionMs(stage.opened.SessionID); now != coreMs {
		t.Errorf("the group as the guest's player took over is at %d ms; want the core's, %d ms", now, coreMs)
	}
	if line := logLineWith(stage.logs.String(), "why=leaderGone"); !strings.Contains(line,
		fmt.Sprintf("positionFrom=core positionMs=%d", coreMs)) {
		t.Errorf("the lead's passing logged as %q; want the core's position kept", line)
	}
}

// A group no one leads, held for want of a player, goes on from the position its first contender back
// presents, a device back from a dropped connection that kept its film and its place; one with none
// to present, a page loaded afresh, leaves it where it was held (D377 §4, §7). The log says which.
func TestHub_ALeaderlessGroupGoesOnFromThePositionItsNewLeaderPresents(t *testing.T) {
	t.Parallel()
	for _, presents := range []bool{true, false} {
		stage := withGuests(t, 1)
		_, watchBox := stage.attach(t, stage.opened.MembershipCredential, RolePanel)
		stage.clock.Advance(4 * time.Second)
		heldMs := stage.groupPositionMs(stage.opened.SessionID)
		stage.everyoneGoes(t)
		stage.clock.Advance(time.Minute)
		placeMs := heldMs + 5000 // it played on from what it had loaded before its connection dropped
		stage.tellPlace(t, stage.guests[0], placeMs, false, presents)
		wantMs, from := heldMs, positionFromCore
		if presents {
			wantMs, from = placeMs, positionFromLeader
		}
		back := watchBox.last(MessageGroup).Group
		if back.State != GroupWaiting || back.Event != EventResumedOnReturn || back.PositionMs != wantMs {
			t.Errorf("presents %t: the group as the guest's player came back: %+v; want it Waiting at %d ms",
				presents, back, wantMs)
		}
		if line := logLineWith(stage.logs.String(), "why=noneLed"); !strings.Contains(line,
			fmt.Sprintf("positionFrom=%s positionMs=%d", from, wantMs)) {
			t.Errorf("presents %t: the return's lead logged as %q; want positionFrom=%s", presents, line, from)
		}
	}
}

// The line for the lead is by run, not by arrival: a run starts as the core first hears a device ready
// with the film, and connecting alone starts none; a page hidden, or a stall past the grace, ends it,
// the device back at the end of the line once it is ready again. Each start and end is logged (D377 §2).
func TestHub_TheLineForTheLeadIsByRunNotByArrival(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	host, _ := stage.attach(t, stage.opened.MembershipCredential, RoleVideo)
	stage.command(t, host, CommandChangeItem, 7)
	stage.ready(t, host, false)
	early, _ := stage.attach(t, stage.join(t).MembershipCredential, RoleVideo)
	stage.clock.Advance(time.Second)
	late, _ := stage.attach(t, stage.join(t).MembershipCredential, RoleVideo)
	third, _ := stage.attach(t, stage.join(t).MembershipCredential, RoleVideo)
	stage.ready(t, late, false) // connected after the early one, ready before it
	stage.ready(t, third, false)
	stage.clock.Advance(time.Second)
	stage.ready(t, early, false)
	stage.hide(t, late, true) // its run ends
	stage.hide(t, late, false)
	stage.ready(t, late, false) // a new run, behind the others
	_, watchBox := stage.attach(t, stage.opened.MembershipCredential, RolePanel)
	stage.hub.Detach(host, GoneClosed)
	if leader := rosterLeader(watchBox); leader != third.DeviceID {
		t.Errorf("device %d leads, the host's player gone; want %d, whose run started first and lasts",
			leader, third.DeviceID)
	}
	logs := stage.logs.String()
	if line := logLineWith(logs, fmt.Sprintf("deviceId=%d logEvent=runEnded", late.DeviceID)); !strings.Contains(
		line, "why=hidden") {
		t.Errorf("the hidden device's run's end logged as %q; want why=hidden", line)
	}
	if line := logLineWith(logs, fmt.Sprintf("deviceId=%d logEvent=runStarted", early.DeviceID)); !strings.Contains(
		line, "runSince=2026-10-05T20:00:02.000000Z") {
		t.Errorf("the early device's run logged as %q; want it started as it was ready, at 2 s", line)
	}
	stage.report(t, third, stage.groupPositionMs(stage.opened.SessionID), false, true) // stalls
	stage.clock.Advance(stalledLeaderGrace)
	stage.pongGroup(t, early, watchBox) // the next message from anyone finds the stall past the grace
	stage.report(t, third, stage.groupPositionMs(stage.opened.SessionID), true, false)
	stage.hub.Detach(early, GoneClosed)
	if leader := rosterLeader(watchBox); leader != late.DeviceID {
		t.Errorf("device %d leads; want %d, ahead in line of the one whose stall outlasted the grace",
			leader, late.DeviceID)
	}
}

// A leader its browser pauses, not its person, while others play the film neither pauses the group
// nor keeps the lead past the grace: the group plays on, and the lead passes to one that plays, the
// log saying goneBecause=paused (D377 §5).
func TestHub_ALeadersOwnPauseNeitherStopsTheGroupNorHoldsTheLead(t *testing.T) {
	t.Parallel()
	stage := withGuests(t, 1)
	panel, watchBox := stage.attach(t, stage.opened.MembershipCredential, RolePanel)
	stage.report(t, stage.host, stage.groupPositionMs(stage.opened.SessionID), false, false)
	if group := stage.pongGroup(t, panel, watchBox); watchBox.count(MessageGroup) != 0 ||
		group.State != GroupPlaying {
		t.Errorf("the group as its leader's browser paused it: %+v; want it playing on, unchanged", group)
	}
	stage.clock.Advance(stalledLeaderGrace)
	stage.ready(t, stage.guests[0], false)
	if leader := rosterLeader(watchBox); leader != stage.guests[0].DeviceID {
		t.Errorf("device %d leads, its pause past the grace; want the guest's, %d", leader, stage.guests[0].DeviceID)
	}
	if line := logLineWith(stage.logs.String(), "why=leaderGone"); !strings.Contains(line, "goneBecause=paused") {
		t.Errorf("the lead's passing logged as %q; want goneBecause=paused", line)
	}
}
