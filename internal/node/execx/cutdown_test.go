// SPDX-License-Identifier: AGPL-3.0-or-later

package execx

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/node/container"
	"github.com/kstenerud/orpheum/internal/node/container/mp4test"
)

// longMP4 is an MP4 of a picture of 20,000 samples and a sound of 40,000, its movie box last, its
// first track's sample description of kind.
func longMP4(kind string) []byte {
	shown := mp4test.MP4Track{Description: kind, Timescale: 24_000, SamplesPerChunk: 5}
	heard := mp4test.MP4Track{Description: "mp4a", Timescale: 48_000, SamplesPerChunk: 40}
	for n := range 40_000 {
		if n < 20_000 {
			shown.Durations, shown.Sizes = append(shown.Durations, 1000), append(shown.Sizes, uint32(4+n%5))
			if n%48 == 0 {
				shown.Syncs = append(shown.Syncs, uint32(n+1))
			}
		}
		heard.Durations, heard.Sizes = append(heard.Durations, 1024), append(heard.Sizes, 6)
	}
	return mp4test.MP4File(mp4test.MP4{Timescale: 1000, Tracks: []mp4test.MP4Track{shown, heard}})
}

// openedTestMP4 is the bytes written as a film named name and opened as the node opens one, and its
// path.
func openedTestMP4(t *testing.T, contents []byte, name string) (string, OpenedFilm) {
	t.Helper()
	path, opened := openedTestFilm(t)
	path = filepath.Join(filepath.Dir(path), name)
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() }) //nolint:errcheck // the test's own file
	if opened.Facts, err = file.Stat(); err != nil {
		t.Fatal(err)
	}
	opened.File = file
	return path, opened
}

// served is all the film's stream serves of it.
func served(t *testing.T, film Film) []byte {
	t.Helper()
	contents := make([]byte, film.Stream.Source.Size())
	if err := film.Stream.Source.ReadAt(t.Context(), contents, 0); err != nil {
		t.Fatal(err)
	}
	return contents
}

// A run of chunks of a streamed MP4 whose movie box is long is served the film with its index cut
// down to the run's stretch: the file's bytes but its movie box's, whose place a smaller one and a
// free box fill; a probe, and any run that reads the film whole, is served the file as it is, as is
// an MP4 whose movie box is shorter than cutDownMinMoovBytes and a film of another container
// (D512).
func TestFilmOf_ARunOfChunksOfALongIndexedMP4IsServedItsIndexCutDown(t *testing.T) {
	t.Parallel()
	contents := longMP4("avc1")
	path, opened := openedTestMP4(t, contents, "film.mp4")
	chunksOf := opened
	chunksOf.Stretch = &Stretch{FromMs: 400_000, ToMs: 430_000}
	short := filmOf(t, cuttingRunner(t, cutDownMinMoovBytes), path, chunksOf, ReadsWholeFilm)
	if short.IsIndexCutDown || !bytes.Equal(served(t, short), contents) {
		t.Fatal("a film whose movie box is shorter than cutDownMinMoovBytes was cut down; want it served as it is")
	}
	runner := cuttingRunner(t, 1)
	checkServedCutDown(t, filmOf(t, runner, path, chunksOf, ReadsWholeFilm), contents)
	for name, reading := range map[string]FilmReading{"a probe": ReadsIndex, "a read-through": ReadsWholeFilm} {
		film := filmOf(t, runner, path, opened, reading)
		if film.IsIndexCutDown || !bytes.Equal(served(t, film), contents) {
			t.Errorf("%s is served the film cut down; want it as it is", name)
		}
	}
	otherPath, other := openedTestMP4(t, contents, "film.mkv")
	other.Stretch = chunksOf.Stretch
	if film := filmOf(t, runner, otherPath, other, ReadsWholeFilm); film.IsIndexCutDown {
		t.Error("a film named a Matroska file is served cut down; want it as it is")
	}
}

// cuttingRunner is a runner that streams films and cuts down the indexes of fromBytes or more.
func cuttingRunner(t *testing.T, fromBytes int64) *Runner {
	t.Helper()
	runner, err := NewRunner(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runner.StreamFilms()
	runner.cutDownFromBytes = fromBytes
	return runner
}

// checkServedCutDown holds the film served to be the contents with their movie box cut down: every
// byte outside it the contents', and its place a smaller one and a free box.
func checkServedCutDown(t *testing.T, film Film, contents []byte) {
	t.Helper()
	moovAt, moov, err := container.FindMoov(bytes.NewReader(contents), int64(len(contents)))
	if err != nil {
		t.Fatal(err)
	}
	cut := served(t, film)
	switch {
	case !film.IsIndexCutDown || len(cut) != len(contents):
		t.Fatalf("a run of chunks is served %d bytes of the film's %d, cut down %v; want them all, cut down",
			len(cut), len(contents), film.IsIndexCutDown)
	case !bytes.Equal(cut[:moovAt], contents[:moovAt]) || !bytes.Equal(cut[moov.End:], contents[moov.End:]):
		t.Fatal("a run of chunks is served bytes outside the movie box other than the film's")
	case bytes.Equal(cut[moovAt:moov.End], contents[moovAt:moov.End]),
		!bytes.Contains(cut[moovAt:moov.End], []byte("free")):
		t.Fatal("a run of chunks is served the film's movie box; want it cut down, a free box after it")
	}
}

// A long-indexed MP4 whose index is not cut down is served as it is, and logged once, with why: an
// encrypted track's (D512).
func TestFilmOf_AnMP4WhoseIndexIsNotCutDownIsLoggedOnceWithWhy(t *testing.T) {
	t.Parallel()
	contents := longMP4("encv")
	path, opened := openedTestMP4(t, contents, "film.mp4")
	opened.Stretch = &Stretch{FromMs: 400_000, ToMs: 430_000}
	runner := cuttingRunner(t, 1)
	var logged bytes.Buffer
	runner.LoggingTo(slog.New(slog.NewTextHandler(&logged, nil)))
	for range 2 {
		film := filmOf(t, runner, path, opened, ReadsWholeFilm)
		if film.IsIndexCutDown || !bytes.Equal(served(t, film), contents) {
			t.Fatal("an encrypted MP4 is served cut down; want it as it is")
		}
	}
	if lines := strings.Count(logged.String(), "D512"); lines != 1 || !strings.Contains(logged.String(), "encrypted") {
		t.Errorf("two runs of an encrypted MP4 logged %q; want one line saying it is encrypted", logged.String())
	}
}
