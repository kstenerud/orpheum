// SPDX-License-Identifier: AGPL-3.0-or-later

package scan

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/disc/disctest"
	"github.com/kstenerud/orpheum/internal/node/diskwatch"
	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/node/nodetest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock"
)

// streamReadingProber keeps the film each probe is given, and what reading it through its stream
// found while the probe ran, as ffprobe reads it.
type streamReadingProber struct {
	films    []execx.Film
	contents []string
}

func (prober *streamReadingProber) Probe(ctx context.Context, film execx.Film) (nodeproto.Probe, error) {
	prober.films = append(prober.films, film)
	if film.Stream != nil {
		read := make([]byte, film.Size)
		if err := film.Stream.Source.ReadAt(ctx, read, 0); err != nil {
			return nodeproto.Probe{}, fmt.Errorf("read the streamed film: %w", err)
		}
		prober.contents = append(prober.contents, string(read))
	}
	return nodeproto.Probe{Format: nodeproto.ProbeFormat{FormatNames: []string{"matroska"}}}, nil
}

// On a node whose FFmpeg reads through it, a scan's probe is served the file the scanner opened, on a
// probe's terms, and given no path to it; on one whose FFmpeg cannot, it reads it by its path (D330).
func TestExamine_AProbeIsServedTheFileTheScannerOpenedWhenTheNodeStreamsFilms(t *testing.T) {
	t.Parallel()
	media := nodetest.OpenMediaRoot(t, writeTree(t, map[string]string{"Film/film.mkv": "a film"}))
	runner, err := execx.NewRunner(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	streaming := options
	streaming.Films, streaming.Disks, streaming.FilmReadWindow = runner, diskwatch.NewGate(clock.System{}), time.Minute
	byPath := &streamReadingProber{}
	examineAll(t, NewScanner([]*confine.MediaRoot{media}, byPath, streaming), media, video("Film/film.mkv"))
	runner.StreamFilms()
	streamed := &streamReadingProber{}
	examineAll(t, NewScanner([]*confine.MediaRoot{media}, streamed, streaming), media, video("Film/film.mkv"))
	wantPath := media.Path() + "/Film/film.mkv"
	if len(byPath.films) != 1 || byPath.films[0].Stream != nil ||
		!slices.Equal(byPath.films[0].Readable(), []string{wantPath}) {
		t.Errorf("a node whose FFmpeg reads by path probed %+v; want %s by its path", byPath.films, wantPath)
	}
	if len(streamed.films) != 1 || streamed.films[0].Stream == nil || len(streamed.films[0].Readable()) != 0 {
		t.Fatalf("a node that streams films probed %+v; want the film streamed, no path granted", streamed.films)
	}
	if !slices.Equal(streamed.contents, []string{"a film"}) {
		t.Errorf("the probe's stream served %q; want the file the scanner opened", streamed.contents)
	}
	facts, err := os.Stat(wantPath)
	if err != nil {
		t.Fatal(err)
	}
	whole, err := runner.FilmOf(t.Context(), wantPath, execx.OpenedFilm{Facts: facts}, execx.ReadsWholeFilm)
	if err != nil {
		t.Fatal(err)
	}
	if probe := streamed.films[0].Stream; probe.BudgetBytes >= whole.Stream.BudgetBytes {
		t.Errorf("the probe may be served %d bytes, a whole read %d; want a probe's budget, the smaller",
			probe.BudgetBytes, whole.Stream.BudgetBytes)
	}
}

// A disc image's probe is served its main title, the disc's facts with it; a node whose FFmpeg reads
// by path cannot serve one, and says so in the file's error rather than probing the image's raw bytes
// (D350).
func TestExamine_ADiscImageIsProbedAsItsMainTitleOnlyWhenStreamed(t *testing.T) {
	t.Parallel()
	dvd := disctest.DVD{Titles: []disctest.DVDTitle{{Cells: []disctest.DVDCell{
		{Data: disctest.Sectors("the film", 2), DurationMs: 5000},
	}}}}
	image := string(disctest.Volume{Files: dvd.Files()}.Bytes())
	media := nodetest.OpenMediaRoot(t, writeTree(t, map[string]string{"Film/Film (2001).iso": image}))
	runner, err := execx.NewRunner(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	streaming := options
	streaming.Films, streaming.Disks, streaming.FilmReadWindow = runner, diskwatch.NewGate(clock.System{}), time.Minute
	byPath := &streamReadingProber{}
	iso := video("Film/Film (2001).iso")
	records := examineAll(t, NewScanner([]*confine.MediaRoot{media}, byPath, options), media, iso)
	if len(byPath.films) != 0 || len(records) != 1 || records[0].Error == nil ||
		records[0].Error.Code != nodeproto.CodeProbeFailed ||
		!strings.Contains(records[0].Error.PlatformError.Text, "fdproxy") {
		t.Errorf("a node that reads by path examined a disc image as %+v, probing %d; want a probe failure saying "+
			"the image needs a streamed FFmpeg", records, len(byPath.films))
	}
	runner.StreamFilms()
	streamed := &streamReadingProber{}
	examineAll(t, NewScanner([]*confine.MediaRoot{media}, streamed, streaming), media, iso)
	if len(streamed.films) != 1 || streamed.films[0].Disc == nil || streamed.films[0].Disc.DurationMs != 5000 ||
		!slices.Equal(streamed.contents, []string{string(disctest.Sectors("the film", 2))}) {
		t.Errorf("a node that streams films probed %+v serving %d films' bytes; want the image's main title, "+
			"its disc facts with it", streamed.films, len(streamed.contents))
	}
}
