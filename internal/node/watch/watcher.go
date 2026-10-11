// SPDX-License-Identifier: AGPL-3.0-or-later

package watch

import (
	"context"
	"log/slog"
	"path"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock"
)

// wholeRoot is a notice's folder for the whole root.
const wholeRoot = "."

// maxHoldQuietSpells is how many quiet spells a root's changes are held at most while events keep
// coming: a copy that never pauses is still noticed, its scan reading what is there so far, and a
// later notice reads the rest.
const maxHoldQuietSpells = 20

// changesBuffer is how many notices wait for the agent while it has no session with the core: a
// notice that finds it full is dropped, which the root's periodic scan covers.
const changesBuffer = 64

// Options are how the watcher holds what it sees.
type Options struct {
	// Quiet is how long a root must go without a change before its changes are noticed:
	// node.watch_quiet_seconds.
	Quiet time.Duration
	// NetworkRoots are the roots on a network filesystem, which are not watched (D27).
	NetworkRoots []string
}

// Watcher watches the node's roots for changes and notices, per root, the folder holding every change
// once the root has gone quiet (D272 §9). It never scans: the core decides.
type Watcher struct {
	roots      []string
	mediaRoots map[string]*confine.MediaRoot
	options    Options
	clock      clock.Clock
	logger     *slog.Logger
	changes    chan nodeproto.Changed
	seen       chan seenChange

	mutex sync.Mutex
	// states are each root's watch, as the kernel layer last set it. Guarded by mutex.
	states map[string]nodeproto.RootWatch
}

// seenChange is a change the kernel layer saw: under folder, relative to root, or the whole root.
type seenChange struct {
	root   string
	folder string
}

// New returns a watcher of the node's roots.
func New(roots []*confine.MediaRoot, options Options, clockSource clock.Clock, logger *slog.Logger) *Watcher {
	states := make(map[string]nodeproto.RootWatch, len(roots))
	paths := make([]string, 0, len(roots))
	byPath := make(map[string]*confine.MediaRoot, len(roots))
	for _, root := range roots {
		paths = append(paths, root.Path())
		byPath[root.Path()] = root
		states[root.Path()] = nodeproto.RootWatch{
			RootPath: root.Path(), Cause: nodeproto.WatchCauseUnavailable,
			Reason: "the watcher has not started",
		}
	}
	return &Watcher{
		roots: paths, mediaRoots: byPath, options: options, clock: clockSource, logger: logger,
		changes: make(chan nodeproto.Changed, changesBuffer), seen: make(chan seenChange, changesBuffer),
		states: states,
	}
}

// Changes are the watcher's notices, for the agent to post (nodeproto.PathChanged).
func (watcher *Watcher) Changes() <-chan nodeproto.Changed { return watcher.changes }

// States are each root's watch, for the node's status.
func (watcher *Watcher) States() []nodeproto.RootWatch {
	watcher.mutex.Lock()
	defer watcher.mutex.Unlock()
	states := make([]nodeproto.RootWatch, 0, len(watcher.roots))
	for _, root := range watcher.roots {
		states = append(states, watcher.states[root])
	}
	return states
}

// setState records a root's watch, and logs a change of it.
func (watcher *Watcher) setState(ctx context.Context, state nodeproto.RootWatch) {
	watcher.mutex.Lock()
	before := watcher.states[state.RootPath]
	watcher.states[state.RootPath] = state
	watcher.mutex.Unlock()
	if before.IsWatching == state.IsWatching && before.Cause == state.Cause {
		return
	}
	if state.IsWatching {
		watcher.logger.InfoContext(ctx, "watching a root for changes", "rootPath", state.RootPath)
		return
	}
	watcher.logger.WarnContext(ctx, "not watching a root for changes; its periodic scan finds them",
		"rootPath", state.RootPath, "cause", string(state.Cause), "reason", state.Reason)
}

// Run watches until ctx ends: the kernel layer reports what it sees, and the changes of each root
// are held until it goes quiet.
func (watcher *Watcher) Run(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var kernel sync.WaitGroup
	kernel.Go(func() { watchKernel(ctx, watcher) })
	watcher.hold(ctx)
	cancel()
	kernel.Wait()
}

// saw is the kernel layer's report of a change under folder, a path relative to root.
func (watcher *Watcher) saw(ctx context.Context, root, folder string) {
	select {
	case watcher.seen <- seenChange{root: root, folder: folder}:
	case <-ctx.Done():
	}
}

// pendingRoot is a root's changes not yet noticed.
type pendingRoot struct {
	folder  string
	firstAt time.Time
	lastAt  time.Time
}

// hold folds each root's changes into the folder holding them all, and notices it once the root has
// gone Quiet, or has been held maxHoldQuietSpells of it while changes kept coming.
func (watcher *Watcher) hold(ctx context.Context) {
	pending := map[string]*pendingRoot{}
	timer := watcher.clock.NewTimer(time.Hour)
	timer.Stop()
	for {
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case change := <-watcher.seen:
			watcher.fold(pending, change)
		case <-timer.C():
		}
		watcher.noticeDue(ctx, pending)
		if wait, isAny := watcher.nextWait(pending); isAny {
			timer.Reset(wait)
		}
	}
}

// fold holds a change with its root's others, as the folder holding them all.
func (watcher *Watcher) fold(pending map[string]*pendingRoot, change seenChange) {
	now := watcher.clock.Now()
	held, isHeld := pending[change.root]
	if !isHeld {
		pending[change.root] = &pendingRoot{folder: cleanFolder(change.folder), firstAt: now, lastAt: now}
		return
	}
	held.folder, held.lastAt = widerFolder(held.folder, change.folder), now
}

// noticeDue notices every held root whose changes are due, in the order of the roots.
func (watcher *Watcher) noticeDue(ctx context.Context, pending map[string]*pendingRoot) {
	now := watcher.clock.Now()
	for _, root := range watcher.roots {
		held, isHeld := pending[root]
		if !isHeld || now.Before(watcher.dueAt(held)) {
			continue
		}
		delete(pending, root)
		select {
		case watcher.changes <- nodeproto.Changed{RootPath: root, Path: held.folder}:
		default:
			watcher.logger.WarnContext(ctx, "dropped a change the core has not taken; the root's periodic scan "+
				"finds it", "rootPath", root, "path", held.folder)
		}
	}
}

// dueAt is when a held root's changes are noticed.
func (watcher *Watcher) dueAt(held *pendingRoot) time.Time {
	quiet, longest := held.lastAt.Add(watcher.options.Quiet), held.firstAt.Add(maxHoldQuietSpells*watcher.options.Quiet)
	if longest.Before(quiet) {
		return longest
	}
	return quiet
}

// nextWait is the wait until the next held root is due, and whether any is held.
func (watcher *Watcher) nextWait(pending map[string]*pendingRoot) (time.Duration, bool) {
	var next time.Time
	for _, held := range pending {
		if due := watcher.dueAt(held); next.IsZero() || due.Before(next) {
			next = due
		}
	}
	return next.Sub(watcher.clock.Now()), !next.IsZero()
}

// cleanFolder is a folder as a notice names it: clean, relative to its root, and UTF-8 throughout,
// since the wire carries text: a folder whose name is not is noticed by the nearest one above it.
func cleanFolder(folder string) string {
	folder = path.Clean(folder)
	if folder == "" || path.IsAbs(folder) || folder == ".." || strings.HasPrefix(folder, "../") {
		return wholeRoot
	}
	for folder != wholeRoot && !utf8.ValidString(folder) {
		folder = path.Dir(folder)
	}
	return folder
}

// widerFolder is the folder holding both.
func widerFolder(first, second string) string {
	first, second = cleanFolder(first), cleanFolder(second)
	for first != wholeRoot && second != first && !strings.HasPrefix(second, first+"/") {
		first = path.Dir(first)
	}
	return first
}
