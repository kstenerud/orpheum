// SPDX-License-Identifier: AGPL-3.0-or-later

package prepared

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/kstenerud/orpheum/internal/platform/diskspace"
)

// The shelf knows each run's size without walking it again (audit 2026-10-25 N1): every run start
// makes room, and walking every file of a full directory took seconds there. A run is walked once,
// when the shelf first finds it; then a chunk store's size moves only as the shelf itself puts files
// in it, and its runs' raw directories, which FFmpeg writes, are measured as they are, being small
// and few.

// runSizes is what the shelf knows of its runs' sizes.
type runSizes struct {
	// walking is held while a run is walked for its size, so a file put meanwhile is not counted
	// twice: a put first makes sure its store is known (known), then adds itself.
	walking sync.Mutex

	mutex sync.Mutex
	// byRun is each run's size, in bytes, apart from its stores' open raw directories; a run
	// being written is not in it. Guarded by mutex.
	byRun map[string]int64
	// openRaws are the raw directories of the runs of chunks open in each store, by its directory;
	// guarded by mutex.
	openRaws map[string]map[string]bool
}

func newRunSizes() *runSizes {
	return &runSizes{byRun: map[string]int64{}, openRaws: map[string]map[string]bool{}}
}

// sizeOf is the run's size now, its path absolute: what it holds of the shelf's budget.
func (shelf *Shelf) sizeOf(runDir, path string) (int64, error) {
	known, err := shelf.knownSize(runDir, path)
	if err != nil {
		return 0, err
	}
	sizes := shelf.sizes
	sizes.mutex.Lock()
	raws := make([]string, 0, len(sizes.openRaws[runDir]))
	for raw := range sizes.openRaws[runDir] {
		raws = append(raws, raw)
	}
	sizes.mutex.Unlock()
	for _, raw := range raws {
		rawBytes, rawErr := diskspace.TreeBytes(raw)
		if errors.Is(rawErr, fs.ErrNotExist) {
			continue // the run closed as it was measured
		}
		if rawErr != nil {
			return 0, rawErr
		}
		known += rawBytes
	}
	return known, nil
}

// knownSize is the run's size apart from its open raw directories, walked when the shelf does not
// know it yet.
func (shelf *Shelf) knownSize(runDir, path string) (int64, error) {
	sizes := shelf.sizes
	sizes.mutex.Lock()
	known, isKnown := sizes.byRun[runDir]
	sizes.mutex.Unlock()
	if isKnown {
		return known, nil
	}
	sizes.walking.Lock()
	defer sizes.walking.Unlock()
	sizes.mutex.Lock()
	known, isKnown = sizes.byRun[runDir]
	open := make(map[string]bool, len(sizes.openRaws[runDir]))
	for raw := range sizes.openRaws[runDir] {
		open[raw] = true
	}
	sizes.mutex.Unlock()
	if isKnown {
		return known, nil
	}
	walked, err := treeBytesApart(path, open)
	if err != nil {
		return 0, err
	}
	sizes.mutex.Lock()
	sizes.byRun[runDir] = walked
	sizes.mutex.Unlock()
	return walked, nil
}

// treeBytesApart is diskspace.TreeBytes of root, the directories apart not counted.
func treeBytesApart(root string, apart map[string]bool) (int64, error) {
	if len(apart) == 0 {
		return diskspace.TreeBytes(root)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return 0, fmt.Errorf("measure %s: %w", root, err)
	}
	var total int64
	for _, entry := range entries {
		path := filepath.Join(root, entry.Name())
		if apart[path] {
			continue
		}
		bytes, err := diskspace.TreeBytes(path)
		if errors.Is(err, fs.ErrNotExist) {
			continue // removed as it was walked, as TreeBytes passes over
		}
		if err != nil {
			return 0, err
		}
		total += bytes
	}
	return total, nil
}

// grew adds delta bytes to a run the shelf knows; one it does not know is walked when it is found.
func (sizes *runSizes) grew(runDir string, delta int64) {
	sizes.mutex.Lock()
	defer sizes.mutex.Unlock()
	if known, isKnown := sizes.byRun[runDir]; isKnown {
		sizes.byRun[runDir] = known + delta
	}
}

// forget drops what the shelf knows of a run, removed or no longer to be trusted: it is walked
// again when it is found.
func (sizes *runSizes) forget(runDir string) {
	sizes.mutex.Lock()
	defer sizes.mutex.Unlock()
	delete(sizes.byRun, runDir)
}

// opened and closed track a run of chunks' raw directory in its store.
func (sizes *runSizes) opened(storeDir, rawPath string) {
	sizes.mutex.Lock()
	defer sizes.mutex.Unlock()
	if sizes.openRaws[storeDir] == nil {
		sizes.openRaws[storeDir] = map[string]bool{}
	}
	sizes.openRaws[storeDir][rawPath] = true
}

func (sizes *runSizes) closed(storeDir, rawPath string) {
	sizes.mutex.Lock()
	defer sizes.mutex.Unlock()
	delete(sizes.openRaws[storeDir], rawPath)
	if len(sizes.openRaws[storeDir]) == 0 {
		delete(sizes.openRaws, storeDir)
	}
}
