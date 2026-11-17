// SPDX-License-Identifier: AGPL-3.0-or-later

package agent

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/node/disc"
	"github.com/kstenerud/orpheum/internal/node/disc/disctest"
	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/node/nodetest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// filmSeen is the film a run was given, and what reading it through its stream found.
type filmSeen struct {
	film     execx.Film
	contents []byte
	readErr  error
}

// filmRecordingChunkMaker tells films each film a run is given, read through its stream while the run
// goes on, as FFmpeg reads it.
type filmRecordingChunkMaker struct{ films chan filmSeen }

func (maker filmRecordingChunkMaker) Make(
	ctx context.Context, source execx.Film, command nodeproto.MakeChunks, _ func(n int),
) (int, error) {
	seen := filmSeen{film: source}
	if source.Stream != nil {
		seen.contents = make([]byte, source.Size)
		seen.readErr = source.Stream.Source.ReadAt(ctx, seen.contents, 0)
	}
	maker.films <- seen
	return command.FirstChunk, nil
}

// On a node whose FFmpeg reads through it, a run is served the very file the node opened and checked
// was the one the core named, and is given no path to it, on the terms the node sets for a whole
// read; on one whose FFmpeg cannot, it reads that file by its path, as before (D330).
func TestAgent_ARunIsServedTheFileTheNodeCheckedOnlyWhenItsFFmpegReadsThroughIt(t *testing.T) {
	t.Parallel()
	for _, isStreamed := range []bool{true, false} {
		core, socket := startFakeCore(t)
		maker := filmRecordingChunkMaker{films: make(chan filmSeen, 1)}
		root, _ := startAgentWithWorkers(t, socket, Workers{
			Scanner: fakeScanner{}, Artworker: fakeArtworker{}, ChunkMaker: maker, Films: filmsRunner(t, isStreamed),
		})
		core.sendRun(1, chunkableFilm(t, root), "3/chunks-picture/0123456789abcdef")
		seen := testutil.Receive(t, maker.films, "the run's film")
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
		finalRunReport(t, core)
	}
}

// A run of a disc image is served the image's main title, the run told which it is, on a whole
// read's terms; a node whose FFmpeg reads by path refuses it, saying why (D350 §3).
func TestAgent_ARunOfADiscImageIsServedItsMainTitle(t *testing.T) {
	t.Parallel()
	dvd := disctest.DVD{Titles: []disctest.DVDTitle{{Cells: []disctest.DVDCell{
		{Data: disctest.Sectors("the film", 3), DurationMs: 9000},
	}}}}
	image := disctest.Volume{Files: dvd.Files()}.Bytes()
	for _, isStreamed := range []bool{true, false} {
		core, socket := startFakeCore(t)
		maker := filmRecordingChunkMaker{films: make(chan filmSeen, 1)}
		root, _ := startAgentWithWorkers(t, socket, Workers{
			Scanner: fakeScanner{}, Artworker: fakeArtworker{}, ChunkMaker: maker, Films: filmsRunner(t, isStreamed),
		})
		if err := os.WriteFile(filepath.Join(root.Path(), "Film.iso"), image, 0o600); err != nil {
			t.Fatal(err)
		}
		named := nodeproto.BytesFile{
			RootPath: root.Path(), Path: []byte("Film.iso"), PayloadHash: nodetest.PayloadHashHex(t, string(image)),
		}
		core.sendMakeChunks(1, nodeproto.MakeChunks{
			Video: named, StoreDir: "3/chunks-picture/0123456789abcdef",
			Stream: nodeproto.ChunkStream{Kind: nodeproto.StreamPicture}, BoundsMs: []int64{0, 2000, 4000},
		})
		if !isStreamed {
			if report := receiveChunksReport(t, core); !strings.Contains(report.Refusal, "fdproxy") {
				t.Errorf("a node whose FFmpeg reads by path ended a disc's run %+v; want a refusal saying why",
					report)
			}
			continue
		}
		seen := testutil.Receive(t, maker.films, "the run's film")
		if seen.film.Disc == nil || seen.film.Disc.Kind != disc.DVD || seen.readErr != nil ||
			string(seen.contents) != string(disctest.Sectors("the film", 3)) || seen.film.Stream.BudgetBytes < 64<<20 {
			t.Errorf("a disc's run was given %+v serving %.20q… (%v); want its DVD's main title, on a whole "+
				"read's terms",
				seen.film, seen.contents, seen.readErr)
		}
		receiveChunksReport(t, core)
	}
}

// checkServedTheCheckedFile checks a run was given its film streamed, with no path, the stream serving
// the file the node checked, within a whole read's budget.
func checkServedTheCheckedFile(t *testing.T, seen filmSeen) {
	t.Helper()
	stream := seen.film.Stream
	switch {
	case stream == nil || len(seen.film.Readable()) != 0:
		t.Errorf("a node whose FFmpeg reads through it gave the run %+v; want the film streamed, no path",
			seen.film)
	case seen.readErr != nil || string(seen.contents) != "a film":
		t.Errorf("the stream served %q, %v; want the file the node checked", seen.contents, seen.readErr)
	case stream.BudgetBytes < 64<<20:
		t.Errorf("the stream's budget is %d bytes; want a whole read's, past its floor", stream.BudgetBytes)
	}
}

// filmsRunner is a runner that makes films, streaming them when isStreamed.
func filmsRunner(t *testing.T, isStreamed bool) *execx.Runner {
	t.Helper()
	runner, err := execx.NewRunner(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if isStreamed {
		runner.StreamFilms()
	}
	return runner
}

func (filmRecordingChunkMaker) StretchOf(nodeproto.MakeChunks) execx.Stretch { return execx.Stretch{} }

// stretchedChunkMaker is a filmRecordingChunkMaker whose every run reads stretch of its film.
type stretchedChunkMaker struct {
	filmRecordingChunkMaker

	stretch execx.Stretch
}

func (maker stretchedChunkMaker) StretchOf(nodeproto.MakeChunks) execx.Stretch { return maker.stretch }

// stretchRecordingFilms makes films as its runner does, telling stretches the stretch each run's
// film was opened for, nil for a run that reads it whole, and saying a film opened for one was cut
// down.
type stretchRecordingFilms struct {
	runner    *execx.Runner
	stretches chan *execx.Stretch
}

func (films stretchRecordingFilms) FilmOf(
	ctx context.Context, absolutePath string, opened execx.OpenedFilm, reading execx.FilmReading,
) (execx.Film, error) {
	films.stretches <- opened.Stretch
	film, err := films.runner.FilmOf(ctx, absolutePath, opened, reading)
	if err != nil {
		return execx.Film{}, fmt.Errorf("the film of %s: %w", absolutePath, err)
	}
	film.IsIndexCutDown = opened.Stretch != nil
	return film, nil
}

// A run of chunks opens its film for the stretch its chunk maker says the run reads, to which a long
// MP4 index is cut down, and its log line says when it was (D512).
func TestAgent_ARunOfChunksOpensItsFilmForTheStretchItReads(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	want := execx.Stretch{FromMs: 123_000, ToMs: 456_000}
	maker := stretchedChunkMaker{filmRecordingChunkMaker{films: make(chan filmSeen, 1)}, want}
	films := stretchRecordingFilms{runner: filmsRunner(t, true), stretches: make(chan *execx.Stretch, 1)}
	logs := &testutil.Log{}
	root, _ := startAgentWithWorkers(t, socket, Workers{
		Scanner: fakeScanner{}, Artworker: fakeArtworker{}, ChunkMaker: maker, Films: films,
	}, func(node *Agent) { node.logger = slog.New(slog.NewJSONHandler(logs, nil)) })
	core.sendRun(1, chunkableFilm(t, root), "3/chunks-picture/0123456789abcdef")
	if got := testutil.Receive(t, films.stretches, "the run's film's stretch"); got == nil || *got != want {
		t.Errorf("a run of chunks opened its film for the stretch %v; want %v", got, want)
	}
	testutil.Receive(t, maker.films, "the run's film")
	finalRunReport(t, core)
	checkLogLine(t, logs.AwaitEvent(t, "chunkRunFinished"), `"indexCutDown":true`)
}
