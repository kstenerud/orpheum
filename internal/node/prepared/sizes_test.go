// SPDX-License-Identifier: AGPL-3.0-or-later

package prepared

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// usedBytes is what the shelf says its runs hold, as an eviction with room to spare finds it.
func usedBytes(t *testing.T, shelf *Shelf) int64 {
	t.Helper()
	usage, err := shelf.Evict(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return usage.UsedBytes
}

// testRun is the run the size tests write.
const testRun = "1/remux/aaaaaaaaaaaaaaaa"

// writeBehind writes a file into testRun behind the shelf's back, the run last used two
// hours before the shelf's now.
func writeBehind(t *testing.T, shelf *Shelf, name string, size int) {
	t.Helper()
	path := filepath.Join(shelf.directory, filepath.FromSlash(testRun))
	if err := os.WriteFile(filepath.Join(path, name), make([]byte, size), 0o600); err != nil {
		t.Fatal(err)
	}
	used := now.Add(-2 * time.Hour)
	if err := os.Chtimes(path, used, used); err != nil {
		t.Fatal(err)
	}
}

// A run found once is not walked again: every run start makes room, and walking each file of a full
// directory took seconds there (audit 2026-10-25 N1). A file written behind the shelf's back is not
// seen, which is how the test knows it was not walked; one evicted is forgotten, and found afresh.
func TestEvict_WalksARunOnceAndForgetsItWhenEvicted(t *testing.T) {
	t.Parallel()
	disk := 50
	shelf := shelfWith(t, Budget{MaxBytes: 1 << 30, KeptAfterLastPlay: time.Hour, DiskUsedLimitPercent: 95}, &disk,
		map[string]struct {
			bytes int
			ago   time.Duration
		}{testRun: {100, 2 * time.Hour}})
	if used := usedBytes(t, shelf); used != 100 {
		t.Fatalf("the shelf holds %d bytes; want the run's 100", used)
	}
	writeBehind(t, shelf, "behind.m4s", 50)
	if used := usedBytes(t, shelf); used != 100 {
		t.Errorf("the second eviction found %d bytes; want the 100 known, the run not walked again", used)
	}
	shelf.budget.MaxBytes = 10
	if used := usedBytes(t, shelf); used != 0 {
		t.Fatalf("over its budget, the shelf kept %d bytes; want the run evicted", used)
	}
	shelf.budget.MaxBytes = 1 << 30
	if err := os.MkdirAll(filepath.Join(shelf.directory, filepath.FromSlash(testRun)), 0o700); err != nil {
		t.Fatal(err)
	}
	writeBehind(t, shelf, "again.m4s", 30)
	if used := usedBytes(t, shelf); used != 30 {
		t.Errorf("a run made again where one was evicted holds %d bytes; want its own 30, walked afresh", used)
	}
}

// A store's size moves as the shelf puts files in it, a replaced file apart, and its open runs' raw
// directories, which FFmpeg writes, count as they are until each run closes.
func TestStore_ItsSizeFollowsWhatIsPutInItAndItsOpenRunsRawFiles(t *testing.T) {
	t.Parallel()
	shelf, rawPath, _ := openTestStore(t)
	finishWriting := shelf.MarkWriting(testStore)
	defer finishWriting()
	before := usedBytes(t, shelf) // the manifest
	if err := shelf.PutFile(testStore, "00003.m4s", make([]byte, 1000)); err != nil {
		t.Fatal(err)
	}
	if err := shelf.PutFile(testStore, "00003.m4s", make([]byte, 400)); err != nil { // made again, smaller
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rawPath, "raw00004.m4s"), make([]byte, 70), 0o600); err != nil {
		t.Fatal(err)
	}
	// The chunk as last put comes with its record (D392 §4).
	chunkBytes := int64(400 + len(recordOf(make([]byte, 400)).encoded()))
	if used := usedBytes(t, shelf); used != before+chunkBytes+70 {
		t.Errorf("the store holds %d bytes; want %d: the manifest, the chunk as last put with its record, and the "+
			"raw file", used, before+chunkBytes+70)
	}
	if err := shelf.CloseRun(rawPath); err != nil {
		t.Fatal(err)
	}
	if used := usedBytes(t, shelf); used != before+chunkBytes {
		t.Errorf("once its run closed, the store holds %d bytes; want %d, the raw file gone", used, before+chunkBytes)
	}
}
