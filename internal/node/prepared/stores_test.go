// SPDX-License-Identifier: AGPL-3.0-or-later

package prepared

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/nodeproto"
)

const testStore = "7/chunks-q1200k480p/0123456789abcdef"

// openTestStore opens testStore, a quality's, on a fresh shelf, returning the shelf and the run's
// raw directory.
func openTestStore(t *testing.T) (*Shelf, string, nodeproto.StoreManifest) {
	t.Helper()
	disk := 50
	shelf := shelfWith(t, Budget{MaxBytes: 1 << 30, DiskUsedLimitPercent: 95}, &disk, nil)
	manifest := nodeproto.StoreManifest{
		StoreDir: testStore,
		Stream:   nodeproto.ChunkStream{Kind: nodeproto.StreamQuality, BitRateKbps: 1200, Height: 480},
	}
	rawPath, err := shelf.OpenStore(manifest)
	if err != nil {
		t.Fatal(err)
	}
	return shelf, rawPath, manifest
}

// A store is made with its manifest, a raw directory for each run, and a chunk put in it whole
// (D247 §10).
func TestStore_IsMadeWithItsManifestAndTakesAChunkWhole(t *testing.T) {
	t.Parallel()
	shelf, rawPath, manifest := openTestStore(t)
	if kept, found := shelf.readStoreManifest(testStore); !found || kept.Stream != manifest.Stream {
		t.Errorf("the store's manifest is %+v, %v; want the one it was opened with", kept, found)
	}
	other, err := shelf.OpenStore(manifest)
	if err != nil || other == rawPath {
		t.Errorf("a second run's raw directory is %q, %v; want one of its own", other, err)
	}
	if shelf.HasFile(testStore, "00003.m4s") {
		t.Error("an empty store holds chunk 3")
	}
	if err := shelf.PutFile(testStore, "00003.m4s", []byte("chunk")); err != nil {
		t.Fatal(err)
	}
	if !shelf.HasFile(testStore, "00003.m4s") || shelf.HasFile(testStore, "00003.m4s.part") {
		t.Error("a put chunk is not in the store whole, under its own name alone")
	}
}

// A run's raw files are discarded, and its raw directory closed, inside the store alone, never the
// store's own files (D247 §10).
func TestStore_DiscardsOnlyItsRunsRawFiles(t *testing.T) {
	t.Parallel()
	shelf, rawPath, _ := openTestStore(t)
	if err := shelf.PutFile(testStore, "00003.m4s", []byte("chunk")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rawPath, "00003.mp4"), []byte("raw"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := shelf.DiscardRaw(rawPath, "00003.mp4"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(rawPath, "00003.mp4")); !os.IsNotExist(err) {
		t.Errorf("a discarded raw file is still there: %v", err)
	}
	if err := shelf.DiscardRaw(filepath.Join(shelf.directory, testStore), nodeproto.StoreManifestName); err == nil {
		t.Error("the store's own manifest was discarded as a raw file")
	}
	if err := shelf.DiscardRaw(rawPath, "../00003.m4s"); err == nil || !shelf.HasFile(testStore, "00003.m4s") {
		t.Errorf("a name climbing out of the raw directory was discarded: %v", err)
	}
	if err := shelf.CloseRun(rawPath); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(rawPath); !os.IsNotExist(err) || !shelf.HasFile(testStore, "00003.m4s") {
		t.Errorf("closing the run left its raw directory (%v), or took the chunks", err)
	}
}

// The node keeps its stores through the core's reconciling, which knows renditions alone, but a
// store directory with no manifest is a making that never finished (D238 §2).
func TestReconcile_KeepsAStoreWithItsManifestAndEvictsOneWithout(t *testing.T) {
	t.Parallel()
	disk := 50
	shelf := shelfWith(t, Budget{MaxBytes: 1 << 30, DiskUsedLimitPercent: 95}, &disk, map[string]struct {
		bytes int
		ago   time.Duration
	}{
		"8/chunks-picture/fedcba9876543210": {100, time.Minute}, // never opened as a store
	})
	if _, err := shelf.OpenStore(nodeproto.StoreManifest{
		StoreDir: testStore, Stream: nodeproto.ChunkStream{Kind: nodeproto.StreamPicture},
	}); err != nil {
		t.Fatal(err)
	}
	evicted, err := shelf.Reconcile(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(evicted, []string{"8/chunks-picture/fedcba9876543210"}) ||
		!slices.Equal(remainingRunDirs(t, shelf), []string{testStore}) {
		t.Errorf("reconciling evicted %v, leaving %v; want only the store without a manifest gone",
			evicted, remainingRunDirs(t, shelf))
	}
	if manifests, err := shelf.Inventory(); err != nil || len(manifests) != 0 {
		t.Errorf("the inventory lists %v, %v; want no store among the renditions", manifests, err)
	}
}
