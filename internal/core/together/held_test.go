// SPDX-License-Identifier: AGPL-3.0-or-later

package together

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// everyoneGoes hides the guest's player and closes the host's, the session's only contenders, so
// no one is left who can lead (the owner's Mac off Wi-Fi with the S23 locked, staging 2026-10-29).
func (stage guestsStage) everyoneGoes(t *testing.T) {
	t.Helper()
	stage.hide(t, stage.guests[0], true)
	stage.hub.Detach(stage.host, GoneClosed)
}

// A playing group whose last contender goes is held where it stood, its clock still for however long
// no one is there, and keeps that it was playing: the first contender back leads it on from there,
// the group standing for it (Waiting) until it plays, then Playing, with no one's press; the log
// says it was held playing, and the return, where (D375 §1, §2).
func TestHub_AGroupLeftWhilePlayingIsHeldAndPlaysOnAsAPlayerReturns(t *testing.T) {
	t.Parallel()
	stage := withGuests(t, 1)
	_, watchBox := stage.attach(t, stage.opened.MembershipCredential, RolePanel)
	stage.clock.Advance(4 * time.Second)
	heldMs := stage.groupPositionMs(stage.opened.SessionID)
	stage.everyoneGoes(t)
	stage.wantHeldPlaying(t, watchBox, heldMs)

	guest := stage.guests[0]
	stage.report(t, guest, heldMs, false, false) // shown again, its film paused where it was held
	back := watchBox.last(MessageGroup).Group
	if back.State != GroupWaiting || back.Event != EventResumedOnReturn || back.IsHeldPlaying ||
		back.PositionMs != heldMs || rosterLeader(watchBox) != guest.DeviceID {
		t.Fatalf("the group as the guest's player came back: %+v, led by %d; want it Waiting at %d ms for "+
			"the guest's, %d, to lead it on", back, rosterLeader(watchBox), heldMs, guest.DeviceID)
	}
	if line := logLineWith(stage.logs.String(), "event=resumedOnReturn"); !strings.Contains(line,
		fmt.Sprintf("positionMs=%d", heldMs)) {
		t.Errorf("the return logged as %q; want where the group goes on from", line)
	}
	stage.report(t, guest, heldMs, true, false)
	playing := watchBox.last(MessageGroup).Group
	if playing.State != GroupPlaying || playing.Event != EventLeaderPlays ||
		playing.PositionMs < heldMs || playing.PositionMs > heldMs+100 {
		t.Errorf("the group as its returned leader plays: %+v; want Playing on from %d ms", playing, heldMs)
	}
}

// wantHeldPlaying checks the group everyone just left while it played is held Paused at heldMs, to
// play on, the log saying so, and stays there for a minute with no one.
func (stage guestsStage) wantHeldPlaying(t *testing.T, watchBox *inbox, heldMs int64) {
	t.Helper()
	held := watchBox.last(MessageGroup).Group
	if held.State != GroupPaused || held.Event != EventEveryoneLeft || !held.IsHeldPlaying ||
		held.PositionMs != heldMs {
		t.Fatalf("the group as everyone went: %+v; want it held Paused at %d ms, to play on", held, heldMs)
	}
	if line := logLineWith(stage.logs.String(), "event=everyoneLeft"); !strings.Contains(line, "heldPlaying=true") {
		t.Errorf("everyone's leaving logged as %q; want it to say the group was held playing", line)
	}
	stage.clock.Advance(time.Minute)
	if now := stage.groupPositionMs(stage.opened.SessionID); now != heldMs {
		t.Errorf("the group a minute after everyone went is at %d ms; want it still at %d ms", now, heldMs)
	}
}

// A group a person paused, left by everyone, stays paused when a contender returns, as it does when
// a person pauses one held playing: the hold is no person's pause, and a person's pause is kept
// (D375 §1, §2).
func TestHub_AGroupLeftWhilePausedStaysPausedAsAPlayerReturns(t *testing.T) {
	t.Parallel()
	for _, pausedWhile := range []string{"before everyone went", "while held"} {
		stage := withGuests(t, 1)
		panel, watchBox := stage.attach(t, stage.opened.MembershipCredential, RolePanel)
		stage.clock.Advance(4 * time.Second)
		if pausedWhile == "before everyone went" {
			stage.command(t, panel, CommandPause, 0)
		}
		heldMs := stage.groupPositionMs(stage.opened.SessionID)
		stage.everyoneGoes(t)
		held := watchBox.last(MessageGroup).Group
		if held.State != GroupPaused || held.PositionMs != heldMs {
			t.Fatalf("paused %s: the group as everyone went: %+v; want Paused at %d ms", pausedWhile, held, heldMs)
		}
		if pausedWhile == "while held" {
			stage.command(t, panel, CommandPause, 0)
		} else if line := logLineWith(stage.logs.String(), "event=everyoneLeft"); !strings.Contains(line,
			"heldPlaying=false") {
			t.Errorf("everyone's leaving a paused group logged as %q; want heldPlaying=false", line)
		}
		if paused := watchBox.last(MessageGroup).Group; paused.IsHeldPlaying {
			t.Errorf("paused %s: the group %+v; want a person's pause, not to play on", pausedWhile, paused)
		}
		changes := watchBox.count(MessageGroup)
		stage.clock.Advance(time.Minute)
		stage.report(t, stage.guests[0], heldMs, false, false)
		if rosterLeader(watchBox) != stage.guests[0].DeviceID {
			t.Errorf("paused %s: device %d leads as the guest's player came back; want the guest's", pausedWhile,
				rosterLeader(watchBox))
		}
		if back := watchBox.last(MessageGroup).Group; watchBox.count(MessageGroup) != changes ||
			back.State != GroupPaused || back.PositionMs != heldMs {
			t.Errorf("paused %s: the group as the guest's player came back: %+v; want it still Paused at %d ms",
				pausedWhile, back, heldMs)
		}
	}
}

// A group held for want of a player is idle, so its idle end ends it (D375 §3, D346).
func TestHub_AGroupHeldForWantOfAPlayerEndsIdle(t *testing.T) {
	t.Parallel()
	stage := withGuests(t, 1)
	stage.clock.Advance(4 * time.Second)
	stage.hub.EndStale(startingLifetimes()) // played until now
	stage.everyoneGoes(t)
	stage.clock.Advance(StartingIdleEnd - time.Second)
	if ended := stage.hub.EndStale(startingLifetimes()); len(ended) != 0 {
		t.Errorf("held a second short of the idle end: EndStale ended %v; want none", ended)
	}
	stage.clock.Advance(time.Second)
	if ended := stage.hub.EndStale(startingLifetimes()); len(ended) != 1 {
		t.Errorf("held for the idle end: EndStale ended %v; want the session", ended)
	}
}
