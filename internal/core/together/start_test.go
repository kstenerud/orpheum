// SPDX-License-Identifier: AGPL-3.0-or-later

package together

import (
	"context"
	"strings"
	"testing"
	"time"
)

// A group with no leader that starts playing starts moving the start's lead from then, and says what
// is left of it, so every player starts at once rather than each as the message reaches it; one that
// plays on moves on as it was (D268 §3). A group with a leader has no lead: it plays as its leader
// does (D371 §1).
func TestHub_AGroupStartsMovingTheStartsLeadAfterItsSyncPointEnds(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	tuning := testTuning
	tuning.StartLeadMs = 500
	box := &inbox{}
	host, err := stage.hub.Attach(stage.opened.MembershipCredential, RolePanel, true, nil, tuning, box.send)
	if err != nil {
		t.Fatal(err)
	}
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

// While a player of a group with no leader has not yet learned how late it starts, the group's start
// lead is the run-up,
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
		guestDevice, _ := stage.attach(t, stage.join(t).MembershipCredential, RoleVideo)
		box := &inbox{}
		host, err := stage.hub.Attach(stage.opened.MembershipCredential, RolePanel, true, nil, tuning, box.send)
		if err != nil {
			t.Fatal(err)
		}
		stage.hub.Handle(t.Context(), guestDevice, Inbound{Type: MessageState, StartLagMs: start.startLagMs})
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

// A jump of more than 30 s, or the setting's distance, is snapped back to the keyframe at or before
// it, the group starting the run-up after it; a shorter skip, one whose film's keyframes are not
// known, or the leader's own, lands where asked (D285 §1; the owner: "Using the scroll bar to jump is by nature
// imprecise").
func TestHub_ALongJumpIsSnappedBackToItsKeyframeAndAShortSkipIsNot(t *testing.T) {
	t.Parallel()
	for _, jump := range []struct {
		name    string
		toMs    int64
		isKnown bool
		wantMs  int64
		// snapMs is together_snap_seeks_beyond_seconds as the device brought it; zero brings none.
		snapMs int64
	}{
		{"a long jump", 1_461_502, true, 1_461_541, 0},
		{"a short skip", 70_000, true, 70_000, 0},
		{"a long jump, keyframes unknown", 1_461_502, false, 1_461_502, 0},
		// The distance is a setting (the constants sweep, 2026-10-20): at 5 s, a 10 s skip is snapped.
		{"a 10 s skip, snapped beyond 5 s", 70_000, true, 1_461_541, 5000},
	} {
		stage := newStage(t)
		var asked []int64
		stage.hub.SnapSeeksBy(func(_ context.Context, fileID, positionMs int64) (int64, bool) {
			asked = append(asked, fileID, positionMs)
			return 1_459_541, jump.isKnown // the chunk holding 24:21.502 starts at 24:19.541
		})
		tuning := testTuning
		tuning.RunUpMs, tuning.SnapSeeksBeyondMs = 2000, jump.snapMs
		leader, _ := stage.attach(t, stage.opened.MembershipCredential, RoleVideo)
		box := &inbox{}
		host, err := stage.hub.Attach(stage.opened.MembershipCredential, RolePanel, true, nil, tuning, box.send)
		if err != nil {
			t.Fatal(err)
		}
		stage.hub.Handle(t.Context(), leader, Inbound{Type: MessageState, StartLagMs: new(int64(100))})
		stage.hub.Handle(t.Context(), host, Inbound{
			Type: MessageCommand, Command: CommandChangeItem, FileID: 7, PositionMs: 60_000,
		})
		stage.hub.Handle(t.Context(), host, Inbound{
			Type: MessageCommand, Command: CommandSeek, PositionMs: jump.toMs, BasedOnSeq: box.last(MessageGroup).Seq,
		})
		group := box.last(MessageGroup).Group
		if group.Event != EventSeeked || group.PositionMs != jump.wantMs {
			t.Errorf("%s to %d ms: %+v; want the group at %d ms", jump.name, jump.toMs, group, jump.wantMs)
		}
		if len(asked) != 2 || asked[0] != 7 || asked[1] != jump.toMs {
			t.Errorf("%s: asked for the keyframe of %v; want film 7's at %d ms", jump.name, asked, jump.toMs)
		}
		stage.hub.Handle(t.Context(), leader, Inbound{
			Type: MessageCommand, Command: CommandSeek, PositionMs: jump.toMs, BasedOnSeq: box.last(MessageGroup).Seq,
		})
		if group := box.last(MessageGroup).Group; group.PositionMs != jump.toMs {
			t.Errorf("%s, the leader's own seek to %d ms: the group at %d ms; want where the leader went, "+
				"the leader never sought to the group's place (D371 §1)", jump.name, jump.toMs, group.PositionMs)
		}
	}
}
