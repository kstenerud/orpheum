// SPDX-License-Identifier: GPL-3.0-or-later

package scan

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/node/nodetest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

func TestList_ListsEveryEntryWithItsStatAndReadsNoFile(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"Movies/Wing (1979)/Wing (1979).mkv": "alien",
		"Movies/Wing (1979)/Wing (1979).nfo": "<movie/>",
		"Movies/Metropolis (1995)/Sample/clip.mkv":   "debris",
		"Movies/._Metropolis.mkv":                    "\x00\x05\x16\x07",
		".trash/old.mkv":                       "old",
		"notes.txt":                            "not media",
	})
	if err := syscall.Mkfifo(filepath.Join(root, "Movies", "pipe.mkv"), 0o600); err != nil {
		t.Fatal(err)
	}
	media := nodetest.OpenMediaRoot(t, root)
	prober := &fakeProber{}
	records, err := NewScanner([]*confine.MediaRoot{media}, prober, options).List(t.Context(), media, ".", 0)
	if err != nil {
		t.Fatal(err)
	}
	// Every rule that is the library's is the core's now: trash, samples, AppleDouble and extensions
	// are listed like anything else (D240 §3).
	want := []string{
		"scan_begin",
		"listed .trash/",
		"listed .trash/old.mkv",
		"listed Movies/",
		"listed Movies/._Metropolis.mkv",
		"listed Movies/Wing (1979)/",
		"listed Movies/Wing (1979)/Wing (1979).mkv",
		"listed Movies/Wing (1979)/Wing (1979).nfo",
		"listed Movies/Metropolis (1995)/",
		"listed Movies/Metropolis (1995)/Sample/",
		"listed Movies/Metropolis (1995)/Sample/clip.mkv",
		"skip Movies/pipe.mkv not_regular",
		"listed notes.txt",
		"scan_complete",
	}
	if got := summary(records); !slices.Equal(got, want) {
		t.Errorf("listing:\n  %s\nwant:\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
	if len(prober.probed) != 0 {
		t.Errorf("a listing probed %v; it reads no file", prober.probed)
	}
	alien := records[6].Listed
	info, err := os.Stat(filepath.Join(root, "Movies", "Wing (1979)", "Wing (1979).mkv"))
	if err != nil {
		t.Fatal(err)
	}
	if alien.SizeBytes != int64(len("alien")) || alien.ModifiedAt != info.ModTime().UnixMilli() ||
		alien.FilesystemDevice == 0 {
		t.Errorf("listed %+v, want its size, modified time and device", alien)
	}
	if complete := records[len(records)-1].ScanComplete; complete.ListedCount != 11 || complete.SkippedCount != 1 {
		t.Errorf("scan_complete = %+v, want 11 listed and 1 skipped", complete)
	}
	if begin := records[0].ScanBegin; begin.FilesystemDevice == 0 {
		t.Errorf("scan_begin = %+v, want the root's device, as a scan's", begin)
	}
}

func TestList_GoesOnlyAsDeepAsAskedFromWhereAsked(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"Movies/Wing (1979)/Wing (1979).mkv": "alien",
		"Movies/loose.mkv":                     "loose",
		"Shows/Dark/S01E01.mkv":                "dark",
	})
	media := nodetest.OpenMediaRoot(t, root)
	scanner := NewScanner([]*confine.MediaRoot{media}, &fakeProber{}, options)
	for _, each := range []struct {
		start string
		depth int
		want  []string
	}{
		{".", 1, []string{"scan_begin", "listed Movies/", "listed Shows/", "scan_complete"}},
		{"Movies", 1, []string{
			"scan_begin", "listed Movies/Wing (1979)/", "listed Movies/loose.mkv", "scan_complete",
		}},
		{"Movies", 0, []string{
			"scan_begin", "listed Movies/Wing (1979)/", "listed Movies/Wing (1979)/Wing (1979).mkv",
			"listed Movies/loose.mkv", "scan_complete",
		}},
	} {
		records, err := scanner.List(t.Context(), media, each.start, each.depth)
		if err != nil {
			t.Fatal(err)
		}
		if got := summary(records); !slices.Equal(got, each.want) {
			t.Errorf("list %s to %d:\n  %s\nwant:\n  %s", each.start, each.depth, strings.Join(got, "\n  "),
				strings.Join(each.want, "\n  "))
		}
	}
}

func TestList_ListsALinkOnlyIntoAMediaRoot(t *testing.T) {
	t.Parallel()
	films := writeTree(t, map[string]string{"Metropolis (1995)/Metropolis.mkv": "heat"})
	shows := writeTree(t, map[string]string{"Shared/Wing.mkv": "alien"})
	elsewhere := writeTree(t, map[string]string{"secret.mkv": "not media", "shadow": "root:x:0:0"})
	links := map[string]string{
		"into another root.mkv": filepath.Join(shows, "Shared", "Wing.mkv"),
		"a whole directory":     filepath.Join(shows, "Shared"),
		"out of the roots.mkv":  filepath.Join(elsewhere, "secret.mkv"),
		"etc shadow.mkv":        filepath.Join(elsewhere, "shadow"),
		"loop":                  films,
		"dangling.mkv":          filepath.Join(films, "gone.mkv"),
	}
	for name, target := range links {
		if err := os.Symlink(target, filepath.Join(films, name)); err != nil {
			t.Fatal(err)
		}
	}
	filmsRoot, showsRoot := nodetest.OpenMediaRoot(t, films), nodetest.OpenMediaRoot(t, shows)
	scanner := NewScanner([]*confine.MediaRoot{filmsRoot, showsRoot}, &fakeProber{}, options)
	// A directory reached again through a link is listed, and not walked twice (D206).
	want := []string{
		"scan_begin",
		"listed Metropolis (1995)/",
		"listed Metropolis (1995)/Metropolis.mkv",
		"listed a whole directory/",
		"listed a whole directory/Wing.mkv",
		"error dangling.mkv entry not_found",
		"skip etc shadow.mkv symlink_outside_roots",
		"listed into another root.mkv",
		"listed loop/",
		"skip out of the roots.mkv symlink_outside_roots",
		"scan_complete",
	}
	if got := summary(listAll(t, scanner, filmsRoot)); !slices.Equal(got, want) {
		t.Errorf("listing:\n  %s\nwant:\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
	// What a link into another root names is read there; what leads out of the roots is not read.
	examined := summary(examineAll(t, scanner, filmsRoot, video("into another root.mkv"), video("etc shadow.mkv")))
	if want := []string{"observed into another root.mkv", "error etc shadow.mkv entry io_error"}; !slices.Equal(
		examined, want) {
		t.Errorf("examined %v, want %v (D206)", examined, want)
	}
}

func TestExamine_ReadsEachFileAsItsKindSaysOrSaysWhyNot(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"Wing/Wing.mkv":    "alien",
		"Wing/Wing.nfo":    "<movie/>",
		"Wing/Wing.en.srt": "1\n00:00:01,000 --> 00:00:02,000\nHello\n",
		"Wing/Broken.mkv":   "broken",
	})
	media := nodetest.OpenMediaRoot(t, root)
	prober := &fakeProber{failing: map[string]error{"Broken.mkv": errors.New("moov atom not found")}}
	files := []nodeproto.ExamineFile{
		{Path: nodeproto.Path{Text: "Wing/Wing.mkv"}, Kind: nodeproto.ExamineVideo},
		{Path: nodeproto.Path{Text: "Wing/Wing.nfo"}, Kind: nodeproto.ExamineNFO},
		{Path: nodeproto.Path{Text: "Wing/Wing.en.srt"}, Kind: nodeproto.ExamineSubtitle},
		{Path: nodeproto.Path{Text: "Wing/Broken.mkv"}, Kind: nodeproto.ExamineVideo},
		{Path: nodeproto.Path{Text: "Wing/Gone.mkv"}, Kind: nodeproto.ExamineVideo},
		{Path: nodeproto.Path{Text: "Wing"}, Kind: nodeproto.ExamineVideo},
	}
	betweenCount := 0
	records, err := NewScanner([]*confine.MediaRoot{media}, prober, options).Examine(t.Context(), media, files,
		func() error { betweenCount++; return nil })
	if err != nil {
		t.Fatal(err)
	}
	// Two errors of one code in one directory stay two: each file's answer is its own.
	want := []string{
		"observed Wing/Wing.mkv",
		"sidecar Wing/Wing.nfo nfo",
		"sidecar Wing/Wing.en.srt subtitle",
		"error Wing/Broken.mkv entry probe_failed",
		"error Wing/Gone.mkv entry not_found",
		"error Wing entry io_error",
	}
	if got := summary(records); !slices.Equal(got, want) {
		t.Errorf("examined:\n  %s\nwant:\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
	checkSidecarBodies(t, records)
	if observed := records[0].Observed; observed.SizeBytes != int64(len("alien")) ||
		observed.Identity.PayloadHash == "" {
		t.Errorf("observed = %+v, want its size and payload hash", observed)
	}
	if betweenCount != len(files) {
		t.Errorf("between was called %d times, want once before each of %d files", betweenCount, len(files))
	}
}

func TestExamine_StopsWhenToldToBetweenFiles(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{"a.mkv": "a", "b.mkv": "b"})
	media := nodetest.OpenMediaRoot(t, root)
	files := []nodeproto.ExamineFile{
		{Path: nodeproto.Path{Text: "a.mkv"}, Kind: nodeproto.ExamineVideo},
		{Path: nodeproto.Path{Text: "b.mkv"}, Kind: nodeproto.ExamineVideo},
	}
	stop := errors.New("the node is stopping")
	calls := 0
	scanner := NewScanner([]*confine.MediaRoot{media}, &fakeProber{}, options)
	records, err := scanner.Examine(t.Context(), media, files, func() error {
		calls++
		if calls == 2 {
			return stop
		}
		return nil
	})
	if !errors.Is(err, stop) {
		t.Errorf("Examine = %v, want the error between gave", err)
	}
	if got := summary(records); !slices.Equal(got, []string{"observed a.mkv"}) {
		t.Errorf("examined %v, want only the file before it stopped", got)
	}
}

// A listing moves with each entry it lists, and an examining with each read of a hash too, so the
// node can tell one that has stalled on its disk from one still working (D233 §5).
func TestListAndExamine_MoveWithEachRecordAndEachRead(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{"Film/film.mkv": strings.Repeat("v", 1<<16), "Film/film.nfo": "<movie/>"})
	media := nodetest.OpenMediaRoot(t, root)
	scanner := NewScanner([]*confine.MediaRoot{media}, &fakeProber{}, options)
	var moves atomic.Int64
	ctx := execx.WithMoved(t.Context(), func() { moves.Add(1) })
	records, err := scanner.List(ctx, media, ".", 0)
	if err != nil {
		t.Fatal(err)
	}
	listed := 0
	for _, record := range records {
		if record.Type == nodeproto.RecordListed {
			listed++
		}
	}
	if got := moves.Load(); listed == 0 || got < int64(listed) {
		t.Errorf("a listing of %d entries moved %d times; want once an entry", listed, got)
	}
	moves.Store(0)
	examined, err := scanner.Examine(ctx, media, []nodeproto.ExamineFile{video("Film/film.mkv")},
		func() error { return nil })
	if err != nil || len(examined) != 1 {
		t.Fatalf("examined %+v, %v", examined, err)
	}
	if got := moves.Load(); got <= 1 {
		t.Errorf("an examining of one video moved %d times; want its hashes' reads too, not its record alone", got)
	}
}
