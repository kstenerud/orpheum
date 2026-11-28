// SPDX-License-Identifier: AGPL-3.0-or-later

package agent

import (
	"context"
	"sync"

	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// runsGoing are the runs going on the node that make a picture, each a run of a quality or a video's
// trickplay, so a run of a quality can say whether another went beside it at any time during its
// speed window: only a window alone on its node is a fair measure of its speed
// (design/plans/start-quality-by-node-speed.md §3, D572). A run of sound, or of a stream copied, is
// none of them: every playback has its sound's run beside its picture's, which costs the node
// little, and counting it left no run fair. Its zero value is ready.
type runsGoing struct {
	mutex sync.Mutex
	going map[*runGoing]struct{}
}

// runGoing is one run, marked once another has gone beside it; one of runs nil is beside none. Its
// ctx is its job's: a run stopped is beside no run that starts after, though its FFmpeg may take a
// moment to end, so the run a cancel made way for, given just after it, starts alone.
type runGoing struct {
	ctx       context.Context //nolint:containedctx // read only to see whether the run was stopped
	runs      *runsGoing
	wasBeside bool
}

// starts says a run of the job of ctx has started; ends on what it returns says it has ended.
// Another goes beside it from when both are going, whichever started first, unless the one going
// before was stopped.
func (runs *runsGoing) starts(ctx context.Context) *runGoing {
	runs.mutex.Lock()
	defer runs.mutex.Unlock()
	if runs.going == nil {
		runs.going = map[*runGoing]struct{}{}
	}
	run := &runGoing{ctx: ctx, runs: runs}
	for other := range runs.going {
		if other.ctx.Err() == nil {
			other.wasBeside, run.wasBeside = true, true
		}
	}
	runs.going[run] = struct{}{}
	return run
}

// isBeside is whether another run has gone beside run since it started, by now.
func (run *runGoing) isBeside() bool {
	if run.runs == nil {
		return false
	}
	run.runs.mutex.Lock()
	defer run.runs.mutex.Unlock()
	return run.wasBeside
}

// ends says run has ended, so no later run is beside it. Ending it again does nothing.
func (run *runGoing) ends() {
	if run.runs == nil {
		return
	}
	run.runs.mutex.Lock()
	defer run.runs.mutex.Unlock()
	delete(run.runs.going, run)
}

// runStarts says a run of chunks of a stream of kind, of the job of ctx, has started, as
// runsGoing.starts does for one that makes a picture; a run of any other stream is beside none, and
// none is beside it.
func (agent *Agent) runStarts(ctx context.Context, kind nodeproto.ChunkStreamKind) *runGoing {
	if kind != nodeproto.StreamQuality {
		return &runGoing{}
	}
	return agent.runsGoing.starts(ctx)
}
