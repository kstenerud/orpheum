// SPDX-License-Identifier: AGPL-3.0-or-later

package together

import (
	"context"
	"testing"
	"time"
)

// A group with no leader, its one player's page hidden (D372 §5), starts moving from where it stands
// at once, as the host's command plays it, with no lead and no run-up: whoever plays catches up to
// it (D371 §2, §6), as one with a leader plays as its leader does.
func TestHub_AGroupWithNoLeaderPlaysAtOnceFromWhereItStands(t *testing.T) {
	t.Parallel()
	for _, fromMs := range []int64{0, 600_000} {
		stage := newStage(t)
		guestDevice, _ := stage.attach(t, stage.join(t).MembershipCredential, RoleVideo)
		stage.hide(t, guestDevice, true)
		panel, box := stage.attach(t, stage.opened.MembershipCredential, RolePanel)
		stage.hub.Handle(t.Context(), panel, Inbound{
			Type: MessageCommand, Command: CommandChangeItem, FileID: 7, PositionMs: fromMs,
		})
		if group := box.last(MessageGroup).Group; group.State != GroupPlaying || group.PositionMs != fromMs {
			t.Errorf("film 7 put on at %d ms with no leader: %+v; want Playing from there", fromMs, group)
		}
		stage.clock.Advance(200 * time.Millisecond)
		if group := stage.pongGroup(t, panel, box); group.PositionMs != fromMs+200 {
			t.Errorf("200 ms after film 7 was put on at %d ms: the group at %d ms; want it moving at once",
				fromMs, group.PositionMs)
		}
	}
}

// A jump of more than 30 s, or the setting's distance, is snapped back to the keyframe at or before
// it, there being no run-up after it (D371); a shorter skip, one whose film's keyframes are not
// known, or the leader's own, lands where asked (D285 §1; the owner: "Using the scroll bar to jump is by nature
// imprecise").
func TestHub_ALongJumpIsSnappedBackToItsKeyframeAndAShortSkipIsNot(t *testing.T) {
	t.Parallel()
	for _, jump := range []struct {
		name    string
		toMs    int64
		isKnown bool
		wantMs  int64
		// snapMs is togetherSnapSeeksBeyondSeconds as the device brought it; zero brings none.
		snapMs int64
	}{
		{"a long jump", 1_461_502, true, 1_459_541, 0},
		{"a short skip", 70_000, true, 70_000, 0},
		{"a long jump, keyframes unknown", 1_461_502, false, 1_461_502, 0},
		// The distance is a setting (the constants sweep, 2026-10-20): at 5 s, a 10 s skip is snapped.
		{"a 10 s skip, snapped beyond 5 s", 70_000, true, 1_459_541, 5000},
	} {
		stage := newStage(t)
		var asked []int64
		stage.hub.SnapSeeksBy(func(_ context.Context, fileID, positionMs int64) (int64, bool) {
			asked = append(asked, fileID, positionMs)
			return 1_459_541, jump.isKnown // the chunk holding 24:21.502 starts at 24:19.541
		})
		tuning := testTuning
		tuning.SnapSeeksBeyondMs = jump.snapMs
		leader, _ := stage.attach(t, stage.opened.MembershipCredential, RoleVideo)
		box := &inbox{}
		host, err := stage.hub.Attach(stage.opened.MembershipCredential, RolePanel, true, nil, tuning, box.send)
		if err != nil {
			t.Fatal(err)
		}
		stage.hub.Handle(t.Context(), host, Inbound{
			Type: MessageCommand, Command: CommandChangeItem, FileID: 7, PositionMs: 60_000,
		})
		stage.ready(t, leader, true) // the host's player, loading the film, leads it
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
