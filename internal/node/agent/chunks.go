// SPDX-License-Identifier: AGPL-3.0-or-later

package agent

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/kstenerud/orpheum/internal/node/encode"
	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/node/prepared"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/slogx"
)

// ChunkMaker makes a run of chunks of one stream of a film, telling onChunk each as it is in its
// store, and returns the last it made, or -1, and where a quality's picture was decoded (""
// for a run that decodes none); chunks.Maker is the real one.
type ChunkMaker interface {
	Make(
		ctx context.Context, source execx.Film, command nodeproto.MakeChunks, onChunk func(n int),
	) (int, encode.Decoding, error)
	// StretchOf is the stretch of its film the run reads, to which the film's long index is cut down
	// (execx.OpenedFilm.Stretch, D512).
	StretchOf(command nodeproto.MakeChunks) execx.Stretch
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
		if agent.workers.SpeedTrials != nil {
			ends := agent.workers.SpeedTrials.ChunksRunStarts()
			defer ends()
		}
		ended := agent.runStarts(command.Stream.Kind)
		defer ended() // a run that panics is beside no later one
		times := chunksRunTimes{startedAt: agent.clock.Now()}
		report := agent.runChunks(jobCtx, nodeID, held, command, &times)
		made := agent.madeIn(command, report, times)
		withSpeedFigures(&report, command, made, times, ended())
		run := append([]any{
			"jobId", jobID, "storeDir", command.StoreDir, "firstChunk", command.FirstChunk,
			"lastChunk", report.LastChunk,
		}, chunksRunFigures(made, times)...)
		if report.WasBesideAnotherRun {
			// Why the core keeps no figure of a run that looks fair (start-quality-by-node-speed.md §3).
			run = append(run, "wentBesideAnotherRun", true)
		}
		switch {
		case jobCtx.Err() != nil && ctx.Err() == nil:
			agent.logger.InfoContext(ctx, "a run of chunks was cancelled",
				append(run, slogx.EventKey, "chunkRunCancelled")...)
			return
		case report.State == nodeproto.RunDone:
			agent.logger.InfoContext(ctx, "a run of chunks finished",
				append(run, slogx.EventKey, "chunkRunFinished")...)
		default:
			agent.logger.WarnContext(ctx, "a run of chunks did not finish",
				append(run, slogx.EventKey, "chunkRunFailed",
					"state", report.State, "why", report.Failure+report.Refusal+stallDetail(report.Stall))...)
		}
		agent.postResult(ctx, nodeID, "a run of chunks", held.named(nodeproto.Result{MakeChunks: &report}))
		agent.evict(ctx)
	}}
}

// chunksRunTimes are when a run of chunks started, when its film was opened and when it made its
// first chunk, each zero until it is; and whether its film's index was cut down, which is what makes
// a long-indexed MP4's first chunk quick (D512).
type chunksRunTimes struct {
	startedAt, openedAt, firstChunkAt time.Time
	isIndexCutDown                    bool
	// decodedOn is where a quality's picture was decoded, once its run has ended.
	decodedOn encode.Decoding
}

// runMade is what a run of chunks made: its chunks, the film they span and the wall time it took,
// both in ms.
type runMade struct {
	chunks             int
	filmMs, wallTimeMs int64
}

// madeIn is what the run of command made, as its report says, by now.
func (agent *Agent) madeIn(
	command nodeproto.MakeChunks, report nodeproto.MakeChunksReport, times chunksRunTimes,
) runMade {
	made := runMade{
		chunks:     max(report.LastChunk-command.FirstChunk+1, 0),
		wallTimeMs: agent.clock.Now().Sub(times.startedAt).Milliseconds(),
	}
	if made.chunks > 0 && made.chunks < len(command.BoundsMs) {
		made.filmMs = command.BoundsMs[made.chunks] - command.BoundsMs[0]
	}
	return made
}

// withSpeedFigures puts a run of a quality's figures in its report, for the core to judge its node's
// speed by (start-quality-by-node-speed.md §3): its speed, the film it made, where its picture was
// decoded and whether another run went beside it. A run of another stream, or one that made no film,
// carries none.
func withSpeedFigures(
	report *nodeproto.MakeChunksReport, command nodeproto.MakeChunks, made runMade, times chunksRunTimes,
	wasBeside bool,
) {
	if command.Stream.Kind != nodeproto.StreamQuality || made.filmMs <= 0 || made.wallTimeMs <= 0 {
		return
	}
	report.SpeedPercent = int(made.filmMs * realTimePercent / made.wallTimeMs)
	report.FilmMadeMs, report.DecodedOn, report.WasBesideAnotherRun = made.filmMs, string(times.decodedOn), wasBeside
}

// realTimePercent is real time in percent: a run's speed is its film over its wall time, by it.
const realTimePercent = 100

// chunksRunFigures are a run's numbers for its log line: the chunks it made, the film they span and
// the wall time it took, both in ms, its speed as film made per wall time (6.2 is 6.2 times real
// time), how long it took to open its film and to make its first chunk, the time between them
// FFmpeg's to its first output, whether its film's index was cut down (D512), and, for a quality,
// where its picture was decoded (decodedOn: gpu or cpu, chunks.Maker.Make). Whether a run
// keeps ahead of a player who seeks into it was found only by reading the code
// (playback-in-practice.md §3, 2026-10-06).
func chunksRunFigures(made runMade, times chunksRunTimes) []any {
	figures := []any{"chunksMade", made.chunks, "filmMadeMs", made.filmMs, "wallTimeMs", made.wallTimeMs}
	if made.filmMs > 0 && made.wallTimeMs > 0 {
		figures = append(figures, "speed", math.Round(float64(made.filmMs)/float64(made.wallTimeMs)*10)/10)
	}
	if !times.openedAt.IsZero() {
		figures = append(figures, "filmOpenedAfterMs", times.openedAt.Sub(times.startedAt).Milliseconds())
	}
	if !times.firstChunkAt.IsZero() {
		figures = append(figures, "firstChunkAfterMs", times.firstChunkAt.Sub(times.startedAt).Milliseconds())
	}
	if times.isIndexCutDown {
		figures = append(figures, "indexCutDown", true)
	}
	if times.decodedOn != "" {
		figures = append(figures, "decodedOn", string(times.decodedOn))
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
	stretch := agent.workers.ChunkMaker.StretchOf(command)
	source, closeSource, err := agent.openFilmFor(ctx, command.Video, &stretch)
	if stall := stallOf(err); stall != nil {
		return nodeproto.MakeChunksReport{State: nodeproto.RunStalled, LastChunk: -1, Stall: stall}
	}
	if err != nil {
		return refused(err)
	}
	defer closeSource()
	times.openedAt, times.isIndexCutDown = agent.clock.Now(), source.IsIndexCutDown
	onChunk := func(n int) {
		if times.firstChunkAt.IsZero() {
			times.firstChunkAt = agent.clock.Now()
		}
		running := held.progress(nodeproto.ProgressRunning)
		running.ChunksMadeThrough = &n
		agent.sayProgress(ctx, nodeID, running)
	}
	last, decodedOn, err := agent.workers.ChunkMaker.Make(ctx, source, command, onChunk)
	times.decodedOn = decodedOn
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
