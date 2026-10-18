// SPDX-License-Identifier: AGPL-3.0-or-later

package chunks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/kstenerud/orpheum/internal/core/jobs"
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
// kind, as its playback's is; one with no grid yet is ErrNoGrid. It is the server's own start of a
// player's run, as the player starts or stalls, whose interest is recorded once it is on its way;
// a player's ask is askedFor.
func (service *Service) Ensure(ctx context.Context, fileID int64, kind GridKind, stream Stream, wanted int) error {
	return service.ensure(ctx, ensuring{fileID: fileID, kind: kind, stream: stream, wanted: wanted})
}

// askedFor is Ensure for a player's own ask, which plans no run its interest would not want: a
// paused player filling its buffer started a run past what it keeps ahead at each ask, stopped
// within a second as it was (staging, 2026-10-18).
func (service *Service) askedFor(ctx context.Context, fileID int64, kind GridKind, stream Stream, wanted int) error {
	return service.ensure(ctx, ensuring{fileID: fileID, kind: kind, stream: stream, wanted: wanted, isAsked: true})
}

// ensuring is what Ensure makes sure of: a film's stream's chunk wanted, cut at kind, and whether a
// player asked for it.
type ensuring struct {
	fileID  int64
	kind    GridKind
	stream  Stream
	wanted  int
	isAsked bool
}

// ensure is Ensure, and askedFor when the ensuring says a player asked.
func (service *Service) ensure(ctx context.Context, asked ensuring) error {
	fileID, kind, stream, wanted := asked.fileID, asked.kind, asked.stream, asked.wanted
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
	wants, askErr := service.ask(ctx, film, stream, wanted, asked.isAsked)
	if askErr != nil {
		return askErr
	}
	first, last, isPlanned := service.runs.plan(dir, film.grid, wanted, wants)
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

// ask is a player's ask for chunk wanted of the store: an evicted store refuses it when it is neither
// made nor on its way (ErrNoRoomToMake, D250 §6), and plans no run (runTracker.plan): only a player
// its eviction left without its chunks is told why it stops. Refusing every ask, a chunk made and
// served was answered 500, and a player whose quality another run was making at 2.5x playback asked
// for it every two seconds for minutes (the owner's Wings on nas.example, 2026-10-17). For a player's
// ask, isAsked, it returns what its interest wants, which the run planned for it must be; every run
// for the server's own start, or where nothing says.
func (service *Service) ask(
	ctx context.Context, film cutFilm, stream Stream, wanted int, isAsked bool,
) (wantedness, error) {
	dir := StoreDir(film.file.ID, stream, film.grid.ID())
	service.runs.knowGrid(dir, film.grid)
	if service.runs.isEvicted(dir) && !service.runs.isCovered(dir, wanted) {
		return nil, fmt.Errorf("%s: %w", dir, ErrNoRoomToMake)
	}
	wants, err := service.wantednessOf(ctx, film.file.ID, stream, isAsked)
	if err != nil {
		return nil, err
	}
	queued, isCut := service.runs.cutQueuedAhead(dir, film.grid, wanted)
	if !isCut {
		return wants, nil
	}
	cancelled, err := service.cancelRun(ctx, queued)
	if err != nil {
		return nil, err
	}
	service.logger.InfoContext(ctx, "cancelled a queued run of chunks to start it where the player is",
		queued.attributes(cancelled)...)
	return wants, nil
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
