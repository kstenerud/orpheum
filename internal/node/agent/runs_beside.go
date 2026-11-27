// SPDX-License-Identifier: AGPL-3.0-or-later

package agent

import (
	"sync"

	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// runsGoing are the runs going on the node that make a picture, each a run of a quality or a video's
// trickplay, so a run of a quality can say whether another went beside it at any time while it did:
// only one alone on its node is a fair measure of its speed (design/plans/start-quality-by-node-speed.md
// §3). A run of sound, or of a stream copied, is none of them: every playback has its sound's run
// beside its picture's, which costs the node little, and counting it left no run fair. Its zero
// value is ready.
type runsGoing struct {
	mutex sync.Mutex
	going map[*runGoing]struct{}
}

// runGoing is one run, marked once another has gone beside it.
type runGoing struct{ wasBeside bool }

// starts says a run has started; the func it returns says it has ended, and whether another went
// beside it meanwhile, one that started before it and was still going or one that started after.
func (runs *runsGoing) starts() (ends func() (wasBeside bool)) {
	runs.mutex.Lock()
	defer runs.mutex.Unlock()
	if runs.going == nil {
		runs.going = map[*runGoing]struct{}{}
	}
	run := &runGoing{}
	for other := range runs.going {
		other.wasBeside, run.wasBeside = true, true
	}
	runs.going[run] = struct{}{}
	return func() bool {
		runs.mutex.Lock()
		defer runs.mutex.Unlock()
		delete(runs.going, run)
		return run.wasBeside
	}
}

// runStarts says a run of chunks of a stream of kind has started, as runsGoing.starts does for one
// that makes a picture; a run of any other stream is beside none, and none is beside it.
func (agent *Agent) runStarts(kind nodeproto.ChunkStreamKind) (ends func() (wasBeside bool)) {
	if kind != nodeproto.StreamQuality {
		return func() bool { return false }
	}
	return agent.runsGoing.starts()
}
