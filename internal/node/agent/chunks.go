// SPDX-License-Identifier: GPL-3.0-or-later

package agent

import (
	"context"
	"errors"

	"github.com/kstenerud/orpheum/internal/node/prepared"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// ChunkMaker makes a run of chunks of one stream of a film, telling onChunk each as it is in its
// store, and returns the last it made, or -1; chunks.Maker is the real one.
type ChunkMaker interface {
	Make(ctx context.Context, sourcePath string, command nodeproto.MakeChunks, onChunk func(n int)) (int, error)
}

// chunksWork is a run of chunks held until a slot of its kind is free, an encode's for a quality
// and a remux's for a copied picture or a sound, which tells the core each chunk it makes and how
// it ended (D247 §10).
func (agent *Agent) chunksWork(
	ctx context.Context, nodeID int64, held attempt, command nodeproto.MakeChunks,
) heldWork {
	jobID := held.jobID
	return heldWork{saysHowFar: true, run: func(jobCtx context.Context) {
		report := agent.runChunks(jobCtx, nodeID, held, command)
		if jobCtx.Err() != nil && ctx.Err() == nil {
			agent.logger.InfoContext(ctx, "a run of chunks was cancelled",
				"jobId", jobID, "lastChunk", report.LastChunk)
			return
		}
		if report.State != nodeproto.PrepareDone {
			agent.logger.WarnContext(ctx, "a run of chunks did not finish", "jobId", jobID,
				"storeDir", command.StoreDir, "state", report.State, "lastChunk", report.LastChunk,
				"why", report.Failure+report.Refusal+stallDetail(report.Stall))
		}
		agent.postResult(ctx, nodeID, "a run of chunks", held.named(nodeproto.Result{MakeChunks: &report}))
		agent.evict(ctx)
	}}
}

// runChunks makes the run's chunks from a source checked to be the one the core named (D100), and
// says how far it got and how it ended: done, FFmpeg failing on the source, stalled, or refused
// without trying.
func (agent *Agent) runChunks(
	ctx context.Context, nodeID int64, held attempt, command nodeproto.MakeChunks,
) nodeproto.MakeChunksReport {
	refused := func(err error) nodeproto.MakeChunksReport {
		return nodeproto.MakeChunksReport{
			State: nodeproto.PrepareRefused, LastChunk: -1, Refusal: err.Error(), RefusalCode: refusalCodeOf(err),
		}
	}
	if agent.workers.ChunkMaker == nil {
		return refused(errors.New("this node does not make chunks"))
	}
	sourcePath, err := agent.verifiedSourcePath(command.Video)
	if err != nil {
		return refused(err)
	}
	onChunk := func(n int) {
		running := held.progress(nodeproto.ProgressRunning)
		running.ChunksMadeThrough = &n
		agent.sayProgress(ctx, nodeID, running)
	}
	last, err := agent.workers.ChunkMaker.Make(ctx, sourcePath, command, onChunk)
	switch stall := stallOf(err); {
	case errors.Is(err, prepared.ErrNoRoom):
		noRoom := refused(err)
		noRoom.RefusalCode, noRoom.LastChunk = nodeproto.RefusedNoRoom, last
		return noRoom
	case stall != nil:
		return nodeproto.MakeChunksReport{State: nodeproto.PrepareStalled, LastChunk: last, Stall: stall}
	case err != nil:
		return nodeproto.MakeChunksReport{State: nodeproto.PrepareFailed, LastChunk: last, Failure: err.Error()}
	}
	return nodeproto.MakeChunksReport{State: nodeproto.PrepareDone, LastChunk: last}
}
