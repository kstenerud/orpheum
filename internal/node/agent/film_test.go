// SPDX-License-Identifier: AGPL-3.0-or-later

package agent

import (
	"context"
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/node/prepare"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// filmSeen is the film a run was given, and what reading it through its stream found.
type filmSeen struct {
	film     execx.Film
	contents []byte
	readErr  error
}

// filmRecordingPreparer tells films each film a prepare is given, read through its stream while the
// prepare runs, as FFmpeg reads it.
type filmRecordingPreparer struct{ films chan filmSeen }

func (preparer filmRecordingPreparer) Prepare(
	ctx context.Context, source execx.Film, _ nodeproto.Prepare, _ func(preparedThroughMs int64),
) (prepare.Made, error) {
	seen := filmSeen{film: source}
	if source.Stream != nil {
		seen.contents = make([]byte, source.Size)
		seen.readErr = source.Stream.Source.ReadAt(ctx, seen.contents, 0)
	}
	preparer.films <- seen
	return prepare.Made{}, nil
}

// On a node whose FFmpeg reads through it, a run is served the very file the node opened and checked
// was the one the core named, and is given no path to it, on the terms the node sets for a whole
// read; on one whose FFmpeg cannot, it reads that file by its path, as before (D330).
func TestAgent_ARunIsServedTheFileTheNodeCheckedOnlyWhenItsFFmpegReadsThroughIt(t *testing.T) {
	t.Parallel()
	for _, isStreamed := range []bool{true, false} {
		core, socket := startFakeCore(t)
		preparer := filmRecordingPreparer{films: make(chan filmSeen, 1)}
		root, _ := startAgentWithWorkers(t, socket, Workers{
			Scanner: fakeScanner{}, Artworker: fakeArtworker{}, Preparer: preparer, StreamsFilms: isStreamed,
		})
		core.sendPrepare(1, preparableFilm(t, root), "3/remux/0123456789abcdef")
		seen := testutil.Receive(t, preparer.films, "the prepare's film")
		wantPath := root.Path() + "/film.mkv"
		if seen.film.Path != wantPath || seen.film.Size != int64(len("a film")) {
			t.Errorf("streamed %v: the film is %s of %d bytes; want %s of 6", isStreamed, seen.film.Path,
				seen.film.Size, wantPath)
		}
		if isStreamed {
			checkServedTheCheckedFile(t, seen)
		} else if seen.film.Stream != nil || !slices.Equal(seen.film.Readable(), []string{wantPath}) {
			t.Errorf("a node whose FFmpeg reads by path streamed the film, or did not grant its path: %+v",
				seen.film)
		}
		finalPrepareReport(t, core)
	}
}

// checkServedTheCheckedFile checks a run was given its film streamed, with no path, the stream serving
// the file the node checked, on the terms of a whole read.
func checkServedTheCheckedFile(t *testing.T, seen filmSeen) {
	t.Helper()
	stream := seen.film.Stream
	switch {
	case stream == nil || len(seen.film.Readable()) != 0:
		t.Errorf("a node whose FFmpeg reads through it gave the run %+v; want the film streamed, no path",
			seen.film)
	case seen.readErr != nil || string(seen.contents) != "a film":
		t.Errorf("the stream served %q, %v; want the file the node checked", seen.contents, seen.readErr)
	case stream.Run < 1 || stream.BudgetBytes != wholeFilmBudget(6) || stream.MostOutstanding != 2 ||
		stream.DataPipeBytes != 1<<20 || seen.film.CacheBlocks != filmCacheBlocks:
		t.Errorf("the stream's terms are %+v, %d blocks kept; want a run's number, a whole read's "+
			"budget, two READs, a 1 MiB pipe and %d blocks", *stream, seen.film.CacheBlocks, filmCacheBlocks)
	}
}
