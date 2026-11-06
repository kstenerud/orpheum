// SPDX-License-Identifier: AGPL-3.0-or-later

package agent

import (
	"context"

	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// SystemVideoMaker makes the server's own videos into the node's system folder: systemvideo.Maker.
type SystemVideoMaker interface {
	Make(ctx context.Context, command nodeproto.MakeSystemVideos) nodeproto.SystemVideosMade
	// Folder is the system folder, one of the node's roots, which the hello names.
	Folder() string
}

// systemVideosWork makes the system videos asked (D435 §4), held until a slot is free, and answers
// with the files made or kept, or why they could not be. A node with no system folder says so.
func (agent *Agent) systemVideosWork(
	ctx context.Context, nodeID int64, held attempt, command nodeproto.MakeSystemVideos,
) heldWork {
	return heldWork{run: func(jobCtx context.Context) {
		agent.sayProgress(ctx, nodeID, held.progress(nodeproto.ProgressRunning))
		made := nodeproto.SystemVideosMade{Failure: "this node has no system folder"}
		if agent.workers.SystemVideoMaker != nil {
			made = agent.workers.SystemVideoMaker.Make(jobCtx, command)
		}
		if jobCtx.Err() != nil && ctx.Err() == nil {
			agent.logger.InfoContext(ctx, "making the system videos was cancelled", "jobId", held.jobID)
			return
		}
		if made.Failure != "" {
			agent.logger.WarnContext(ctx, "could not make the system videos", "jobId", held.jobID,
				"failure", made.Failure)
		}
		agent.postResult(ctx, nodeID, "the system videos", held.named(nodeproto.Result{SystemVideos: &made}))
	}}
}
