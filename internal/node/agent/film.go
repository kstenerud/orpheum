// SPDX-License-Identifier: AGPL-3.0-or-later

package agent

import (
	"cmp"
	"context"

	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/node/ffstreamer"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/readonly"
)

// filmBudgetFloorBytes is the least a run may be served of its film, so a small film, its index or a
// block's rounding never passes the budget (ffstreamer's PROTOCOL.md §9.6).
const filmBudgetFloorBytes = 64 << 20

// filmDataPipeBytes is the size asked for the pipe that carries a film's bytes: at the default 64 KiB,
// throughput halved in measurement (§4.3).
const filmDataPipeBytes = 1 << 20

// filmMostOutstanding is how many READs FFmpeg may have unanswered: the block it waits for, and the
// next, prefetched as it reads straight through.
const filmMostOutstanding = 2

// filmCacheBlocks is how many of a streamed film's blocks FFmpeg keeps (§10.2). FFmpeg reads a few
// seconds of every track as it opens a film, whatever the run maps, so a film whose tracks are stored
// one after another is read from as many places as it has tracks, sweeping them over and over, and a
// cache smaller than the sweep thrashes: an 11-track film was served 25 times its size at 19 or 32
// blocks of 1 MiB, and a third of it at 48 (orpheum-ffmpeg's bench/interleave.py, 2026-10-22).
const filmCacheBlocks = 48

// wholeFilmBudget is the most a run that reads its film through may be served: the film once, a
// quarter more for blocks read twice (an interleaved film measured 1.04 to 1.12 times its size), and
// the floor.
func wholeFilmBudget(sizeBytes int64) int64 {
	return sizeBytes + sizeBytes/4 + filmBudgetFloorBytes
}

// openFilm opens the film named for one run, once it is the file the core named (D100): read by its
// path, or, on a node whose FFmpeg reads through it (Workers.StreamsFilms, D330), served to the run
// over the file the node opened and checked, through its disk's gate, the run given no path to it.
// The caller calls done once the run has ended.
func (agent *Agent) openFilm(ctx context.Context, named nodeproto.BytesFile) (film execx.Film, done func(), err error) {
	file, facts, err := agent.openNamed(ctx, named)
	if err != nil {
		return execx.Film{}, nil, err
	}
	film = execx.Film{Path: agent.roots[named.RootPath].Path() + "/" + string(named.Path), Size: facts.Size()}
	if !agent.workers.StreamsFilms {
		readonly.Close(file)
		return film, func() {}, nil
	}
	// A block of the film answers as an open does: a disk asleep spins up within the window.
	window := cmp.Or(agent.workers.OpenStallWindow, defaultOpenStallWindow)
	film.Stream = &ffstreamer.Config{
		Run:             agent.filmRuns.Add(1),
		Source:          ffstreamer.NewFileSource(file, facts, agent.diskGate, named.RootPath, film.Path, window),
		MostOutstanding: filmMostOutstanding,
		BudgetBytes:     wholeFilmBudget(facts.Size()),
		DataPipeBytes:   filmDataPipeBytes,
	}
	film.CacheBlocks = filmCacheBlocks
	return film, func() { readonly.Close(file) }, nil
}
