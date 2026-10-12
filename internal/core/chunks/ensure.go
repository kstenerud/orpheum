// SPDX-License-Identifier: AGPL-3.0-or-later

package chunks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/store/cachegen"
)

// lookaheadMs is how far past a chunk a player asks for the chunks are made or on their way before
// it waits for none: a minute of film, Kyoo's look-ahead (slow-links.md §4), about what hls.js
// buffers ahead, so a player never asks for a chunk no run is making (D247 §10).
const lookaheadMs = 60_000

// runSpanMs is the most film one run makes: five minutes, so a seek starts encoding where the
// player is rather than finishing a long run elsewhere, and a run's FFmpeg is started about once
// every five minutes of film rather than for each chunk (D247 §10; Kyoo's runs, slow-links.md §4).
const runSpanMs = 300_000

// ErrNoGrid is a film with no grid yet: its picture's timeline is being read (ReadForPlay).
var ErrNoGrid = errors.New("the film is not cut into chunks yet")

// runArgs are a make_chunks job's arguments: one run, chunks FirstChunk through LastChunk of the
// store the job targets, whose chunk_store row names the film, the stream and the grid.
type runArgs struct {
	FirstChunk int `json:"firstChunk"`
	LastChunk  int `json:"lastChunk"`
}

// Ensure makes sure a player asking for chunk wanted of a film's stream will have it and the chunks
// after it: of those that start within lookaheadMs of film from it, the first neither made nor on
// its way starts a run, at play priority on the film's node, through the earliest of runSpanMs of
// film, the chunk before the next made or on its way, and the film's last chunk. A run is never
// queued over chunks a run will make. Each store is the target of its own runs' jobs, so the runs of
// a film's streams queue side by side, and a run asked for while another of its store waits queued
// is folded into that one, as the queue folds any repeat request: the next ask plans it again. A
// store with runs on their way is kept wanted (D234 §3). A run queued is logged. The film is cut at
// kind, as its playback's is; one with no grid yet is ErrNoGrid.
func (service *Service) Ensure(ctx context.Context, fileID int64, kind GridKind, stream Stream, wanted int) error {
	film, isCut, err := service.cut(ctx, fileID, kind)
	if err != nil {
		return err
	}
	if !isCut {
		return fmt.Errorf("file %d: %w", fileID, ErrNoGrid)
	}
	if wanted < 0 || wanted > film.grid.lastChunk() {
		return fmt.Errorf("file %d has no chunk %d: its last is %d", fileID, wanted, film.grid.lastChunk())
	}
	gridID := film.grid.ID()
	dir := StoreDir(fileID, stream, gridID)
	if askErr := service.ask(ctx, film, stream, wanted); askErr != nil {
		return askErr
	}
	first, last, isPlanned := service.runs.plan(dir, film.grid, wanted)
	if !isPlanned {
		if storeID, isMaking := service.runs.making(dir); isMaking && storeID != 0 {
			return service.queue.KeepWanted(ctx, jobs.KindMakeChunks, jobs.Target{ChunkStoreID: storeID})
		}
		return nil
	}
	storeID, err := service.storeIDOf(ctx, fileID, stream, gridID)
	if err != nil {
		service.runs.forget(dir, first)
		return err
	}
	service.runs.knowStoreID(dir, storeID)
	args, err := json.Marshal(runArgs{FirstChunk: first, LastChunk: last})
	if err != nil {
		service.runs.forget(dir, first)
		return fmt.Errorf("encode a run's arguments: %w", err)
	}
	isQueued, err := service.queue.Enqueue(ctx, jobs.Request{
		Kind: jobs.KindMakeChunks, Target: jobs.Target{ChunkStoreID: storeID}, ArgsJSON: string(args),
		Priority: jobs.PriorityPlayNow, Reason: "a player is waiting for its chunks",
		Destination: jobs.NodeDestination(film.file.NodeID),
	})
	if err != nil || !isQueued {
		service.runs.forget(dir, first)
	}
	if err != nil {
		return fmt.Errorf("queue chunks %d to %d of %s: %w", first, last, dir, err)
	}
	if isQueued {
		// Which chunk started a run, and the film it spans, say whether a seek far ahead started one
		// or waited on another (playback-in-practice.md §3).
		service.logger.InfoContext(ctx, "queued a run of chunks", "storeDir", dir, "wantedChunk", wanted,
			"firstChunk", first, "lastChunk", last, "firstChunkStartMs", film.grid.StartsMs[first],
			"lastChunkEndMs", film.grid.endMs(last))
	}
	return nil
}

// ask records a player's ask for chunk wanted of the store: the runs making it and the chunks within
// lookaheadMs of film after it, which it plays on to, are in use. An evicted store refuses it
// (ErrNoRoomToMake, D250 §6); runs it is a seek far past are cancelled (cancelFarRuns).
func (service *Service) ask(ctx context.Context, film cutFilm, stream Stream, wanted int) error {
	dir := StoreDir(film.file.ID, stream, film.grid.ID())
	playsThrough := film.grid.chunkAt(film.grid.StartsMs[wanted] + lookaheadMs - 1)
	service.runs.asked(dir, wanted, playsThrough, service.clock.Now())
	if service.runs.isEvicted(dir) {
		return fmt.Errorf("%s: %w", dir, ErrNoRoomToMake)
	}
	return service.cancelFarRuns(ctx, film, stream, wanted)
}

// cancelFarRuns cancels the runs of the store chunk wanted is a seek far from (runTracker.cutFar,
// D250 §5): no one follows them now, and their encodes are room another run needs (§6). The new
// run is queued after, so the queue, which keeps one job of a store queued, does not fold it into
// one cancelled.
func (service *Service) cancelFarRuns(ctx context.Context, film cutFilm, stream Stream, wanted int) error {
	beyond, err := service.settings.Duration(ctx, settings.ChunkNewRunBeyondSeconds)
	if err != nil {
		return fmt.Errorf("read %s: %w", settings.ChunkNewRunBeyondSeconds, err)
	}
	fileID, gridID := film.file.ID, film.grid.ID()
	dir := StoreDir(fileID, stream, gridID)
	cuts := service.runs.cutFar(dir, film.grid, wanted, beyond.Milliseconds())
	if len(cuts) == 0 {
		return nil
	}
	storeID, err := service.storeIDOf(ctx, fileID, stream, gridID)
	if err != nil {
		return err
	}
	for _, cut := range cuts {
		cancelled, err := service.queue.CancelMatching(ctx, jobs.KindMakeChunks, jobs.Target{ChunkStoreID: storeID},
			func(argsJSON string) bool {
				var args runArgs
				return json.Unmarshal([]byte(argsJSON), &args) == nil && args.FirstChunk == cut.First
			})
		if err != nil {
			return fmt.Errorf("cancel the run of %s from chunk %d: %w", dir, cut.First, err)
		}
		service.logger.InfoContext(ctx, "cancelled a run of chunks a seek went far past", "storeDir", dir,
			"wantedChunk", wanted, "firstChunk", cut.First, "lastChunk", cut.Last, "madeThrough", cut.MadeThrough,
			"cancelledJobs", cancelled)
	}
	return nil
}

// storeIDOf is the chunk_store row of a stream of a film on a grid, made the first time it is named.
func (service *Service) storeIDOf(ctx context.Context, fileID int64, stream Stream, gridID string) (int64, error) {
	var storeID int64
	err := service.cache.Write(ctx, func(queries *cachegen.Queries) error {
		var writeErr error
		storeID, writeErr = queries.EnsureChunkStore(ctx, cachegen.EnsureChunkStoreParams{
			SourceFile: fileID, Stream: stream.Name(), GridID: gridID,
		})
		return writeErr
	})
	if err != nil {
		return 0, fmt.Errorf("record the store of %s: %w", StoreDir(fileID, stream, gridID), err)
	}
	return storeID, nil
}
