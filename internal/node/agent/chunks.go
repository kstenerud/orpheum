// SPDX-License-Identifier: AGPL-3.0-or-later

package agent

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/node/prepared"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// ChunkMaker makes a run of chunks of one stream of a film, telling onChunk each as it is in its
// store, and returns the last it made, or -1; chunks.Maker is the real one.
type ChunkMaker interface {
	Make(ctx context.Context, source execx.Film, command nodeproto.MakeChunks, onChunk func(n int)) (int, error)
}

// chunksWork is a run of chunks held until a slot of its kind is free, an encode's for a quality
// and a remux's for a copied picture or a sound, which tells the core each chunk it makes and how
// it ended (D247 §10). However it ends, it is logged with how much film it made and how fast
// (chunksRunFigures).
func (agent *Agent) chunksWork(
	ctx context.Context, nodeID int64, held attempt, command nodeproto.MakeChunks,
) heldWork {
	jobID := held.jobID
	return heldWork{saysHowFar: true, run: func(jobCtx context.Context) {
		times := chunksRunTimes{startedAt: agent.clock.Now()}
		report := agent.runChunks(jobCtx, nodeID, held, command, &times)
		run := append([]any{
			"jobId", jobID, "storeDir", command.StoreDir, "firstChunk", command.FirstChunk,
			"lastChunk", report.LastChunk,
		}, agent.chunksRunFigures(command, report, times)...)
		switch {
		case jobCtx.Err() != nil && ctx.Err() == nil:
			agent.logger.InfoContext(ctx, "a run of chunks was cancelled", run...)
			return
		case report.State == nodeproto.RunDone:
			agent.logger.InfoContext(ctx, "a run of chunks finished", run...)
		default:
			agent.logger.WarnContext(ctx, "a run of chunks did not finish", append(run,
				"state", report.State, "why", report.Failure+report.Refusal+stallDetail(report.Stall))...)
		}
		agent.postResult(ctx, nodeID, "a run of chunks", held.named(nodeproto.Result{MakeChunks: &report}))
		agent.evict(ctx)
	}}
}

// chunksRunTimes are when a run of chunks started, and when it made its first chunk, zero until it
// does.
type chunksRunTimes struct {
	startedAt, firstChunkAt time.Time
}

// chunksRunFigures are a run's numbers for its log line: the chunks it made, the film they span and
// the wall time it took, both in ms, its speed as film made per wall time (6.2 is 6.2 times real
// time), and how long it took to make its first chunk. Whether a run keeps ahead of a player who
// seeks into it was found only by reading the code (playback-in-practice.md §3, 2026-10-06).
func (agent *Agent) chunksRunFigures(
	command nodeproto.MakeChunks, report nodeproto.MakeChunksReport, times chunksRunTimes,
) []any {
	chunksMade := max(report.LastChunk-command.FirstChunk+1, 0)
	var filmMadeMs int64
	if chunksMade > 0 && chunksMade < len(command.BoundsMs) {
		filmMadeMs = command.BoundsMs[chunksMade] - command.BoundsMs[0]
	}
	wallTimeMs := agent.clock.Now().Sub(times.startedAt).Milliseconds()
	figures := []any{"chunksMade", chunksMade, "filmMadeMs", filmMadeMs, "wallTimeMs", wallTimeMs}
	if filmMadeMs > 0 && wallTimeMs > 0 {
		figures = append(figures, "speed", math.Round(float64(filmMadeMs)/float64(wallTimeMs)*10)/10)
	}
	if !times.firstChunkAt.IsZero() {
		figures = append(figures, "firstChunkAfterMs", times.firstChunkAt.Sub(times.startedAt).Milliseconds())
	}
	return figures
}

// runChunks makes the run's chunks from a source checked to be the one the core named (D100), and
// says how far it got and how it ended: done, FFmpeg failing on the source, stalled, or refused
// without trying. It notes in times when it made its first chunk.
func (agent *Agent) runChunks(
	ctx context.Context, nodeID int64, held attempt, command nodeproto.MakeChunks, times *chunksRunTimes,
) nodeproto.MakeChunksReport {
	refused := func(err error) nodeproto.MakeChunksReport {
		return nodeproto.MakeChunksReport{
			State: nodeproto.RunRefused, LastChunk: -1, Refusal: err.Error(), RefusalCode: refusalCodeOf(err),
		}
	}
	if agent.workers.ChunkMaker == nil {
		return refused(errors.New("this node does not make chunks"))
	}
	source, closeSource, err := agent.openFilm(ctx, command.Video)
	if stall := stallOf(err); stall != nil {
		return nodeproto.MakeChunksReport{State: nodeproto.RunStalled, LastChunk: -1, Stall: stall}
	}
	if err != nil {
		return refused(err)
	}
	defer closeSource()
	onChunk := func(n int) {
		if times.firstChunkAt.IsZero() {
			times.firstChunkAt = agent.clock.Now()
		}
		running := held.progress(nodeproto.ProgressRunning)
		running.ChunksMadeThrough = &n
		agent.sayProgress(ctx, nodeID, running)
	}
	last, err := agent.workers.ChunkMaker.Make(ctx, source, command, onChunk)
	switch stall := stallOf(err); {
	case errors.Is(err, prepared.ErrNoRoom):
		noRoom := refused(err)
		noRoom.RefusalCode, noRoom.LastChunk = nodeproto.RefusedNoRoom, last
		return noRoom
	case stall != nil:
		return nodeproto.MakeChunksReport{State: nodeproto.RunStalled, LastChunk: last, Stall: stall}
	case err != nil:
		return nodeproto.MakeChunksReport{State: nodeproto.RunFailed, LastChunk: last, Failure: err.Error()}
	}
	return nodeproto.MakeChunksReport{State: nodeproto.RunDone, LastChunk: last}
}
