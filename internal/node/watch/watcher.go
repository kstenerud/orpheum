// SPDX-License-Identifier: AGPL-3.0-or-later

package watch

import (
	"context"
	"log/slog"
	"path"
	"slices"
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
	// Quiet is how long a folder must go without a change before its changes are noticed:
	// node.watch_quiet_seconds.
	Quiet time.Duration
	// IsOnNetworkFilesystem says whether a folder is on a network filesystem, which is not watched
	// (D27); nil says none is.
	IsOnNetworkFilesystem func(path string) bool
}

// Watcher watches the library folders the core gives it (nodeproto.HelloReply.WatchFolders), each a
// configured root or a folder inside one, and notices, per folder, the folder holding every change once
// it has gone quiet (D272 §9). It watches nothing else: a configured root's folders that no library
// uses are never walked. It never scans: the core decides.
type Watcher struct {
	// configured are the node's configured roots, by path: a folder none holds is refused (D107).
	configured map[string]*confine.MediaRoot
	options    Options
	clock      clock.Clock
	logger     *slog.Logger
	changes    chan nodeproto.Changed
	seen       chan seenChange
	// wanted carries the folders to watch, the latest set only, to the kernel layer.
	wanted chan []string

	mutex sync.Mutex
	// folders are the folders to watch, in byte order; nil until Watch first names them. Guarded by
	// mutex.
	folders []string
	// states are each folder's watch, as the kernel layer last set it. Guarded by mutex.
	states map[string]nodeproto.RootWatch
}

// seenChange is a change the kernel layer saw: under folder, relative to root, a watched library
// folder, or the whole of it.
type seenChange struct {
	root   string
	folder string
}

// New returns a watcher of folders inside the node's configured roots, watching none until Watch
// names them.
func New(roots []*confine.MediaRoot, options Options, clockSource clock.Clock, logger *slog.Logger) *Watcher {
	byPath := make(map[string]*confine.MediaRoot, len(roots))
	for _, root := range roots {
		byPath[root.Path()] = root
	}
	return &Watcher{
		configured: byPath, options: options, clock: clockSource, logger: logger,
		changes: make(chan nodeproto.Changed, changesBuffer), seen: make(chan seenChange, changesBuffer),
		wanted: make(chan []string, 1), states: map[string]nodeproto.RootWatch{},
	}
}

// Watch has the watcher watch folders, and only them: the core's word on which library folders it
// watches (nodeproto.HelloReply.WatchFolders). A folder new to it is said not to be watched until the
// kernel layer has started its watch, and one no longer named is stopped. It returns at once; the
// same folders again change nothing.
func (watcher *Watcher) Watch(folders []string) {
	folders = slices.Compact(slices.Sorted(slices.Values(folders)))
	if folders == nil {
		folders = []string{}
	}
	watcher.mutex.Lock()
	defer watcher.mutex.Unlock()
	if watcher.folders != nil && slices.Equal(folders, watcher.folders) {
		return
	}
	states := make(map[string]nodeproto.RootWatch, len(folders))
	for _, folder := range folders {
		state, isKnown := watcher.states[folder]
		if !isKnown {
			state = nodeproto.RootWatch{
				RootPath: folder, Cause: nodeproto.WatchCauseUnavailable, Reason: "the watcher has not started",
			}
		}
		states[folder] = state
	}
	watcher.folders, watcher.states = folders, states
	select {
	case <-watcher.wanted: // the kernel layer has not taken the last set: this one replaces it
	default:
	}
	watcher.wanted <- folders
}

// Changes are the watcher's notices, for the agent to post (nodeproto.PathChanged).
func (watcher *Watcher) Changes() <-chan nodeproto.Changed { return watcher.changes }

// States are each folder's watch, for the node's status.
func (watcher *Watcher) States() []nodeproto.RootWatch {
	watcher.mutex.Lock()
	defer watcher.mutex.Unlock()
	states := make([]nodeproto.RootWatch, 0, len(watcher.folders))
	for _, folder := range watcher.folders {
		states = append(states, watcher.states[folder])
	}
	return states
}

// watchedFolders are the folders to watch, now.
func (watcher *Watcher) watchedFolders() []string {
	watcher.mutex.Lock()
	defer watcher.mutex.Unlock()
	return watcher.folders
}

// setState records a folder's watch, unless it is no longer one to watch, and warns of one not
// watched.
func (watcher *Watcher) setState(ctx context.Context, state nodeproto.RootWatch) {
	watcher.mutex.Lock()
	before, isWanted := watcher.states[state.RootPath]
	if isWanted {
		watcher.states[state.RootPath] = state
	}
	watcher.mutex.Unlock()
	if !isWanted || state.IsWatching || (!before.IsWatching && before.Cause == state.Cause) {
		return
	}
	watcher.logger.WarnContext(ctx, "not watching a library folder for changes; its periodic scan finds them",
		"rootPath", state.RootPath, "cause", string(state.Cause), "reason", state.Reason)
}

// refuseEvery says of every folder it is given, until ctx ends, that it is not watched, and why: where
// the kernel gives no watcher.
func (watcher *Watcher) refuseEvery(ctx context.Context, cause nodeproto.WatchCause, reason string) {
	for {
		select {
		case <-ctx.Done():
			return
		case folders := <-watcher.wanted:
			for _, folder := range folders {
				watcher.setState(ctx, nodeproto.RootWatch{RootPath: folder, Cause: cause, Reason: reason})
			}
		}
	}
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

// noticeDue notices every held folder whose changes are due, in the order of the folders; one no
// longer watched is dropped.
func (watcher *Watcher) noticeDue(ctx context.Context, pending map[string]*pendingRoot) {
	now := watcher.clock.Now()
	folders := watcher.watchedFolders()
	for root := range pending {
		if !slices.Contains(folders, root) {
			delete(pending, root)
		}
	}
	for _, root := range folders {
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
