// SPDX-License-Identifier: GPL-3.0-or-later

package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"

	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// sayProgress tells the core where an attempt is, once: progress is best effort, and the newest
// supersedes it, so one the core did not take is not sent again (D234 §5).
func (agent *Agent) sayProgress(ctx context.Context, nodeID int64, progress nodeproto.Progress) {
	encoded, err := json.Marshal(progress)
	if err == nil {
		var accepted struct{}
		err = agent.exchange(ctx, http.MethodPost, nodeproto.PathProgress, nodeID, bytes.NewReader(encoded), &accepted)
	}
	if err != nil && ctx.Err() == nil {
		agent.logger.DebugContext(ctx, "could not tell the core where a job is", "jobId", progress.JobID,
			"state", progress.State, "err", err)
	}
}

// sayHeld tells the core, at a status tick, where each attempt the node holds is: waiting for a slot
// or a reader, running, or paused. A running prepare says how far it is as FFmpeg moves on instead, so
// the core's record of how far it is never goes back; a paused one says it is paused.
func (agent *Agent) sayHeld(ctx context.Context, nodeID int64) {
	for _, held := range agent.work.states() {
		if held.isRunning && !held.isPaused && held.saysHowFar {
			continue
		}
		state := nodeproto.ProgressWaiting
		switch {
		case held.isPaused:
			state = nodeproto.ProgressPaused
		case held.isRunning:
			state = nodeproto.ProgressRunning
		}
		agent.sayProgress(ctx, nodeID, held.progress(state))
	}
}

// progress is the core's word on where the attempt is.
func (held attempt) progress(state nodeproto.ProgressState) nodeproto.Progress {
	return nodeproto.Progress{JobID: held.jobID, AttemptToken: held.token, State: state}
}
