// SPDX-License-Identifier: AGPL-3.0-or-later

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
		"Movies/Arkon (1979)/Arkon (1979).mkv": "arkon",
		"Movies/Arkon (1979)/Arkon (1979).nfo": "<movie/>",
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
	records, err := NewScanner([]*confine.MediaRoot{media}, prober, options).List(t.Context(), media, everything)
	if err != nil {
		t.Fatal(err)
	}
	// Every rule that is the library's is the core's now: trash, samples, AppleDouble and extensions
	// are listed like anything else (D240 §3).
	want := []string{
		"scanBegin",
		"listed .trash/",
		"listed .trash/old.mkv",
		"listed Movies/",
		"listed Movies/._Metropolis.mkv",
		"listed Movies/Arkon (1979)/",
		"listed Movies/Arkon (1979)/Arkon (1979).mkv",
		"listed Movies/Arkon (1979)/Arkon (1979).nfo",
		"listed Movies/Metropolis (1995)/",
		"listed Movies/Metropolis (1995)/Sample/",
		"listed Movies/Metropolis (1995)/Sample/clip.mkv",
		"skip Movies/pipe.mkv notRegular",
		"listed notes.txt",
		"scanComplete",
	}
	if got := summary(records); !slices.Equal(got, want) {
		t.Errorf("listing:\n  %s\nwant:\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
	if len(prober.probed) != 0 {
		t.Errorf("a listing probed %v; it reads no file", prober.probed)
	}
	arkon := records[6].Listed
	info, err := os.Stat(filepath.Join(root, "Movies", "Arkon (1979)", "Arkon (1979).mkv"))
	if err != nil {
		t.Fatal(err)
	}
	if arkon.SizeBytes != int64(len("arkon")) || arkon.ModifiedAt != info.ModTime().UnixMilli() ||
		arkon.FilesystemDevice == 0 {
		t.Errorf("listed %+v, want its size, modified time and device", arkon)
	}
	// Its inode tells it moved on its disk (D304 §5).
	if inode, _ := confine.InodeOf(info); inode == 0 || arkon.Inode != inode {
		t.Errorf("listed inode %d, want %d", arkon.Inode, inode)
	}
	if complete := records[len(records)-1].ScanComplete; complete.ListedCount != 11 || complete.SkippedCount != 1 {
		t.Errorf("scanComplete = %+v, want 11 listed and 1 skipped", complete)
	}
	if begin := records[0].ScanBegin; begin.FilesystemDevice == 0 {
		t.Errorf("scanBegin = %+v, want the root's device, as a scan's", begin)
	}
}

func TestList_GoesOnlyAsDeepAsAskedFromWhereAsked(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"Movies/Arkon (1979)/Arkon (1979).mkv": "arkon",
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
		{".", 1, []string{"scanBegin", "listed Movies/", "listed Shows/", "scanComplete"}},
		{"Movies", 1, []string{
			"scanBegin", "listed Movies/Arkon (1979)/", "listed Movies/loose.mkv", "scanComplete",
		}},
		{"Movies", 0, []string{
			"scanBegin", "listed Movies/Arkon (1979)/", "listed Movies/Arkon (1979)/Arkon (1979).mkv",
			"listed Movies/loose.mkv", "scanComplete",
		}},
	} {
		listing := everything
		listing.Path, listing.Depth = each.start, each.depth
		records, err := scanner.List(t.Context(), media, listing)
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
	shows := writeTree(t, map[string]string{"Shared/Arkon.mkv": "arkon"})
	elsewhere := writeTree(t, map[string]string{"secret.mkv": "not media", "shadow": "root:x:0:0"})
	links := map[string]string{
		"into another root.mkv": filepath.Join(shows, "Shared", "Arkon.mkv"),
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
		"scanBegin",
		"listed Metropolis (1995)/",
		"listed Metropolis (1995)/Metropolis.mkv",
		"listed a whole directory/",
		"listed a whole directory/Arkon.mkv",
		"error dangling.mkv entry notFound",
		"skip etc shadow.mkv symlinkOutsideRoots",
		"listed into another root.mkv",
		"listed loop/",
		"skip out of the roots.mkv symlinkOutsideRoots",
		"scanComplete",
	}
	if got := summary(listAll(t, scanner, filmsRoot)); !slices.Equal(got, want) {
		t.Errorf("listing:\n  %s\nwant:\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
	// What a link into another root names is read there; what leads out of the roots is not read.
	examined := summary(examineAll(t, scanner, filmsRoot, video("into another root.mkv"), video("etc shadow.mkv")))
	if want := []string{"observed into another root.mkv", "error etc shadow.mkv entry ioError"}; !slices.Equal(
		examined, want) {
		t.Errorf("examined %v, want %v (D206)", examined, want)
	}
}

func TestExamine_ReadsEachFileAsItsKindSaysOrSaysWhyNot(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"Arkon/Arkon.mkv":    "arkon",
		"Arkon/Arkon.nfo":    "<movie/>",
		"Arkon/Arkon.en.srt": "1\n00:00:01,000 --> 00:00:02,000\nHello\n",
		"Arkon/Broken.mkv":   "broken",
	})
	media := nodetest.OpenMediaRoot(t, root)
	prober := &fakeProber{failing: map[string]error{"Broken.mkv": errors.New("moov atom not found")}}
	files := []nodeproto.ExamineFile{
		{Path: nodeproto.Path{Text: "Arkon/Arkon.mkv"}, Kind: nodeproto.ExamineVideo},
		{Path: nodeproto.Path{Text: "Arkon/Arkon.nfo"}, Kind: nodeproto.ExamineNFO},
		{Path: nodeproto.Path{Text: "Arkon/Arkon.en.srt"}, Kind: nodeproto.ExamineSubtitle},
		{Path: nodeproto.Path{Text: "Arkon/Broken.mkv"}, Kind: nodeproto.ExamineVideo},
		{Path: nodeproto.Path{Text: "Arkon/Gone.mkv"}, Kind: nodeproto.ExamineVideo},
		{Path: nodeproto.Path{Text: "Arkon"}, Kind: nodeproto.ExamineVideo},
	}
	betweenCount := 0
	records, err := NewScanner([]*confine.MediaRoot{media}, prober, options).Examine(t.Context(), media, files,
		func() error { betweenCount++; return nil })
	if err != nil {
		t.Fatal(err)
	}
	// Two errors of one code in one directory stay two: each file's answer is its own.
	want := []string{
		"observed Arkon/Arkon.mkv",
		"sidecar Arkon/Arkon.nfo nfo",
		"sidecar Arkon/Arkon.en.srt subtitle",
		"error Arkon/Broken.mkv entry probeFailed",
		"error Arkon/Gone.mkv entry notFound",
		"error Arkon entry ioError",
	}
	if got := summary(records); !slices.Equal(got, want) {
		t.Errorf("examined:\n  %s\nwant:\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
	checkSidecarBodies(t, records)
	if observed := records[0].Observed; observed.SizeBytes != int64(len("arkon")) ||
		observed.Identity.PayloadHash == "" {
		t.Errorf("observed = %+v, want its size and payload hash", observed)
	}
	info, err := os.Stat(filepath.Join(root, "Arkon", "Arkon.mkv"))
	if err != nil {
		t.Fatal(err)
	}
	inode, _ := confine.InodeOf(info)
	if observed := records[0].Observed; observed.FilesystemDevice == 0 || inode == 0 || observed.Inode != inode {
		t.Errorf("observed on device %d at inode %d, want inode %d: the core keeps them to tell a move (D304 §5)",
			observed.FilesystemDevice, observed.Inode, inode)
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
	records, err := scanner.List(ctx, media, everything)
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
