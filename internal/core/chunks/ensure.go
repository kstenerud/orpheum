// SPDX-License-Identifier: AGPL-3.0-or-later

package chunks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/platform/slogx"
	"github.com/kstenerud/orpheum/internal/store/cachegen"
)

// ErrNoGrid is a film with no grid yet: its picture's timeline is being read (ReadForPlay).
var ErrNoGrid = errors.New("the film is not cut into chunks yet")

// runArgs are a make_chunks job's arguments: one run, chunks FirstChunk through LastChunk of the
// store the job targets, whose chunk_store row names the film, the stream and the grid.
type runArgs struct {
	FirstChunk int `json:"firstChunk"`
	LastChunk  int `json:"lastChunk"`
}

// Ensure makes sure a player asking for chunk wanted of a film's stream will have it and the chunks
// after it: of those that start within chunk_lookahead_seconds of film from it, the first neither made
// nor on its way starts a run, at play priority on the film's node, through the earliest of
// chunk_run_span_seconds of film, the chunk before the next made or on its way, and the film's last
// chunk (D247 §10: a minute ahead, Kyoo's look-ahead, about what hls.js buffers; five minutes a run,
// so a seek starts encoding where the player is and FFmpeg starts about every five minutes). A run is never
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
	service.refreshChunkTiming(ctx)
	storeKey := film.storeKey(stream)
	dir := StoreDir(fileID, stream, storeKey)
	wants, askErr := service.ask(ctx, film, stream, wanted, asked.isAsked)
	if askErr != nil {
		return askErr
	}
	first, last, isPlanned := service.runs.plan(dir, film.grid, wanted, wants)
	if !isPlanned {
		return service.plannedNone(ctx, asked, dir)
	}
	if refused := service.refuseGivenUp(dir, first); refused != nil {
		return refused
	}
	storeID, err := service.storeIDOf(ctx, fileID, stream, storeKey)
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
		service.logger.InfoContext(ctx, "queued a run of chunks",
			slogx.EventKey, "chunkRunQueued",
			"storeDir", dir, "wantedChunk", wanted,
			"firstChunk", first, "lastChunk", last, "firstChunkStartMs", film.grid.StartsMs[first],
			"lastChunkEndMs", film.grid.endMs(last))
	}
	return nil
}

// plannedNone ends an ensure that planned no run of the store at dir: one of its runs on its way is
// kept wanted (D234 §3), and a player's ask for a chunk neither made nor on its way, which no player
// wants a run for, is logged (logUnwantedAsk).
func (service *Service) plannedNone(ctx context.Context, asked ensuring, dir string) error {
	if asked.isAsked && !service.runs.isEvicted(dir) && !service.runs.isCovered(dir, asked.wanted) {
		service.logUnwantedAsk(ctx, dir, asked.stream, asked.wanted)
	}
	if storeID, isMaking := service.runs.making(dir); isMaking && storeID != 0 {
		return service.queue.KeepWanted(ctx, jobs.KindMakeChunks, jobs.Target{ChunkStoreID: storeID})
	}
	return nil
}

// logUnwantedAsk says, at debug, a player's ask for a chunk not made nor on its way that planned no
// run, as no player wants one from there, and what the players of its stream and of its film want:
// an init asked for from where its playback last reported, behind where its player jumped, is
// answered "ask again" until the jump's chunk starts the run (the owner's film, 2026-10-26).
func (service *Service) logUnwantedAsk(ctx context.Context, dir string, stream Stream, wanted int) {
	interests, _ := service.interestsOf(fileOfStore(dir))
	keptAhead, err := service.settings.Duration(ctx, settings.ChunkKeptAheadSeconds)
	if err != nil {
		keptAhead = 0 // the log says the positions all the same
	}
	service.logger.DebugContext(ctx, "planned no run of chunks: no player wants one from there",
		slogx.EventKey, "noRunPlanned",
		"storeDir", dir,
		"wantedChunk", wanted, "streamInterests", describeInterests(stream.Name(), interests, keptAhead.Milliseconds()),
		"filmInterests", describeFilmInterests(interests, keptAhead.Milliseconds()))
}

// ask is a player's ask for chunk wanted of the store: an evicted store refuses it when it is neither
// made nor on its way (ErrNoRoomToMake, D250 §6), and plans no run (runTracker.plan): only a player
// its eviction left without its chunks is told why it stops. Refusing every ask, a chunk made and
// served was answered 500, and a player whose quality another run was making at 2.5x playback asked
// for it every two seconds for minutes (the owner's film on nas.example, 2026-10-17). For a player's
// ask, isAsked, it returns what its interest wants, which the run planned for it must be; every run
// for the server's own start, or where nothing says.
func (service *Service) ask(
	ctx context.Context, film cutFilm, stream Stream, wanted int, isAsked bool,
) (wantedness, error) {
	dir := film.storeDir(stream)
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

// storeIDOf is the chunk_store row of a stream of a film by its store's key (cutFilm.storeKey), made
// the first time it is named.
func (service *Service) storeIDOf(ctx context.Context, fileID int64, stream Stream, storeKey string) (int64, error) {
	var storeID int64
	err := service.cache.Write(ctx, func(queries *cachegen.Queries) error {
		var writeErr error
		storeID, writeErr = queries.EnsureChunkStore(ctx, cachegen.EnsureChunkStoreParams{
			SourceFile: fileID, Stream: stream.Name(), StoreKey: storeKey,
		})
		return writeErr
	})
	if err != nil {
		return 0, fmt.Errorf("record the store of %s: %w", StoreDir(fileID, stream, storeKey), err)
	}
	return storeID, nil
}
