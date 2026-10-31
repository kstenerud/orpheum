// SPDX-License-Identifier: AGPL-3.0-or-later

package prepared

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/platform/diskspace"
)

// Budget is what the prepared directory may hold (D217 §5).
type Budget struct {
	// MaxBytes is the most the runs may take together.
	MaxBytes int64
	// KeptAfterLastPlay is how long after its last play a run is kept before any past it is
	// evicted.
	KeptAfterLastPlay time.Duration
	// WatchedWithin is how recently a run served a file for it to count as being watched,
	// which eviction never removes (node.prepared_watched_minutes; the owner, 2026-10-06).
	WatchedWithin time.Duration
	// DiskUsedLimitPercent is how full the directory's filesystem may be, whatever MaxBytes says: a
	// fixed budget cannot know what else shares the disk.
	DiskUsedLimitPercent int
	// MinFreeBytes is the least the directory's filesystem keeps free, whatever the rest says: on a
	// large disk a percentage leaves too little, on a small one too much (node.prepared_disk_min_free_mib).
	MinFreeBytes int64
}

// Usage is what the prepared directory holds, against its budget, for the node's report.
type Usage struct {
	UsedBytes            int64
	BudgetBytes          int64
	DiskUsedPercent      int
	DiskUsedLimitPercent int
	DiskAvailableBytes   int64
	DiskMinFreeBytes     int64
}

// isDiskWithin is whether the directory's filesystem is within its limit and keeps its floor free.
func (usage Usage) isDiskWithin() bool {
	return usage.DiskUsedPercent <= usage.DiskUsedLimitPercent && usage.DiskAvailableBytes >= usage.DiskMinFreeBytes
}

// ErrNoRoom is a run refused because the disk is past its limit, or below its floor of free
// space, and nothing can be evicted.
var ErrNoRoom = errors.New("the prepared directory's disk is past its limit, and nothing can be evicted")

// Shelf is the node's prepared directory and its budget.
type Shelf struct {
	directory string
	budget    Budget
	clock     clock.Clock
	// logger hears every eviction: what went, and why (the owner, 2026-10-06).
	logger *slog.Logger
	// diskUsedPercent says how full the filesystem at a path is, and diskAvailableBytes how much of
	// it is free; a test's stand in for statfs.
	diskUsedPercent    func(path string) (int, error)
	diskAvailableBytes func(path string) (int64, error)

	mutex           sync.Mutex
	writersByRunDir map[string]int // guarded by mutex: run directories a run of chunks is writing, and how many
	// lastUsage is what the last eviction left, for the node's status; zero before the first.
	lastUsage Usage // guarded by mutex
	// sizes are the runs' sizes, as the shelf knows them without walking them again.
	sizes *runSizes
	// storeTiers are how strongly the core's players want each chunk store kept, by its directory
	// (D309 §8); nil until the core first said, when the stores are evicted as the runs are.
	storeTiers map[string]nodeproto.KeepTier // guarded by mutex
	// locks are the stores' locks: a file's put and its check against its record hold its store's
	// (storeLock, D392 §4).
	locks storeLocks
}

// SetStoreTiers keeps how strongly the core's players want each chunk store kept, by its directory,
// for the next eviction (D309 §8): a store it does not name is the first removed.
func (shelf *Shelf) SetStoreTiers(tiers map[string]nodeproto.KeepTier) {
	shelf.mutex.Lock()
	defer shelf.mutex.Unlock()
	shelf.storeTiers = tiers
}

// LastUsage is what the directory held after its last eviction, and whether there was one.
func (shelf *Shelf) LastUsage() (Usage, bool) {
	shelf.mutex.Lock()
	defer shelf.mutex.Unlock()
	return shelf.lastUsage, shelf.lastUsage.BudgetBytes != 0
}

// New returns the shelf over directory, an absolute path.
func New(directory string, budget Budget, clockSource clock.Clock, logger *slog.Logger) *Shelf {
	return &Shelf{
		directory: directory, budget: budget, clock: clockSource, logger: logger,
		diskUsedPercent:    filesystemUsedPercent,
		diskAvailableBytes: filesystemAvailableBytes,
		writersByRunDir:    map[string]int{}, sizes: newRunSizes(),
	}
}

// MarkServed records that one of a run's files was served now: its directory's time is
// set, which is what eviction reads as its last use. The run is named by its directory,
// as <file_id>/chunks-<stream>/<grid_id>. A use it cannot record only makes the run look older.
func (shelf *Shelf) MarkServed(runDir string) {
	now := shelf.clock.Now()
	path := filepath.Join(shelf.directory, filepath.FromSlash(runDir))
	_ = os.Chtimes(path, now, now) //nolint:errcheck // see above
}

// MarkWriting marks a run being written, which eviction leaves alone, until finishWriting is called.
func (shelf *Shelf) MarkWriting(runDir string) (finishWriting func()) {
	shelf.mutex.Lock()
	defer shelf.mutex.Unlock()
	shelf.writersByRunDir[runDir]++
	return func() {
		shelf.mutex.Lock()
		defer shelf.mutex.Unlock()
		shelf.writersByRunDir[runDir]--
		if shelf.writersByRunDir[runDir] <= 0 {
			delete(shelf.writersByRunDir, runDir)
		}
	}
}

// MakeRoom makes room for a run of chunks: it evicts what the budget and the disk need, and refuses when
// the disk is still past its limit, or below its floor of free space (D217 §5).
func (shelf *Shelf) MakeRoom(ctx context.Context) error {
	usage, err := shelf.Evict(ctx)
	if err != nil {
		return err
	}
	if !usage.isDiskWithin() {
		return fmt.Errorf("%w (%d%% full, the limit %d%%; %d MiB free, the floor %d MiB)", ErrNoRoom,
			usage.DiskUsedPercent, usage.DiskUsedLimitPercent, usage.DiskAvailableBytes>>20, usage.DiskMinFreeBytes>>20)
	}
	return nil
}

// run is one run directory, a store of chunks: its directory, relative, its size and its last use.
type run struct {
	runDir    string
	sizeBytes int64
	lastUse   time.Time
}

// Evict removes runs, least recently used first, until the directory is within its budget
// and its disk within its limit: every one past its kept time before any within it, and never one
// being written or being watched (served within WatchedWithin). It returns what is left; a disk
// still past its limit then refuses the next run (MakeRoom), rather than break a film playing.
func (shelf *Shelf) Evict(ctx context.Context) (Usage, error) {
	runs, err := shelf.listRuns()
	if err != nil {
		return Usage{}, err
	}
	var total int64
	for _, each := range runs {
		total += each.sizeBytes
	}
	usage, err := shelf.measure(total)
	if err != nil {
		return Usage{}, err
	}
	now := shelf.clock.Now()
	isPastKeptTime := func(each run) bool { return now.Sub(each.lastUse) >= shelf.budget.KeptAfterLastPlay }
	isWatched := func(each run) bool { return now.Sub(each.lastUse) < shelf.budget.WatchedWithin }
	var spared []string
	shelf.mutex.Lock()
	tiers := shelf.storeTiers
	shelf.mutex.Unlock()
	sortForEviction(runs, func(each run) int { return evictionRank(each, tiers, isPastKeptTime(each)) })
	for _, each := range runs {
		if usage.UsedBytes <= shelf.budget.MaxBytes && usage.isDiskWithin() {
			break
		}
		if shelf.isWriting(each.runDir) {
			continue
		}
		if isWatched(each) {
			spared = append(spared, each.runDir)
			continue
		}
		if removeErr := shelf.removeRun(each.runDir); removeErr != nil {
			return Usage{}, removeErr
		}
		shelf.logEviction(ctx, each, usage, isPastKeptTime(each))
		total -= each.sizeBytes
		if usage, err = shelf.measure(total); err != nil {
			return Usage{}, err
		}
	}
	shelf.logKeptWatched(ctx, spared, usage)
	shelf.mutex.Lock()
	shelf.lastUsage = usage
	shelf.mutex.Unlock()
	return usage, nil
}

// sortForEviction orders runs as eviction takes them: the lowest rank first (evictionRank), and the
// least lately used within one.
func sortForEviction(runs []run, rankOf func(run) int) {
	slices.SortStableFunc(runs, func(one, other run) int {
		if rankOf(one) != rankOf(other) {
			return rankOf(one) - rankOf(other)
		}
		return one.lastUse.Compare(other.lastUse)
	})
}

// evictionRank is where a run stands in eviction's order, the lowest taken first: a chunk store by
// its players' tier, one no player gives a tier before any run and one in use after every one
// (D309 §8); a run, and every chunk store while the core has said nothing of them, past its
// kept time before any within it.
func evictionRank(each run, tiers map[string]nodeproto.KeepTier, isPastKeptTime bool) int {
	_, store, _ := strings.Cut(each.runDir, "/")
	if tiers != nil && strings.HasPrefix(store, "chunks-") {
		switch tiers[each.runDir] {
		case nodeproto.KeepMaybe:
			return 2
		case nodeproto.KeepLikely:
			return 3
		case nodeproto.KeepInUse:
			return 4
		case nodeproto.KeepUnlikely:
			return 0
		}
		return 0
	}
	if isPastKeptTime {
		return 1
	}
	return 2
}

// logKeptWatched says, once per eviction, which runs being watched were kept though the
// directory or its disk still needs room, which the next run's refusal follows from.
func (shelf *Shelf) logKeptWatched(ctx context.Context, spared []string, usage Usage) {
	if len(spared) == 0 || (usage.UsedBytes <= shelf.budget.MaxBytes && usage.isDiskWithin()) {
		return
	}
	shelf.logger.WarnContext(ctx, "kept prepared runs being watched though the directory or its disk needs room",
		"watched", spared, "usedMiB", usage.UsedBytes>>20, "budgetMiB", usage.BudgetBytes>>20,
		"diskUsedPercent", usage.DiskUsedPercent, "diskUsedLimitPercent", usage.DiskUsedLimitPercent,
		"diskFreeMiB", usage.DiskAvailableBytes>>20, "diskMinFreeMiB", usage.DiskMinFreeBytes>>20)
}

// logEviction says what an eviction removed and why, with the numbers that made it: a run
// removed while still within its kept time is a warning, since someone may be about to play it, or
// be playing it. A disk filling up was found only by reading the code (staging, 2026-10-06).
func (shelf *Shelf) logEviction(ctx context.Context, each run, before Usage, wasPastKeptTime bool) {
	why := "the prepared directory is over its budget"
	if !before.isDiskWithin() {
		why = "its disk is past its limit, or below its floor of free space"
	}
	attributes := []any{
		"why", why, "runDir", each.runDir, "sizeMiB", each.sizeBytes >> 20,
		"lastUsed", each.lastUse.UTC().Format(time.RFC3339),
		"usedMiB", before.UsedBytes >> 20, "budgetMiB", before.BudgetBytes >> 20,
		"diskUsedPercent", before.DiskUsedPercent, "diskUsedLimitPercent", before.DiskUsedLimitPercent,
		"diskFreeMiB", before.DiskAvailableBytes >> 20, "diskMinFreeMiB", before.DiskMinFreeBytes >> 20,
	}
	if wasPastKeptTime {
		shelf.logger.InfoContext(ctx, "evicted a prepared run past its kept time", attributes...)
		return
	}
	shelf.logger.WarnContext(ctx, "evicted a prepared run still within its kept time", attributes...)
}

// measure is what the directory holds, total, against its budget, and its filesystem now.
func (shelf *Shelf) measure(total int64) (Usage, error) {
	usedPercent, err := shelf.diskUsedPercent(shelf.directory)
	if err != nil {
		return Usage{}, err
	}
	availableBytes, err := shelf.diskAvailableBytes(shelf.directory)
	if err != nil {
		return Usage{}, err
	}
	return Usage{
		UsedBytes: total, BudgetBytes: shelf.budget.MaxBytes, DiskUsedPercent: usedPercent,
		DiskUsedLimitPercent: shelf.budget.DiskUsedLimitPercent, DiskAvailableBytes: availableBytes,
		DiskMinFreeBytes: shelf.budget.MinFreeBytes,
	}, nil
}

func (shelf *Shelf) isWriting(dir string) bool {
	shelf.mutex.Lock()
	defer shelf.mutex.Unlock()
	return shelf.writersByRunDir[dir] > 0
}

// listRuns are the run directories three levels down, <file_id>/<kind>/<id>, each with its
// size, as the shelf knows it (sizeOf), and last use.
func (shelf *Shelf) listRuns() ([]run, error) {
	matches, err := filepath.Glob(filepath.Join(shelf.directory, "*", "*", "*"))
	if err != nil {
		return nil, fmt.Errorf("list the prepared runs: %w", err)
	}
	runs := make([]run, 0, len(matches))
	for _, path := range matches {
		info, statErr := os.Stat(path)
		if statErr != nil || !info.IsDir() {
			continue // a run gone meanwhile, or a stray file, is not a run
		}
		relative, err := filepath.Rel(shelf.directory, path)
		if err != nil {
			return nil, fmt.Errorf("name %s: %w", path, err)
		}
		runDir := filepath.ToSlash(relative)
		sizeBytes, err := shelf.sizeOf(runDir, path)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run{runDir: runDir, sizeBytes: sizeBytes, lastUse: info.ModTime()})
	}
	return runs, nil
}

// removeRun deletes one run directory, and its kind and file folders when it leaves them empty.
func (shelf *Shelf) removeRun(dir string) error {
	path := filepath.Join(shelf.directory, filepath.FromSlash(dir))
	defer shelf.sizes.forget(dir)
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("evict %s: %w", dir, err)
	}
	shelf.removeEmptyFolders(filepath.Dir(path))
	return nil
}

// removeEmptyFolders removes folder and the folders above it, up to the shelf's own, while each is
// empty; the first that is not, because another run or kind is still there, stops it.
func (shelf *Shelf) removeEmptyFolders(folder string) {
	for ; folder != shelf.directory && folder != "."; folder = filepath.Dir(folder) {
		if os.Remove(folder) != nil {
			return
		}
	}
}

// filesystemAvailableBytes is how much of the filesystem holding path is free to this process.
func filesystemAvailableBytes(path string) (int64, error) {
	space, err := diskspace.Measure(path)
	if err != nil {
		return 0, err
	}
	return space.AvailableBytes, nil
}

// filesystemUsedPercent is how full, as a whole percent, the filesystem holding path is.
func filesystemUsedPercent(path string) (int, error) {
	space, err := diskspace.Measure(path)
	if err != nil {
		return 0, err
	}
	return space.UsedPercent(), nil
}
