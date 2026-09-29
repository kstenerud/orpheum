// SPDX-License-Identifier: GPL-3.0-or-later

package prepared

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
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
	shelf := New(directory, budget, clocktest.NewFake(now))
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

func TestEvict_RemovesTheLeastRecentlyUsedPastTheGraceFirstAndNeverOneBeingWritten(t *testing.T) {
	t.Parallel()
	disk := 50
	budget := Budget{MaxBytes: 250, Grace: 72 * time.Hour, DiskUsedLimitPercent: 95}
	shelf := shelfWith(t, budget, &disk, map[string]struct {
		bytes int
		ago   time.Duration
	}{
		"1/remux/aaaaaaaaaaaaaaaa":   {100, 10 * 24 * time.Hour}, // oldest, past its grace, but being written
		"2/remux/bbbbbbbbbbbbbbbb":   {100, 5 * 24 * time.Hour},  // past its grace
		"3/remux/cccccccccccccccc":   {100, 1 * time.Hour},       // within its grace, least recent within
		"4/h264-8m/dddddddddddddddd": {100, 10 * time.Minute},    // within its grace
	})
	done := shelf.MarkWriting("1/remux/aaaaaaaaaaaaaaaa")
	defer done()
	usage, err := shelf.Evict()
	if err != nil {
		t.Fatal(err)
	}
	// 400 bytes against 250: the one past its grace goes; the one being written stays; then the
	// least recent within its grace, as the budget still needs it.
	want := []string{"1/remux/aaaaaaaaaaaaaaaa", "4/h264-8m/dddddddddddddddd"}
	if got := remainingRunDirs(t, shelf); !slices.Equal(got, want) || usage.UsedBytes != 200 {
		t.Errorf("left %v with %d bytes, want %v with 200 (D217 §5)", got, usage.UsedBytes, want)
	}
	if _, err := os.Stat(filepath.Join(shelf.directory, "2")); !os.IsNotExist(err) {
		t.Errorf("an evicted run's empty file folder is still there: %v", err)
	}
}

func TestEvict_ADiskPastItsLimitEvictsWhateverTheBudgetSays(t *testing.T) {
	t.Parallel()
	disk := 97
	budget := Budget{MaxBytes: 1 << 30, Grace: 72 * time.Hour, DiskUsedLimitPercent: 95}
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
	if _, err := shelf.Evict(); err != nil {
		t.Fatal(err)
	}
	if got := remainingRunDirs(t, shelf); !slices.Equal(got, []string{"2/remux/bbbbbbbbbbbbbbbb"}) {
		t.Errorf("left %v, want the older one evicted for the disk, though within the budget", got)
	}
}

func TestRoom_RefusesWhenTheDiskIsPastItsLimitWithNothingToEvict(t *testing.T) {
	t.Parallel()
	disk := 99
	shelf := shelfWith(t, Budget{MaxBytes: 1 << 30, Grace: time.Hour, DiskUsedLimitPercent: 95}, &disk, nil)
	if err := shelf.MakeRoom(); !errors.Is(err, ErrNoRoom) {
		t.Errorf("MakeRoom with the disk 99%% full and nothing to evict: %v, want ErrNoRoom", err)
	}
	disk = 50
	if err := shelf.MakeRoom(); err != nil {
		t.Errorf("MakeRoom with the disk half full: %v, want room", err)
	}
}

func TestServed_MakesARenditionTheMostRecentlyUsed(t *testing.T) {
	t.Parallel()
	disk := 50
	shelf := shelfWith(t, Budget{MaxBytes: 100, Grace: time.Hour, DiskUsedLimitPercent: 95}, &disk, map[string]struct {
		bytes int
		ago   time.Duration
	}{
		"1/remux/aaaaaaaaaaaaaaaa": {100, 5 * time.Hour},
		"2/remux/bbbbbbbbbbbbbbbb": {100, 2 * time.Hour},
	})
	shelf.MarkServed("1/remux/aaaaaaaaaaaaaaaa") // played now: the other is the least recently used
	if _, err := shelf.Evict(); err != nil {
		t.Fatal(err)
	}
	if got := remainingRunDirs(t, shelf); !slices.Equal(got, []string{"1/remux/aaaaaaaaaaaaaaaa"}) {
		t.Errorf("left %v, want the one just served kept", got)
	}
}
