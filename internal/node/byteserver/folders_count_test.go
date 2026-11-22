// SPDX-License-Identifier: AGPL-3.0-or-later

package byteserver_test

import (
	"context"
	"maps"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/node/byteserver"
	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/diskwatch"
	"github.com/kstenerud/orpheum/internal/node/nodetest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// Each folder listed carries how many videos it holds, in it and below it, as a scan of it would read
// them: no sample, AppleDouble file, sidecar or image, nothing in a sample, system or disc's folder,
// and no link followed; a hidden folder's videos are read, as a scan reads them (D539 §7).
func TestServeFolders_CountsEachFoldersVideosAsAScanWouldReadThem(t *testing.T) {
	t.Parallel()
	directory, outside := t.TempDir(), t.TempDir()
	for _, file := range []string{
		"films/Arkon (1979)/Arkon (1979).mkv", "films/Arkon (1979)/Arkon (1979).nfo",
		"films/Arkon (1979)/Arkon (1979).srt", "films/Arkon (1979)/poster.jpg", "films/Arkon (1979)/sample.mkv",
		"films/Arkon (1979)/._Arkon (1979).mkv", "films/Arkon (1979)/Sample/x.mkv", "films/a.iso",
		"films/a/VIDEO_TS/a.vob", "films/@eaDir/a.mkv", "films/.hidden/b.MP4", "films/b/c/clip.mp4",
		"shows/notes.txt", outside + "/a.mkv",
	} {
		place := filepath.Join(directory, file)
		if filepath.IsAbs(file) {
			place = file
		}
		if err := os.MkdirAll(filepath.Dir(place), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(place, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(outside, filepath.Join(directory, "films", "elsewhere")); err != nil {
		t.Fatal(err)
	}
	linked := filepath.Join(directory, "films", "b", "a.mkv")
	if err := os.Symlink(filepath.Join(outside, "a.mkv"), linked); err != nil {
		t.Fatal(err)
	}
	root := nodetest.OpenMediaRoot(t, directory)
	handler := byteserver.New([]*confine.MediaRoot{root}).Handler()
	_, answer := folders(t, handler, root.Path())
	if want := map[string]int64{"films": 4, "shows": 0}; !maps.Equal(answer.VideoCounts, want) {
		t.Errorf("the root's folders' counts: %v, want %v", answer.VideoCounts, want)
	}
	_, answer = folders(t, handler, root.Path()+"/films")
	// @eaDir is listed, and counted as a scan of it, chosen as a library's folder, would read it.
	want := map[string]int64{"@eaDir": 1, "Arkon (1979)": 1, "a": 0, "b": 1}
	if !maps.Equal(answer.VideoCounts, want) {
		t.Errorf("films' folders' counts: %v, want %v: none for a hidden folder or a link", answer.VideoCounts, want)
	}
}

// A listing's counts end once its budget does, about 300 ms in all: a folder counted by then keeps
// its count, the folder being counted and every one after it carry none, never a part of one, and
// the listing is answered readable with every folder. A count cut short so leaves the share not
// stuck: the next listing counts them all (D539 §7).
func TestServeFolders_AFolderNotCountedWithinTheBudgetCarriesNoCount(t *testing.T) {
	t.Parallel()
	root := nodetest.OpenMediaRoot(t, t.TempDir())
	start := time.Date(2026, 11, 22, 9, 0, 0, 0, time.UTC)
	countClock := clocktest.NewFake(start)
	gate := diskwatch.NewGate(clocktest.NewFake(start))
	var countsOfB atomic.Int32
	countingB := make(chan struct{}, 1)
	handler := byteserver.New([]*confine.MediaRoot{root}).WatchingOpens(gate, 30*time.Second).
		ListingFoldersWith(func(*confine.MediaRoot, string) ([]string, error) {
			return []string{"a", "b", "c"}, nil
		}).
		CountingVideosWith(func(ctx context.Context, _ *confine.MediaRoot, relative string) (int64, error) {
			switch relative {
			case "a":
				return 3, nil
			case "b":
				if countsOfB.Add(1) == 1 { // a large folder on a slow disk
					countingB <- struct{}{}
					<-ctx.Done()
					return 0, ctx.Err()
				}
				return 7, nil
			}
			return 5, nil
		}, countClock).Handler()
	answered := make(chan nodeproto.FolderListing, 1)
	go func() {
		_, answer := folders(t, handler, root.Path())
		answered <- answer
	}()
	testutil.Receive(t, countingB, "the count of b")
	countClock.Advance(300 * time.Millisecond)
	answer := testutil.Receive(t, answered, "the listing's answer")
	if want := map[string]int64{"a": 3}; answer.State != nodeproto.ProbeReadable || len(answer.Folders) != 3 ||
		!maps.Equal(answer.VideoCounts, want) {
		t.Errorf("a listing whose budget ran out counting b: %+v, want readable with all three, counts %v",
			answer, want)
	}
	_, answer = folders(t, handler, root.Path())
	if want := map[string]int64{"a": 3, "b": 7, "c": 5}; answer.State != nodeproto.ProbeReadable ||
		!maps.Equal(answer.VideoCounts, want) {
		t.Errorf("the next listing: %+v, want readable with counts %v: the share is not stuck", answer, want)
	}
}
