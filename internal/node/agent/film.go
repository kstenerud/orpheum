// SPDX-License-Identifier: AGPL-3.0-or-later

package agent

import (
	"cmp"
	"context"

	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/readonly"
)

// Films makes the film a run reads from the file the node opened: execx.Runner's FilmOf, which
// streams it when the node's FFmpeg can be streamed one (D330).
type Films interface {
	FilmOf(absolutePath string, opened execx.OpenedFilm, reading execx.FilmReading) execx.Film
}

// openFilm opens the film named for one run that reads it through, once it is the file the core named
// (D100), and makes it the run's film (Workers.Films): streamed from the very file the node opened and
// checked, through its disk's gate, or by its path. The caller calls done once the run has ended.
func (agent *Agent) openFilm(ctx context.Context, named nodeproto.BytesFile) (film execx.Film, done func(), err error) {
	file, facts, err := agent.openNamed(ctx, named)
	if err != nil {
		return execx.Film{}, nil, err
	}
	path := agent.roots[named.RootPath].Path() + "/" + string(named.Path)
	if agent.workers.Films == nil {
		readonly.Close(file)
		return execx.Film{Path: path, Size: facts.Size()}, func() {}, nil
	}
	// A block of the film answers as an open does: a disk asleep spins up within the window.
	window := cmp.Or(agent.workers.OpenStallWindow, defaultOpenStallWindow)
	film = agent.workers.Films.FilmOf(path, execx.OpenedFilm{
		File: file, Facts: facts, Disks: agent.diskGate, Root: named.RootPath, Window: window,
	}, execx.ReadsWholeFilm)
	return film, func() { readonly.Close(file) }, nil
}
