// SPDX-License-Identifier: GPL-3.0-or-later

package prepared

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/platform/diskspace"
)

// Budget is what the prepared directory may hold (D217 §5).
type Budget struct {
	// MaxBytes is the most the renditions may take together.
	MaxBytes int64
	// KeptAfterLastPlay is how long after its last play a rendition is kept before any past it is
	// evicted.
	KeptAfterLastPlay time.Duration
	// DiskUsedLimitPercent is how full the directory's filesystem may be, whatever MaxBytes says: a
	// fixed budget cannot know what else shares the disk.
	DiskUsedLimitPercent int
}

// Usage is what the prepared directory holds, against its budget, for the node's report.
type Usage struct {
	UsedBytes            int64
	BudgetBytes          int64
	DiskUsedPercent      int
	DiskUsedLimitPercent int
}

// ErrNoRoom is a prepare refused because the disk is past its limit and nothing can be evicted.
var ErrNoRoom = errors.New("the prepared directory's disk is past its limit, and nothing can be evicted")

// Shelf is the node's prepared directory and its budget.
type Shelf struct {
	directory string
	budget    Budget
	clock     clock.Clock
	// diskUsedPercent says how full the filesystem at a path is; a test's stands in for statfs.
	diskUsedPercent func(path string) (int, error)

	mutex                 sync.Mutex
	writersByRenditionDir map[string]int // guarded by mutex: run directories a prepare is writing, and how many
	// lastUsage is what the last eviction left, for the node's status; zero before the first.
	lastUsage Usage // guarded by mutex
}

// LastUsage is what the directory held after its last eviction, and whether there was one.
func (shelf *Shelf) LastUsage() (Usage, bool) {
	shelf.mutex.Lock()
	defer shelf.mutex.Unlock()
	return shelf.lastUsage, shelf.lastUsage.BudgetBytes != 0
}

// New returns the shelf over directory, an absolute path.
func New(directory string, budget Budget, clockSource clock.Clock) *Shelf {
	return &Shelf{
		directory: directory, budget: budget, clock: clockSource, diskUsedPercent: filesystemUsedPercent,
		writersByRenditionDir: map[string]int{},
	}
}

// MarkServed records that one of a rendition's files was served now: its run directory's time is
// set, which is what eviction reads as its last use. The rendition is named by its directory,
// as <file_id>/<profile>/<run_id>. A use it cannot record only makes the rendition look older.
func (shelf *Shelf) MarkServed(renditionDir string) {
	now := shelf.clock.Now()
	path := filepath.Join(shelf.directory, filepath.FromSlash(renditionDir))
	_ = os.Chtimes(path, now, now) //nolint:errcheck // see above
}

// MarkWriting marks a rendition being written, which eviction leaves alone, until finishWriting is called.
func (shelf *Shelf) MarkWriting(renditionDir string) (finishWriting func()) {
	shelf.mutex.Lock()
	defer shelf.mutex.Unlock()
	shelf.writersByRenditionDir[renditionDir]++
	return func() {
		shelf.mutex.Lock()
		defer shelf.mutex.Unlock()
		shelf.writersByRenditionDir[renditionDir]--
		if shelf.writersByRenditionDir[renditionDir] <= 0 {
			delete(shelf.writersByRenditionDir, renditionDir)
		}
	}
}

// MakeRoom makes room for a prepare: it evicts what the budget and the disk need, and refuses when
// the disk is still past its limit (D217 §5).
func (shelf *Shelf) MakeRoom() error {
	usage, err := shelf.Evict()
	if err != nil {
		return err
	}
	if usage.DiskUsedPercent > shelf.budget.DiskUsedLimitPercent {
		return fmt.Errorf("%w (%d%% full, the limit %d%%)", ErrNoRoom, usage.DiskUsedPercent,
			shelf.budget.DiskUsedLimitPercent)
	}
	return nil
}

// run is one making of a rendition: its directory, relative, its size and its last use.
type run struct {
	renditionDir string
	sizeBytes    int64
	lastUse      time.Time
}

// Evict removes renditions, least recently used first, until the directory is within its budget
// and its disk within its limit: every one past its kept time before any within it, and never one
// being written. It returns what is left.
func (shelf *Shelf) Evict() (Usage, error) {
	runs, err := shelf.listRuns()
	if err != nil {
		return Usage{}, err
	}
	var total int64
	for _, each := range runs {
		total += each.sizeBytes
	}
	usedPercent, err := shelf.diskUsedPercent(shelf.directory)
	if err != nil {
		return Usage{}, err
	}
	now := shelf.clock.Now()
	isPastKeptTime := func(each run) bool { return now.Sub(each.lastUse) >= shelf.budget.KeptAfterLastPlay }
	// Oldest first, and every one past its kept time before any within it.
	slices.SortStableFunc(runs, func(one, other run) int {
		if isPastKeptTime(one) != isPastKeptTime(other) {
			if isPastKeptTime(one) {
				return -1
			}
			return 1
		}
		return one.lastUse.Compare(other.lastUse)
	})
	for _, each := range runs {
		if total <= shelf.budget.MaxBytes && usedPercent <= shelf.budget.DiskUsedLimitPercent {
			break
		}
		if shelf.isWriting(each.renditionDir) {
			continue
		}
		if removeErr := shelf.removeRun(each.renditionDir); removeErr != nil {
			return Usage{}, removeErr
		}
		total -= each.sizeBytes
		if usedPercent, err = shelf.diskUsedPercent(shelf.directory); err != nil {
			return Usage{}, err
		}
	}
	usage := Usage{
		UsedBytes: total, BudgetBytes: shelf.budget.MaxBytes, DiskUsedPercent: usedPercent,
		DiskUsedLimitPercent: shelf.budget.DiskUsedLimitPercent,
	}
	shelf.mutex.Lock()
	shelf.lastUsage = usage
	shelf.mutex.Unlock()
	return usage, nil
}

func (shelf *Shelf) isWriting(dir string) bool {
	shelf.mutex.Lock()
	defer shelf.mutex.Unlock()
	return shelf.writersByRenditionDir[dir] > 0
}

// listRuns are the run directories three levels down, <file_id>/<profile>/<run_id>, each with its
// size and last use.
func (shelf *Shelf) listRuns() ([]run, error) {
	matches, err := filepath.Glob(filepath.Join(shelf.directory, "*", "*", "*"))
	if err != nil {
		return nil, fmt.Errorf("list the prepared renditions: %w", err)
	}
	runs := make([]run, 0, len(matches))
	for _, path := range matches {
		info, statErr := os.Stat(path)
		if statErr != nil || !info.IsDir() {
			continue // a run gone meanwhile, or a stray file, is not a rendition
		}
		sizeBytes, err := diskspace.TreeBytes(path)
		if err != nil {
			return nil, err
		}
		relative, err := filepath.Rel(shelf.directory, path)
		if err != nil {
			return nil, fmt.Errorf("name %s: %w", path, err)
		}
		runs = append(runs,
			run{renditionDir: filepath.ToSlash(relative), sizeBytes: sizeBytes, lastUse: info.ModTime()})
	}
	return runs, nil
}

// removeRun deletes one run directory, and its profile and file folders when it leaves them empty.
func (shelf *Shelf) removeRun(dir string) error {
	path := filepath.Join(shelf.directory, filepath.FromSlash(dir))
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("evict %s: %w", dir, err)
	}
	shelf.removeEmptyFolders(filepath.Dir(path))
	return nil
}

// removeEmptyFolders removes folder and the folders above it, up to the shelf's own, while each is
// empty; the first that is not, because another run or profile is still there, stops it.
func (shelf *Shelf) removeEmptyFolders(folder string) {
	for ; folder != shelf.directory && folder != "."; folder = filepath.Dir(folder) {
		if os.Remove(folder) != nil {
			return
		}
	}
}

// filesystemUsedPercent is how full, as a whole percent, the filesystem holding path is.
func filesystemUsedPercent(path string) (int, error) {
	space, err := diskspace.Measure(path)
	if err != nil {
		return 0, err
	}
	return space.UsedPercent(), nil
}
