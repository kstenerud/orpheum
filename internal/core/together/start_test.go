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
