// SPDX-License-Identifier: GPL-3.0-or-later

package prepared

import (
	"bytes"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
)

var now = time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)

// shelfWith makes a shelf over runs, each of its size, last used that long ago, with the disk as
// full as diskPercent says.
func shelfWith(t *testing.T, budget Budget, diskPercent *int, runs map[string]struct {
	bytes int
	ago   time.Duration
},
) *Shelf {
	t.Helper()
	directory := t.TempDir()
	for dir, made := range runs {
		path := filepath.Join(directory, filepath.FromSlash(dir))
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "seg00001.m4s"), make([]byte, made.bytes), 0o600); err != nil {
			t.Fatal(err)
		}
		used := now.Add(-made.ago)
		if err := os.Chtimes(path, used, used); err != nil {
			t.Fatal(err)
		}
	}
	shelf := New(directory, budget, clocktest.NewFake(now), slog.New(slog.DiscardHandler))
	shelf.diskUsedPercent = func(string) (int, error) { return *diskPercent, nil }
	return shelf
}

// remainingRunDirs are the run directories still on the shelf, sorted.
func remainingRunDirs(t *testing.T, shelf *Shelf) []string {
	t.Helper()
	runs, err := shelf.listRuns()
	if err != nil {
		t.Fatal(err)
	}
	dirs := make([]string, 0, len(runs))
	for _, each := range runs {
		dirs = append(dirs, each.renditionDir)
	}
	slices.Sort(dirs)
	return dirs
}

func TestEvict_RemovesTheLeastRecentlyUsedPastTheirKeptTimeFirstAndNeverOneBeingWritten(t *testing.T) {
	t.Parallel()
	disk := 50
	budget := Budget{MaxBytes: 250, KeptAfterLastPlay: 72 * time.Hour, DiskUsedLimitPercent: 95}
	shelf := shelfWith(t, budget, &disk, map[string]struct {
		bytes int
		ago   time.Duration
	}{
		"1/remux/aaaaaaaaaaaaaaaa": {100, 10 * 24 * time.Hour}, // oldest, past its grace, but being written
		"2/remux/bbbbbbbbbbbbbbbb": {100, 5 * 24 * time.Hour},  // past its grace
		"3/remux/cccccccccccccccc": {100, 1 * time.Hour},       // within its grace, least recent within
		"4/h264/dddddddddddddddd":  {100, 10 * time.Minute},    // within its grace
	})
	done := shelf.MarkWriting("1/remux/aaaaaaaaaaaaaaaa")
	defer done()
	usage, err := shelf.Evict(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	// 400 bytes against 250: the one past its grace goes; the one being written stays; then the
	// least recent within its grace, as the budget still needs it.
	want := []string{"1/remux/aaaaaaaaaaaaaaaa", "4/h264/dddddddddddddddd"}
	if got := remainingRunDirs(t, shelf); !slices.Equal(got, want) || usage.UsedBytes != 200 {
		t.Errorf("left %v with %d bytes, want %v with 200 (D217 §5)", got, usage.UsedBytes, want)
	}
	if _, err := os.Stat(filepath.Join(shelf.directory, "2")); !os.IsNotExist(err) {
		t.Errorf("an evicted run's empty file folder is still there: %v", err)
	}
}

// Every eviction is logged, with why and the numbers that made it: one within its kept time as a
// warning, one past it as information. A disk past its limit evicted a film while it played on
// staging, and nothing said so (2026-10-06).
func TestEvict_SaysWhatItRemovedAndWhy(t *testing.T) {
	t.Parallel()
	disk := 97
	budget := Budget{MaxBytes: 1 << 30, KeptAfterLastPlay: 72 * time.Hour, DiskUsedLimitPercent: 95}
	shelf := shelfWith(t, budget, &disk, map[string]struct {
		bytes int
		ago   time.Duration
	}{
		"1/remux/aaaaaaaaaaaaaaaa": {100, 5 * 24 * time.Hour},
		"2/remux/bbbbbbbbbbbbbbbb": {100, 1 * time.Hour},
	})
	var logged bytes.Buffer
	shelf.logger = slog.New(slog.NewTextHandler(&logged, nil))
	if _, err := shelf.Evict(t.Context()); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(logged.String()), "\n")
	if len(lines) != 2 ||
		!strings.Contains(lines[0], "level=INFO") || !strings.Contains(lines[0], "1/remux/aaaaaaaaaaaaaaaa") ||
		!strings.Contains(lines[1], "level=WARN") || !strings.Contains(lines[1], "2/remux/bbbbbbbbbbbbbbbb") ||
		!strings.Contains(lines[1], "its disk is past its limit") || !strings.Contains(lines[1], "diskUsedPercent=97") {
		t.Errorf("the evictions were logged as:\n%s\nwant the one past its kept time as INFO, then the one within "+
			"it as WARN, each saying the disk is past its limit at 97%%", logged.String())
	}
}

func TestEvict_ADiskPastItsLimitEvictsWhateverTheBudgetSays(t *testing.T) {
	t.Parallel()
	disk := 97
	budget := Budget{MaxBytes: 1 << 30, KeptAfterLastPlay: 72 * time.Hour, DiskUsedLimitPercent: 95}
	shelf := shelfWith(t, budget, &disk, map[string]struct {
		bytes int
		ago   time.Duration
	}{
		"1/remux/aaaaaaaaaaaaaaaa": {100, 5 * 24 * time.Hour},
		"2/remux/bbbbbbbbbbbbbbbb": {100, 1 * time.Hour},
	})
	shelf.diskUsedPercent = func(string) (int, error) {
		if len(remainingRunDirs(t, shelf)) < 2 {
			return 90, nil // one eviction brings the disk under its limit
		}
		return disk, nil
	}
	if _, err := shelf.Evict(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := remainingRunDirs(t, shelf); !slices.Equal(got, []string{"2/remux/bbbbbbbbbbbbbbbb"}) {
		t.Errorf("left %v, want the older one evicted for the disk, though within the budget", got)
	}
}

func TestMakeRoom_RefusesWhenTheDiskIsPastItsLimitWithNothingToEvict(t *testing.T) {
	t.Parallel()
	disk := 99
	shelf := shelfWith(t, Budget{MaxBytes: 1 << 30, KeptAfterLastPlay: time.Hour, DiskUsedLimitPercent: 95}, &disk, nil)
	if err := shelf.MakeRoom(t.Context()); !errors.Is(err, ErrNoRoom) {
		t.Errorf("MakeRoom with the disk 99%% full and nothing to evict: %v, want ErrNoRoom", err)
	}
	disk = 50
	if err := shelf.MakeRoom(t.Context()); err != nil {
		t.Errorf("MakeRoom with the disk half full: %v, want room", err)
	}
}

// A rendition served within the watched window is never evicted, whatever the disk says: the next
// prepare is refused instead, and the log names what was kept. On staging a disk past its limit
// evicted the film being watched, and it stopped (the owner, 2026-10-06).
func TestEvict_NeverRemovesOneBeingWatched(t *testing.T) {
	t.Parallel()
	disk := 97
	budget := Budget{
		MaxBytes: 1 << 30, KeptAfterLastPlay: 72 * time.Hour, WatchedWithin: 10 * time.Minute,
		DiskUsedLimitPercent: 95,
	}
	shelf := shelfWith(t, budget, &disk, map[string]struct {
		bytes int
		ago   time.Duration
	}{
		"1/remux/aaaaaaaaaaaaaaaa": {100, time.Hour},       // played earlier: it may go
		"2/remux/bbbbbbbbbbbbbbbb": {100, 2 * time.Minute}, // being watched
	})
	var logged bytes.Buffer
	shelf.logger = slog.New(slog.NewTextHandler(&logged, nil))
	if _, err := shelf.Evict(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := remainingRunDirs(t, shelf); !slices.Equal(got, []string{"2/remux/bbbbbbbbbbbbbbbb"}) {
		t.Errorf("left %v, want only the one being watched", got)
	}
	if !strings.Contains(logged.String(), "kept prepared renditions being watched") ||
		!strings.Contains(logged.String(), "2/remux/bbbbbbbbbbbbbbbb") {
		t.Errorf("the log does not name the rendition kept for being watched:\n%s", logged.String())
	}
	if err := shelf.MakeRoom(t.Context()); !errors.Is(err, ErrNoRoom) {
		t.Errorf("MakeRoom with the disk still 97%% full and only a film being watched: %v, want ErrNoRoom", err)
	}
}

func TestMarkServed_MakesARenditionTheMostRecentlyUsed(t *testing.T) {
	t.Parallel()
	disk := 50
	budget := Budget{MaxBytes: 100, KeptAfterLastPlay: time.Hour, DiskUsedLimitPercent: 95}
	shelf := shelfWith(t, budget, &disk, map[string]struct {
		bytes int
		ago   time.Duration
	}{
		"1/remux/aaaaaaaaaaaaaaaa": {100, 5 * time.Hour},
		"2/remux/bbbbbbbbbbbbbbbb": {100, 2 * time.Hour},
	})
	shelf.MarkServed("1/remux/aaaaaaaaaaaaaaaa") // played now: the other is the least recently used
	if _, err := shelf.Evict(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := remainingRunDirs(t, shelf); !slices.Equal(got, []string{"1/remux/aaaaaaaaaaaaaaaa"}) {
		t.Errorf("left %v, want the one just served kept", got)
	}
}

// The disk keeps its floor of free space whatever its percentage says (node.prepared_disk_min_free_mib): below
// it, renditions are evicted, and a prepare with nothing left to evict is refused.
func TestMakeRoom_KeepsTheDisksFloorOfFreeSpace(t *testing.T) {
	t.Parallel()
	disk := 50
	shelf := shelfWith(t, Budget{
		MaxBytes: 1 << 30, KeptAfterLastPlay: time.Hour, DiskUsedLimitPercent: 95, MinFreeBytes: 100 << 20,
	}, &disk, map[string]struct {
		bytes int
		ago   time.Duration
	}{"1/remux/aaaaaaaaaaaaaaaa": {100, 5 * 24 * time.Hour}})
	free := int64(10 << 20)
	shelf.diskAvailableBytes = func(string) (int64, error) {
		if len(remainingRunDirs(t, shelf)) == 0 {
			return free, nil
		}
		return free / 2, nil
	}
	if err := shelf.MakeRoom(t.Context()); !errors.Is(err, ErrNoRoom) {
		t.Errorf("MakeRoom with 10 MiB free and a floor of 100: %v, want ErrNoRoom", err)
	}
	if got := remainingRunDirs(t, shelf); len(got) != 0 {
		t.Errorf("left %v below the floor, want every rendition evicted for it", got)
	}
	free = 200 << 20
	if err := shelf.MakeRoom(t.Context()); err != nil {
		t.Errorf("MakeRoom with 200 MiB free: %v, want room", err)
	}
}
