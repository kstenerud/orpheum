// SPDX-License-Identifier: AGPL-3.0-or-later

package execx

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/node/disc"
	"github.com/kstenerud/orpheum/internal/node/disc/disctest"
	"github.com/kstenerud/orpheum/internal/node/diskwatch"
	"github.com/kstenerud/orpheum/internal/node/fdproxy"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// protocolsListing is the start of ffmpeg -protocols from the patched build, then its output
// protocols.
const protocolsListing = "Supported file protocols:\nInput:\n  async\n  bluray\n  fd\n  fdproxy\n  file\n" +
	"Output:\n  file\n  pipe\n"

// A node streams its films only to an FFmpeg that lists fdproxy among the protocols it reads: the
// stock build, which does not, reads each film by its path (D330), and so does one that names it only
// as an output.
func TestHasFDProxy_IsTheProtocolAmongTheInputsFFmpegLists(t *testing.T) {
	t.Parallel()
	for _, example := range []struct {
		name, listing string
		want          bool
	}{
		{"the patched build", protocolsListing, true},
		{"the stock build", "Supported file protocols:\nInput:\n  async\n  file\nOutput:\n  file\n", false},
		{"an output only", "Supported file protocols:\nInput:\n  file\nOutput:\n  fdproxy\n", false},
	} {
		runner := newRunner(t, testutil.InstallFakeFFmpeg(t, testutil.FakeReply{FFmpegStdout: example.listing}))
		got, err := runner.HasFDProxy(t.Context())
		if err != nil || got != example.want {
			t.Errorf("%s: %v, %v; want %v", example.name, got, err, example.want)
		}
	}
}

// A runner that streams films serves each from the file the node opened, through its disk's gate,
// numbering each run and bounding it by what its kind reads: a probe far less than a whole read. One
// that does not reads each by its path (D330).
func TestFilmOf_StreamsTheOpenedFileOnTheTermsOfWhatTheRunReads(t *testing.T) {
	t.Parallel()
	path, opened := openedTestFilm(t)
	runner, err := NewRunner(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	film := filmOf(t, runner, path, opened, ReadsWholeFilm)
	if film.Stream != nil || film.Path != path || film.Size != 1000 {
		t.Errorf("a runner that does not stream made %+v; want the film by its path", film)
	}
	runner.StreamFilms()
	whole, probe := filmOf(t, runner, path, opened, ReadsWholeFilm), filmOf(t, runner, path, opened, ReadsIndex)
	if whole.Stream == nil || probe.Stream == nil {
		t.Fatalf("a runner that streams made %+v and %+v; want both streamed", whole, probe)
	}
	if whole.Stream.Run < 1 || probe.Stream.Run == whole.Stream.Run {
		t.Errorf("runs numbered %d and %d; want each its own, from 1", whole.Stream.Run, probe.Stream.Run)
	}
	wantWhole, wantProbe := int64(1250+filmBudgetFloorBytes), int64(1000/16+filmBudgetFloorBytes)
	if whole.Stream.BudgetBytes != wantWhole || probe.Stream.BudgetBytes != wantProbe {
		t.Errorf("budgets %d for a whole read and %d for a probe; want %d, the film and a quarter, and %d, "+
			"a sixteenth, over the floor", whole.Stream.BudgetBytes, probe.Stream.BudgetBytes, wantWhole, wantProbe)
	}
	if whole.CacheBlocks != filmCacheBlocks || whole.Stream.MostOutstanding != 2 ||
		whole.Stream.DataPipeBytes != 1<<20 {
		t.Errorf("terms %+v, %d blocks kept; want two READs, a 1 MiB pipe and %d blocks", *whole.Stream,
			whole.CacheBlocks, filmCacheBlocks)
	}
	if err := whole.Stream.Source.ReadAt(t.Context(), make([]byte, 10), 990); err != nil {
		t.Errorf("the stream could not read the opened file: %v", err)
	}
}

// A disc image's film is its main title: served as the image's runs that hold it, sized and
// budgeted as the title, and carrying it for the probe; a runner that cannot stream refuses it, and
// an image that is no disc says so (D350 §3).
func TestFilmOf_ADiscImageIsServedAsItsMainTitleAndOnlyStreamed(t *testing.T) {
	t.Parallel()
	bluRay := disctest.BluRay{
		Playlists: []disctest.BluRayPlaylist{{Number: 5, Items: []disctest.BluRayItem{
			{Clip: "00001", OutMs: 1000}, {Clip: "00002", OutMs: 1000},
		}}},
		Clips: map[string][]byte{"00001": disctest.M2TS("one", 40), "00002": disctest.M2TS("two", 35)},
	}
	path, opened := openedTestImage(t, disctest.Volume{Files: bluRay.Files(), Metadata: true}.Bytes())
	runner, err := NewRunner(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runner.FilmOf(t.Context(), path, opened, ReadsIndex); !errors.Is(err, ErrDiscNeedsStreaming) {
		t.Errorf("a runner that reads by path made a disc's film with %v; want ErrDiscNeedsStreaming", err)
	}
	runner.StreamFilms()
	film := filmOf(t, runner, path, opened, ReadsIndex)
	want := string(disctest.M2TS("one", 40)) + string(disctest.M2TS("two", 35))
	served := make([]byte, film.Size)
	if readErr := film.Stream.Source.ReadAt(t.Context(), served, 0); readErr != nil || string(served) != want {
		t.Errorf("the disc's film served %.30q… (%v); want its playlist's clips, joined", served, readErr)
	}
	if film.Disc == nil || film.Disc.Number != 5 || film.Size != int64(len(want)) ||
		film.Stream.Source.Size() != film.Size || film.Stream.BudgetBytes != film.Size/16+filmBudgetFloorBytes {
		t.Errorf("the disc's film is %+v; want playlist 5, sized and budgeted as its title of %d bytes",
			film, len(want))
	}
	_, notDisc := openedTestFilm(t)
	notDisc.IsDiscImage = true
	if _, err := runner.FilmOf(t.Context(), path, notDisc, ReadsIndex); !errors.Is(err, disc.ErrNotADisc) {
		t.Errorf("a file named a disc image that is none made a film with %v; want disc.ErrNotADisc", err)
	}
}

// A DVD's film whose clock starts again partway is served restamped onto one clock, however a read
// falls across its sectors: what any read gives is what the title restamped whole gives there
// (D404).
func TestFilmOf_ADVDsFilmIsServedOnOneClock(t *testing.T) {
	t.Parallel()
	const second = 90_000
	dvd := disctest.DVD{Titles: []disctest.DVDTitle{{Cells: []disctest.DVDCell{
		{Data: disctest.VOBU(25_200, 25_200+600*second), DurationMs: 600_000},
		{Data: disctest.VOBU(10_000, 10_000+300*second), DurationMs: 300_000},
	}}}}
	image := disctest.Volume{Files: dvd.Files()}.Bytes()
	path, opened := openedTestImage(t, image)
	runner, err := NewRunner(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runner.StreamFilms()
	film := filmOf(t, runner, path, opened, ReadsIndex)
	if film.Disc == nil || len(film.Disc.ClockShifts) != 1 {
		t.Fatalf("the DVD's film is %+v; want its title, its clock shifted once", film.Disc)
	}
	whole := make([]byte, film.Size)
	if err := film.Stream.Source.ReadAt(t.Context(), whole, 0); err != nil {
		t.Fatal(err)
	}
	if scr, _, _ := disctest.VOBUTicks(whole[2*disc.SectorBytes:]); scr != 25_200+600*second {
		t.Errorf("the second cell is served at SCR %d; want %d, carrying on from the first", scr, 25_200+600*second)
	}
	reads := [][2]int64{{3*disc.SectorBytes + 20, 1000}, {1000, 2*disc.SectorBytes + 100}, {film.Size - 7, 7}}
	for _, read := range reads {
		part := make([]byte, read[1])
		if err := film.Stream.Source.ReadAt(t.Context(), part, read[0]); err != nil ||
			!bytes.Equal(part, whole[read[0]:read[0]+read[1]]) {
			t.Errorf("%d bytes read at %d are not the title's there, restamped whole (%v)", read[1], read[0], err)
		}
	}
	if err := film.Stream.Source.ReadAt(t.Context(), make([]byte, 10), film.Size-5); err == nil {
		t.Error("a read past the title's end was served; want it refused")
	}
}

// filmOf is runner's film of the file opened at path, for a run that reads it as reading.
func filmOf(t *testing.T, runner *Runner, path string, opened OpenedFilm, reading FilmReading) Film {
	t.Helper()
	film, err := runner.FilmOf(t.Context(), path, opened, reading)
	if err != nil {
		t.Fatal(err)
	}
	return film
}

// openedTestImage is a disc image opened as the node opens one, and its path.
func openedTestImage(t *testing.T, image []byte) (string, OpenedFilm) {
	t.Helper()
	path, opened := openedTestFilm(t)
	path = filepath.Join(filepath.Dir(path), "film.iso")
	if err := os.WriteFile(path, image, 0o600); err != nil {
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
	opened.File, opened.IsDiscImage = file, true
	return path, opened
}

// openedTestFilm is a film of 1,000 bytes opened as the node opens one, and its path.
func openedTestFilm(t *testing.T) (string, OpenedFilm) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "film.mkv")
	if err := os.WriteFile(path, make([]byte, 1000), 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() }) //nolint:errcheck // the test's own file
	facts, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	return path, OpenedFilm{
		File: file, Facts: facts, Disks: diskwatch.NewGate(clock.System{}), Root: "/", Window: time.Minute,
	}
}

// A streamed run passed only having exited 0, said DONE for its input, that DONE OK and for its own
// run, and given the node no reason to fail it (fdproxy's specification, Judging a Run): a run that
// read part of its film never passes as complete.
func TestJudgeFilm_PassesOnlyARunThatExited0WithDoneOKForItsOwnRun(t *testing.T) {
	t.Parallel()
	exitedNonZero := &RunError{Tool: FFmpeg, ExitStatus: 1, Err: errors.New("exit status 1")}
	doneOK := &fdproxy.Done{Run: 7, IsOK: true}
	for _, example := range []struct {
		name    string
		account fdproxy.Account
		ended   error
		// says is something the verdict's error must say; empty for a pass.
		says string
		want error
	}{
		{name: "DONE OK for its run", account: fdproxy.Account{Done: doneOK}},
		{name: "no DONE", account: fdproxy.Account{}, says: "no DONE"},
		{
			name: "DONE ERR, though it exited 0",
			account: fdproxy.Account{Done: &fdproxy.Done{
				Run: 7, Kind: fdproxy.KindChannel, Code: 5, Message: "Input/output error",
			}},
			says: "Input/output error",
		},
		{
			name: "DONE OK for another run", account: fdproxy.Account{Done: &fdproxy.Done{Run: 8, IsOK: true}},
			says: "names run 8", want: fdproxy.ErrViolation,
		},
		{
			name: "the node's own reason, its DONE OK",
			account: fdproxy.Account{
				Done: doneOK, Failure: fdproxy.ErrOverBudget,
			},
			want: fdproxy.ErrOverBudget,
		},
		{
			name: "a nonzero exit, its DONE OK", account: fdproxy.Account{Done: doneOK},
			ended: exitedNonZero, want: exitedNonZero,
		},
	} {
		err := judgeFilm(FFmpeg, example.account, 7, &tailBuffer{}, example.ended)
		isPass := example.says == "" && example.want == nil
		switch {
		case isPass && err != nil:
			t.Errorf("%s: %v; want a pass", example.name, err)
		case !isPass && err == nil:
			t.Errorf("%s: passed; want a failure", example.name)
		case example.want != nil && !errors.Is(err, example.want):
			t.Errorf("%s: %v; want %v", example.name, err, example.want)
		case example.says != "" && !strings.Contains(err.Error(), example.says):
			t.Errorf("%s: %v; want it to say %q", example.name, err, example.says)
		}
	}
}
