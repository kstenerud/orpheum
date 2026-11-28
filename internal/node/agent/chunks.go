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
	// DecodingOf is where a run of command decodes its picture, as it is decided at its start: what
	// its speed window says it was decoded on (D573).
	DecodingOf(command nodeproto.MakeChunks) encode.Decoding
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
		going := agent.runStarts(jobCtx, command.Stream.Kind)
		defer going.ends() // a run that panics is beside no later one
		times := chunksRunTimes{startedAt: agent.clock.Now(), going: going}
		report := agent.runChunks(jobCtx, nodeID, held, command, &times)
		going.ends()
		made := agent.madeIn(command, report, times)
		withSpeedFigures(&report, command, times)
		run := append([]any{
			"jobId", jobID, "storeDir", command.StoreDir, "firstChunk", command.FirstChunk,
			"lastChunk", report.LastChunk,
		}, chunksRunFigures(made, times)...)
		run = append(run, speedWindowFigures(command, times)...)
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
	// going is the run among the node's runs that make a picture, which says whether another went
	// beside it; window is its speed window, once the chunk that closed it was made.
	going  *runGoing
	window speedWindow
}

// speedWindow is a run's speed window, its first nodeproto.SpeedWindowFilmMs of film (D572): the film
// made to the chunk that closed it and the wall time from the run's start to it, both in ms, whether
// another run that makes a picture went beside it by then, and where its picture was decoded. All
// zero until it is closed.
type speedWindow struct {
	filmMs, wallTimeMs int64
	wasBeside          bool
	decodedOn          encode.Decoding
}

// isClosed is whether the run made its window's film.
func (window speedWindow) isClosed() bool { return window.filmMs > 0 }

// measuredWindow is the speed window of a run of command, and whether it measured one: a run of a
// quality that made its window's film.
func (times *chunksRunTimes) measuredWindow(command nodeproto.MakeChunks) (speedWindow, bool) {
	window := times.window
	return window, command.Stream.Kind == nodeproto.StreamQuality && window.isClosed() && window.wallTimeMs > 0
}

// noteChunk closes the run's speed window at the chunk n of command, made now, decoding as decoding,
// if it is the first to reach the window's film, and reports whether it did.
func (times *chunksRunTimes) noteChunk(
	command nodeproto.MakeChunks, n int, now time.Time, decoding encode.Decoding,
) bool {
	if times.window.isClosed() {
		return false
	}
	film := filmThrough(command, n)
	if film < nodeproto.SpeedWindowFilmMs {
		return false
	}
	times.window = speedWindow{
		filmMs: film, wallTimeMs: now.Sub(times.startedAt).Milliseconds(), wasBeside: times.going.isBeside(),
		decodedOn: decoding,
	}
	return true
}

// filmThrough is the film, in ms, a run of command spans from its first chunk through chunk last; 0
// when it made none.
func filmThrough(command nodeproto.MakeChunks, last int) int64 {
	made := last - command.FirstChunk + 1
	if made <= 0 || made >= len(command.BoundsMs) {
		return 0
	}
	return command.BoundsMs[made] - command.BoundsMs[0]
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
	return runMade{
		chunks:     max(report.LastChunk-command.FirstChunk+1, 0),
		filmMs:     filmThrough(command, report.LastChunk),
		wallTimeMs: agent.clock.Now().Sub(times.startedAt).Milliseconds(),
	}
}

// withSpeedFigures puts a run of a quality's figures in its report, for the core to judge its node's
// speed by (start-quality-by-node-speed.md §3, D572): speedFiguresOf.
func withSpeedFigures(report *nodeproto.MakeChunksReport, command nodeproto.MakeChunks, times chunksRunTimes) {
	if figures, isMeasured := speedFiguresOf(command, times); isMeasured {
		report.SpeedWindow = figures
	}
}

// speedFiguresOf is a run of a quality's figures, and whether it has them: its speed window's speed
// and film, where its picture was decoded and whether another run went beside it during the window.
// A run of another stream, or one that never made its window's film, has none. Its report carries
// them, and so does the progress of the chunk that closed its window, for a run cancelled after
// (D573).
func speedFiguresOf(command nodeproto.MakeChunks, times chunksRunTimes) (nodeproto.SpeedWindow, bool) {
	window, isMeasured := times.measuredWindow(command)
	if !isMeasured {
		return nodeproto.SpeedWindow{}, false
	}
	return nodeproto.SpeedWindow{
		SpeedPercent: int(window.filmMs * realTimePercent / window.wallTimeMs), FilmMadeMs: window.filmMs,
		DecodedOn: string(window.decodedOn), WasBesideAnotherRun: window.wasBeside,
	}, true
}

// speedWindowFigures are a run of a quality's speed window's numbers for its log line, beside its
// whole run's (chunksRunFigures): the film and wall time to the chunk that closed it, its speed, and
// whether another run went beside it then, which is why the core keeps no figure of a run that looks
// fair (D572). None for a run of another stream, or one that never made its window's film.
func speedWindowFigures(command nodeproto.MakeChunks, times chunksRunTimes) []any {
	window, isMeasured := times.measuredWindow(command)
	if !isMeasured {
		return nil
	}
	figures := []any{
		"speedWindowFilmMs", window.filmMs, "speedWindowWallTimeMs", window.wallTimeMs,
		"speedWindowSpeed", math.Round(float64(window.filmMs)/float64(window.wallTimeMs)*10) / 10,
	}
	if window.wasBeside {
		figures = append(figures, "speedWindowWentBesideAnotherRun", true)
	}
	return figures
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
	decoding := agent.workers.ChunkMaker.DecodingOf(command)
	onChunk := func(chunk int) {
		now := agent.clock.Now()
		if times.firstChunkAt.IsZero() {
			times.firstChunkAt = now
		}
		running := held.progress(nodeproto.ProgressRunning)
		running.ChunksMadeThrough = &chunk
		if times.noteChunk(command, chunk, now, decoding) {
			if figures, isMeasured := speedFiguresOf(command, *times); isMeasured {
				running.SpeedWindow = &figures
			}
		}
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
