// SPDX-License-Identifier: AGPL-3.0-or-later

package together

import (
	"strings"
	"testing"
	"time"
)

// A group that starts playing at a sync point's end starts moving the start's lead from then, and
// says what is left of it, so every player starts at once rather than each as the message reaches
// it; one that plays on moves on as it was (D268 §3).
func TestHub_AGroupStartsMovingTheStartsLeadAfterItsSyncPointEnds(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	tuning := testTuning
	tuning.StartLeadMs = 500
	box := &inbox{}
	host, err := stage.hub.Attach(stage.opened.MembershipCredential, RolePlayback, true, nil, tuning, box.send)
	if err != nil {
		t.Fatal(err)
	}
	stage.ready(t, host, false)
	stage.command(t, host, CommandChangeItem, 7)
	if group := box.last(MessageGroup).Group; group.State != GroupPlaying || group.StartsInMs != 500 ||
		group.PositionMs != 0 {
		t.Fatalf("as the sync point ends: %+v; want Playing from 0, starting in 500 ms", group)
	}
	if !strings.Contains(stage.logs.String(), "startsInMs=500") {
		t.Errorf("the log does not say the start's lead:\n%s", stage.logs.String())
	}
	for _, step := range []struct {
		advance                time.Duration
		positionMs, startsInMs int64
	}{
		{200 * time.Millisecond, 0, 300},
		{1300 * time.Millisecond, 1000, 0},
	} {
		stage.clock.Advance(step.advance)
		stage.hub.Handle(t.Context(), host, Inbound{Type: MessagePing, TMono: 1})
		if group := box.last(MessagePong).Group; group.PositionMs != step.positionMs ||
			group.StartsInMs != step.startsInMs {
			t.Errorf("after %s more, the pong's group is %+v; want at %d ms, starting in %d ms",
				step.advance, group, step.positionMs, step.startsInMs)
		}
	}
}

// While a player along has not yet learned how late it starts, the group's start lead is the run-up,
// which that player plays through from before where the group starts; at the film's first moments
// the group starts the run-up in. Once every player has learned it, the lead is the start's alone,
// from where the group is (D282).
func TestHub_AGroupGivesARunUpWhileAPlayerHasNotLearnedHowLateItStarts(t *testing.T) {
	t.Parallel()
	for _, start := range []struct {
		name                   string
		startLagMs             *int64
		fromMs                 int64
		wantStartsInMs, wantMs int64
	}{
		{"unlearned, at the film's start", nil, 0, 2000, 2000},
		{"unlearned, mid-film", nil, 600_000, 2000, 600_000},
		{"learned, at the film's start", new(int64(170)), 0, 500, 0},
	} {
		stage := newStage(t)
		tuning := testTuning
		tuning.StartLeadMs, tuning.RunUpMs = 500, 2000
		box := &inbox{}
		host, err := stage.hub.Attach(stage.opened.MembershipCredential, RolePlayback, true, nil, tuning, box.send)
		if err != nil {
			t.Fatal(err)
		}
		stage.hub.Handle(t.Context(), host, Inbound{Type: MessageState, StartLagMs: start.startLagMs})
		stage.hub.Handle(t.Context(), host, Inbound{
			Type: MessageCommand, Command: CommandChangeItem, FileID: 7, PositionMs: start.fromMs,
		})
		group := box.last(MessageGroup).Group
		if group.State != GroupPlaying || group.StartsInMs != start.wantStartsInMs || group.PositionMs != start.wantMs {
			t.Errorf("%s: %+v; want Playing from %d ms, starting in %d ms",
				start.name, group, start.wantMs, start.wantStartsInMs)
		}
	}
}

// A player loading before the group starts moving, as one running up to the start loads from before
// it, is starting, not stalled: the group starts as it named, and only a stall after that is one
// (D282; the browser tier, 2026-10-12, where every run-up was taken for a stall).
func TestHub_APlayerLoadingBeforeTheGroupStartsIsNoStall(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	tuning := testTuning
	tuning.StartLeadMs, tuning.RunUpMs = 500, 2000
	box := &inbox{}
	host, err := stage.hub.Attach(stage.opened.MembershipCredential, RolePlayback, true, nil, tuning, box.send)
	if err != nil {
		t.Fatal(err)
	}
	stage.ready(t, host, false)
	stage.command(t, host, CommandChangeItem, 7)
	seq := box.last(MessageGroup).Seq
	stage.clock.Advance(time.Second)
	stage.ready(t, host, true) // its run-up loads
	stage.ready(t, host, false)
	if box.last(MessageGroup).Seq != seq {
		t.Errorf("a player loading its run-up changed the group: %+v", box.last(MessageGroup).Group)
	}
	stage.clock.Advance(2 * time.Second)
	stage.ready(t, host, true)
	if group := box.last(MessageGroup).Group; group.State != GroupWaiting || group.Event != EventWaitingFor {
		t.Errorf("the player stalling once the group moves: %+v; want Waiting for it", group)
	}
}
