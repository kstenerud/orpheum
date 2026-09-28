// SPDX-License-Identifier: GPL-3.0-or-later

package prepared

import (
	"errors"
	"fmt"
	"io/fs"
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
	// Grace is how long after its last use a rendition is kept before one past it.
	Grace time.Duration
	// MaxDiskPercent is how full the directory's filesystem may be, whatever MaxBytes says: a
	// fixed budget cannot know what else shares the disk.
	MaxDiskPercent int
}

// Usage is what the prepared directory holds, against its budget, for the node's report.
type Usage struct {
	Bytes          int64
	BudgetBytes    int64
	DiskPercent    int
	MaxDiskPercent int
}

// ErrNoRoom is a prepare refused because the disk is past its limit and nothing can be evicted.
var ErrNoRoom = errors.New("the prepared directory's disk is past its limit, and nothing can be evicted")

// Shelf is the node's prepared directory and its budget.
type Shelf struct {
	directory string
	budget    Budget
	clock     clock.Clock
	// diskPercent says how full the filesystem at a path is; a test's stands in for statfs.
	diskPercent func(path string) (int, error)

	mutex   sync.Mutex
	writing map[string]int // guarded by mutex: run directories a prepare is writing, and how many
	// last is what the last eviction left, for the node's status; zero before the first.
	last Usage // guarded by mutex
}

// LastUsage is what the directory held after its last eviction, and whether there was one.
func (shelf *Shelf) LastUsage() (Usage, bool) {
	shelf.mutex.Lock()
	defer shelf.mutex.Unlock()
	return shelf.last, shelf.last.BudgetBytes != 0
}

// New returns the shelf over directory, an absolute path.
func New(directory string, budget Budget, clockSource clock.Clock) *Shelf {
	return &Shelf{
		directory: directory, budget: budget, clock: clockSource, diskPercent: filesystemPercent,
		writing: map[string]int{},
	}
}

// Served records that one of a rendition's files was served now: its run directory's time is
// set, which is what eviction reads as its last use. The rendition is named by its directory,
// as <file_id>/<profile>/<run_id>. A use it cannot record only makes the rendition look older.
func (shelf *Shelf) Served(renditionDir string) {
	now := shelf.clock.Now()
	path := filepath.Join(shelf.directory, filepath.FromSlash(renditionDir))
	_ = os.Chtimes(path, now, now) //nolint:errcheck // see above
}

// Writing marks a rendition being written, which eviction leaves alone, until done is called.
func (shelf *Shelf) Writing(renditionDir string) (done func()) {
	shelf.mutex.Lock()
	defer shelf.mutex.Unlock()
	shelf.writing[renditionDir]++
	return func() {
		shelf.mutex.Lock()
		defer shelf.mutex.Unlock()
		shelf.writing[renditionDir]--
		if shelf.writing[renditionDir] <= 0 {
			delete(shelf.writing, renditionDir)
		}
	}
}

// Room makes room for a prepare: it evicts what the budget and the disk need, and refuses when
// the disk is still past its limit (D217 §5).
func (shelf *Shelf) Room() error {
	usage, err := shelf.Evict()
	if err != nil {
		return err
	}
	if usage.DiskPercent > shelf.budget.MaxDiskPercent {
		return fmt.Errorf("%w (%d%% full, the limit %d%%)", ErrNoRoom, usage.DiskPercent, shelf.budget.MaxDiskPercent)
	}
	return nil
}

// run is one making of a rendition: its directory, relative, its size and its last use.
type run struct {
	dir     string
	bytes   int64
	lastUse time.Time
}

// Evict removes renditions, least recently used first, until the directory is within its budget
// and its disk within its limit: every one past its grace before any within it, and never one
// being written. It returns what is left.
func (shelf *Shelf) Evict() (Usage, error) {
	runs, err := shelf.runs()
	if err != nil {
		return Usage{}, err
	}
	var total int64
	for _, each := range runs {
		total += each.bytes
	}
	disk, err := shelf.diskPercent(shelf.directory)
	if err != nil {
		return Usage{}, err
	}
	now := shelf.clock.Now()
	pastGrace := func(each run) bool { return now.Sub(each.lastUse) >= shelf.budget.Grace }
	// Oldest first, and every one past its grace before any within it.
	slices.SortStableFunc(runs, func(one, other run) int {
		if pastGrace(one) != pastGrace(other) {
			if pastGrace(one) {
				return -1
			}
			return 1
		}
		return one.lastUse.Compare(other.lastUse)
	})
	for _, each := range runs {
		if total <= shelf.budget.MaxBytes && disk <= shelf.budget.MaxDiskPercent {
			break
		}
		if shelf.isWriting(each.dir) {
			continue
		}
		if removeErr := shelf.remove(each.dir); removeErr != nil {
			return Usage{}, removeErr
		}
		total -= each.bytes
		if disk, err = shelf.diskPercent(shelf.directory); err != nil {
			return Usage{}, err
		}
	}
	usage := Usage{
		Bytes: total, BudgetBytes: shelf.budget.MaxBytes, DiskPercent: disk,
		MaxDiskPercent: shelf.budget.MaxDiskPercent,
	}
	shelf.mutex.Lock()
	shelf.last = usage
	shelf.mutex.Unlock()
	return usage, nil
}

func (shelf *Shelf) isWriting(dir string) bool {
	shelf.mutex.Lock()
	defer shelf.mutex.Unlock()
	return shelf.writing[dir] > 0
}

// runs are the run directories three levels down, <file_id>/<profile>/<run_id>, each with its
// size and last use.
func (shelf *Shelf) runs() ([]run, error) {
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
		size, err := treeBytes(path)
		if err != nil {
			return nil, err
		}
		relative, err := filepath.Rel(shelf.directory, path)
		if err != nil {
			return nil, fmt.Errorf("name %s: %w", path, err)
		}
		runs = append(runs, run{dir: filepath.ToSlash(relative), bytes: size, lastUse: info.ModTime()})
	}
	return runs, nil
}

// remove deletes one run directory, and its profile and file folders when it leaves them empty.
func (shelf *Shelf) remove(dir string) error {
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

// treeBytes is the size of every regular file under root.
func treeBytes(root string) (int64, error) {
	var total int64
	err := filepath.WalkDir(root, func(_ string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type().IsRegular() {
			info, infoErr := entry.Info()
			if infoErr != nil {
				return infoErr
			}
			total += info.Size()
		}
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("measure %s: %w", root, err)
	}
	return total, nil
}

// filesystemPercent is how full, as a whole percent, the filesystem holding path is.
func filesystemPercent(path string) (int, error) {
	space, err := diskspace.Measure(path)
	if err != nil {
		return 0, err
	}
	return space.UsedPercent(), nil
}
