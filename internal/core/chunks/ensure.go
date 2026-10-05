// SPDX-License-Identifier: GPL-3.0-or-later

package chunks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/kstenerud/orpheum/internal/core/jobs"
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
// stream named Stream, on the grid named GridID.
type runArgs struct {
	Stream     string `json:"stream"`
	GridID     string `json:"gridId"`
	FirstChunk int    `json:"firstChunk"`
	LastChunk  int    `json:"lastChunk"`
}

// Ensure makes sure a player asking for chunk wanted of a film's stream will have it and the chunks
// after it: of those that start within lookaheadMs of film from it, the first neither made nor on
// its way starts a run, at play priority on the film's node, through the earliest of runSpanMs of
// film, the chunk before the next made or on its way, and the film's last chunk. A run is never
// queued over chunks a run will make. A store with runs on their way is kept wanted (D234 §3). A
// film with no grid yet is ErrNoGrid.
func (service *Service) Ensure(ctx context.Context, fileID int64, stream Stream, wanted int) error {
	film, isCut, err := service.cut(ctx, fileID)
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
	first, last, isPlanned := service.runs.plan(dir, film.grid, wanted)
	if !isPlanned {
		if !service.runs.isMaking(dir) {
			return nil
		}
		return service.queue.KeepWanted(ctx, jobs.KindMakeChunks, jobs.Target{FileID: fileID})
	}
	args, err := json.Marshal(runArgs{Stream: stream.Name(), GridID: gridID, FirstChunk: first, LastChunk: last})
	if err != nil {
		service.runs.forget(dir, first)
		return fmt.Errorf("encode a run's arguments: %w", err)
	}
	// The queue holds one queued job of a kind for a file: a run asked for while another of the film's
	// waits in it, of another stream, is not queued, and is planned again at the player's next ask.
	isQueued, err := service.queue.Enqueue(ctx, jobs.Request{
		Kind: jobs.KindMakeChunks, Target: jobs.Target{FileID: fileID}, ArgsJSON: string(args),
		Priority: jobs.PriorityPlayNow, Reason: "a player is waiting for its chunks",
		Destination: jobs.NodeDestination(film.file.NodeID),
	})
	if err != nil || !isQueued {
		service.runs.forget(dir, first)
	}
	if err != nil {
		return fmt.Errorf("queue chunks %d to %d of %s: %w", first, last, dir, err)
	}
	return nil
}
