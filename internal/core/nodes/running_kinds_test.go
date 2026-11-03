// SPDX-License-Identifier: AGPL-3.0-or-later

package nodes_test

import (
	"testing"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

func TestRunningKinds_CountTheAttemptsEachNodeSaysRun(t *testing.T) {
	t.Parallel()
	bench := newExchanges(t)
	bench.exchanges.Handle(jobs.KindMakeChunks, assignedRun(make(answers, 2), nil))
	sent := bench.assign(t)
	say := func(state nodeproto.ProgressState) {
		bench.exchanges.DeliverProgress(t.Context(), 1, nodeproto.Progress{
			JobID: 7, AttemptToken: sent.AttemptToken, State: state,
		})
	}
	say(nodeproto.ProgressWaiting)
	if running := bench.exchanges.RunningKinds(); len(running) != 0 {
		t.Errorf("work waiting for a slot counts as running: %v", running)
	}
	say(nodeproto.ProgressRunning)
	if running := bench.exchanges.RunningKinds(); running[1][jobs.KindMakeChunks] != 1 || len(running) != 1 {
		t.Errorf("running = %v; want node 1 running one make_chunks", running)
	}
	say(nodeproto.ProgressPaused)
	if running := bench.exchanges.RunningKinds(); len(running) != 0 {
		t.Errorf("paused work counts as running: %v", running)
	}
}
