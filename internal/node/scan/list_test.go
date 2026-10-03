// SPDX-License-Identifier: GPL-3.0-or-later

package scan

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"

	"github.com/kstenerud/orpheum/internal/node/confine"
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
		alien.Device == 0 {
		t.Errorf("listed %+v, want its size, modified time and device", alien)
	}
	if complete := records[len(records)-1].ScanComplete; complete.ListedCount != 11 || complete.ObservedCount != 0 {
		t.Errorf("scan_complete = %+v, want 11 listed and nothing observed", complete)
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
	outside := writeTree(t, map[string]string{"secret.mkv": "not ours"})
	root := writeTree(t, map[string]string{"Movies/Metropolis.mkv": "heat"})
	if err := os.Symlink(filepath.Join(root, "Movies"), filepath.Join(root, "Films")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.mkv"), filepath.Join(root, "Secret.mkv")); err != nil {
		t.Fatal(err)
	}
	media := nodetest.OpenMediaRoot(t, root)
	records, err := NewScanner([]*confine.MediaRoot{media}, &fakeProber{}, options).List(t.Context(), media, ".", 0)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"scan_begin", "listed Films/", "listed Films/Metropolis.mkv", "listed Movies/",
		"skip Secret.mkv symlink_outside_roots", "scan_complete",
	}
	if got := summary(records); !slices.Equal(got, want) {
		t.Errorf("listing:\n  %s\nwant:\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
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
