// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build linux

package watch

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"slices"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/unix"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/slogx"
)

// watchedEvents are what a folder's watch reports: an entry made, removed, moved, written or changed,
// and the folder itself removed or moved. IN_MODIFY keeps a copy in progress from looking quiet.
// Links are not followed, as a scan follows them only into roots, which have their own watches.
const watchedEvents = unix.IN_CREATE | unix.IN_DELETE | unix.IN_MOVED_FROM | unix.IN_MOVED_TO |
	unix.IN_CLOSE_WRITE | unix.IN_MODIFY | unix.IN_ATTRIB | unix.IN_DELETE_SELF | unix.IN_MOVE_SELF |
	unix.IN_ONLYDIR | unix.IN_DONT_FOLLOW | unix.IN_EXCL_UNLINK

// readBufferBytes holds many events at once: each is 16 bytes and its name.
const readBufferBytes = 64 << 10

// watchedFolder is a folder a watch descriptor names: the library folder it is in, and its path
// relative to it.
type watchedFolder struct {
	root   string
	folder string
}

// givenFolder is a library folder the kernel layer was given to watch.
type givenFolder struct {
	// media is the folder as a media root of its own, which it lists through; nil when it is not
	// watched.
	media *confine.MediaRoot
	// opened is media when the kernel layer opened it, a folder inside a configured root, to close
	// when the folder is no longer watched; nil for a configured root itself, which the node holds.
	opened *confine.MediaRoot
}

// kernelWatch is the node's inotify instance and the folders it watches.
type kernelWatch struct {
	watcher *Watcher
	fd      int

	// mutex is held by each change of the folders given and each event taken, so neither sees the
	// other half done.
	mutex sync.Mutex
	// folders are the watched folders by their descriptor, and descriptors by library folder and
	// folder. Guarded by mutex.
	folders     map[int32]watchedFolder
	descriptors map[watchedFolder]int32
	// given are the library folders it was given, watched or not. Guarded by mutex.
	given map[string]*givenFolder
	// isWatching are the library folders watched whole. Guarded by mutex.
	isWatching map[string]bool
}

// watchKernel watches each library folder it is given, not on a network filesystem, with inotify, and
// reports each change to the watcher until ctx ends. A folder it cannot watch whole it does not watch
// at all, and says why.
func watchKernel(ctx context.Context, watcher *Watcher) {
	kernel, file, err := openKernel()
	if err != nil {
		watcher.refuseEvery(ctx, nodeproto.WatchCauseUnavailable, err.Error())
		return
	}
	kernel.watcher = watcher
	stop := context.AfterFunc(ctx, func() { _ = file.Close() }) //nolint:errcheck // ending the read is all it is for
	defer stop()
	followCtx, stopFollowing := context.WithCancel(ctx)
	var following sync.WaitGroup
	following.Go(func() { kernel.follow(followCtx) })
	readErr := kernel.read(ctx, file)
	stopFollowing()
	following.Wait()
	kernel.closeAll(ctx)
	if ctx.Err() == nil {
		watcher.refuseEvery(ctx, nodeproto.WatchCauseUnreadable, readErr.Error())
	}
}

// openKernel makes the inotify instance, watching nothing yet.
func openKernel() (*kernelWatch, *os.File, error) {
	instance, err := unix.InotifyInit1(unix.IN_CLOEXEC | unix.IN_NONBLOCK)
	if err != nil {
		return nil, nil, fmt.Errorf("the kernel gave no inotify instance: %w (fs.inotify.max_user_instances)", err)
	}
	// Non-blocking, so the runtime's poller waits on it and closing it ends a read in progress.
	file := os.NewFile(uintptr(instance), "inotify")
	return &kernelWatch{
		fd: instance, folders: map[int32]watchedFolder{}, descriptors: map[watchedFolder]int32{},
		given: map[string]*givenFolder{}, isWatching: map[string]bool{},
	}, file, nil
}

// follow watches the folders the watcher is given, as they change, until ctx ends.
func (kernel *kernelWatch) follow(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case folders := <-kernel.watcher.wanted:
			kernel.setFolders(ctx, folders)
		}
	}
}

// setFolders watches exactly folders: it stops watching each folder it was given that folders does not
// name, and starts each one they name that it was not given.
func (kernel *kernelWatch) setFolders(ctx context.Context, folders []string) {
	kernel.mutex.Lock()
	defer kernel.mutex.Unlock()
	for folder := range kernel.given {
		if !slices.Contains(folders, folder) {
			kernel.stopFolder(ctx, folder)
		}
	}
	for _, folder := range folders {
		if _, isGiven := kernel.given[folder]; !isGiven {
			kernel.startFolder(ctx, folder)
		}
	}
}

// startFolder watches a library folder and every folder below it, or none, saying which: one on a
// network filesystem (D27), one no configured root holds (D107), and one that cannot be watched whole
// are not watched. Its mutex is held.
func (kernel *kernelWatch) startFolder(ctx context.Context, folder string) {
	given := &givenFolder{}
	kernel.given[folder] = given
	watcher := kernel.watcher
	if watcher.options.IsOnNetworkFilesystem != nil && watcher.options.IsOnNetworkFilesystem(folder) {
		watcher.setState(ctx, nodeproto.RootWatch{
			RootPath: folder, Cause: nodeproto.WatchCauseNetworkFilesystem,
			Reason: "changes made elsewhere to a network or shared filesystem (another machine, or the " +
				"Windows side of a WSL drive) never reach this kernel",
		})
		return
	}
	root, relative, isHeld := confine.Holding(watcher.configured, folder)
	if !isHeld {
		watcher.logger.WarnContext(ctx, "refused to watch a folder no configured root holds", "rootPath", folder)
		watcher.setState(ctx, nodeproto.RootWatch{
			RootPath: folder, Cause: nodeproto.WatchCauseUnreadable,
			Reason: "no root this node was configured with holds it",
		})
		return
	}
	given.media = root
	if relative != wholeRoot {
		opened, err := root.Folder(relative)
		if err != nil {
			given.media = nil
			watcher.setState(ctx, nodeproto.RootWatch{
				RootPath: folder, Cause: nodeproto.WatchCauseUnreadable, Reason: err.Error(),
			})
			return
		}
		given.media, given.opened = opened, opened
	}
	if err := kernel.addTree(folder, wholeRoot); err != nil {
		kernel.unwatchFolder(ctx, folder, err)
		return
	}
	kernel.isWatching[folder] = true
	watcher.setState(ctx, nodeproto.RootWatch{RootPath: folder, IsWatching: true})
	watcher.logger.InfoContext(ctx, "started watching a library folder for changes",
		slogx.EventKey, "folderWatchStarted",
		"rootPath", folder,
		"watchCount", kernel.watchCount(folder))
}

// stopFolder stops watching a library folder no longer given, and closes what it opened. Its mutex is
// held.
func (kernel *kernelWatch) stopFolder(ctx context.Context, folder string) {
	released := kernel.removeTree(folder, wholeRoot)
	kernel.closeGiven(ctx, folder)
	delete(kernel.given, folder)
	delete(kernel.isWatching, folder)
	kernel.watcher.logger.InfoContext(ctx, "stopped watching a library folder for changes", "rootPath", folder,
		"watchCount", released)
}

// closeGiven closes the media root opened for a library folder, if one was.
func (kernel *kernelWatch) closeGiven(ctx context.Context, folder string) {
	given := kernel.given[folder]
	if given == nil || given.opened == nil {
		return
	}
	if err := given.opened.Close(); err != nil {
		kernel.watcher.logger.WarnContext(ctx, "could not close a library folder", "rootPath", folder, "err", err)
	}
	given.opened = nil
}

// closeAll closes every library folder it opened, once it watches no more.
func (kernel *kernelWatch) closeAll(ctx context.Context) {
	kernel.mutex.Lock()
	defer kernel.mutex.Unlock()
	for folder := range kernel.given {
		kernel.closeGiven(ctx, folder)
	}
}

// unwatchFolder stops watching a library folder that cannot be watched whole, and says why: partly
// watched, it would miss changes and look as if it did not. Its mutex is held.
func (kernel *kernelWatch) unwatchFolder(ctx context.Context, folder string, cause error) {
	kernel.removeTree(folder, wholeRoot)
	delete(kernel.isWatching, folder)
	state := nodeproto.RootWatch{RootPath: folder, Cause: nodeproto.WatchCauseUnreadable, Reason: cause.Error()}
	if errors.Is(cause, unix.ENOSPC) {
		state.Cause = nodeproto.WatchCauseWatchLimit
		state.Reason = "the folder has more folders than the kernel lets this user watch " +
			"(fs.inotify.max_user_watches): " + cause.Error()
	}
	kernel.watcher.setState(ctx, state)
}

// addTree watches folder, under the library folder root, and every folder below it.
func (kernel *kernelWatch) addTree(root, folder string) error {
	given := kernel.given[root]
	if given == nil || given.media == nil {
		return fmt.Errorf("watch %s: the library folder is not open", path.Join(root, folder))
	}
	descriptor, err := unix.InotifyAddWatch(kernel.fd, path.Join(root, folder), watchedEvents)
	if err != nil {
		return fmt.Errorf("watch %s: %w", path.Join(root, folder), err)
	}
	watched := watchedFolder{root: root, folder: folder}
	kernel.folders[int32(descriptor)] = watched     //nolint:gosec // a descriptor is an int32 on the wire
	kernel.descriptors[watched] = int32(descriptor) //nolint:gosec // as above
	entries, err := given.media.ReadDir(folder)
	if err != nil {
		return fmt.Errorf("list %s: %w", path.Join(root, folder), err)
	}
	for _, entry := range entries {
		if entry.Type()&fs.ModeType != fs.ModeDir {
			continue // a file, or a link, which is not followed
		}
		if err := kernel.addTree(root, path.Join(folder, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

// removeTree stops watching folder, under the library folder root, and every folder below it, and
// says how many watches it released.
func (kernel *kernelWatch) removeTree(root, folder string) int {
	released := 0
	for watched, descriptor := range kernel.descriptors {
		if watched.root != root || !isAtOrBelow(watched.folder, folder) {
			continue
		}
		//nolint:errcheck,gosec // a descriptor the kernel gave, gone already if its folder is
		_, _ = unix.InotifyRmWatch(kernel.fd, uint32(descriptor))
		delete(kernel.descriptors, watched)
		delete(kernel.folders, descriptor)
		released++
	}
	return released
}

// watchCount is how many watches a library folder holds.
func (kernel *kernelWatch) watchCount(root string) int {
	count := 0
	for watched := range kernel.descriptors {
		if watched.root == root {
			count++
		}
	}
	return count
}

// read reads events until the instance is closed, and reports each change; a read that fails stops
// every watch, and is returned.
func (kernel *kernelWatch) read(ctx context.Context, file *os.File) error {
	buffer := make([]byte, readBufferBytes)
	for {
		count, err := file.Read(buffer)
		if err != nil {
			err = fmt.Errorf("reading the kernel's events failed: %w", err)
			if ctx.Err() == nil {
				kernel.mutex.Lock()
				for folder := range kernel.isWatching {
					kernel.unwatchFolder(ctx, folder, err)
				}
				kernel.mutex.Unlock()
			}
			return err
		}
		kernel.take(ctx, buffer[:count])
	}
}

// take reports the changes a read's events say.
func (kernel *kernelWatch) take(ctx context.Context, events []byte) {
	kernel.mutex.Lock()
	defer kernel.mutex.Unlock()
	for offset := 0; offset+unix.SizeofInotifyEvent <= len(events); {
		event := (*unix.InotifyEvent)(unsafe.Pointer(&events[offset])) //nolint:gosec // the kernel's layout
		nameStart := offset + unix.SizeofInotifyEvent
		name := string(bytes.TrimRight(events[nameStart:nameStart+int(event.Len)], "\x00"))
		offset = nameStart + int(event.Len)
		kernel.takeEvent(ctx, event.Wd, event.Mask, name)
	}
}

// takeEvent reports one event's change, and follows folders made, moved in, moved out and removed. Its
// mutex is held.
func (kernel *kernelWatch) takeEvent(ctx context.Context, descriptor int32, mask uint32, name string) {
	if mask&unix.IN_Q_OVERFLOW != 0 {
		// Events were lost: every watched library folder is noticed whole.
		for folder := range kernel.isWatching {
			kernel.watcher.saw(ctx, folder, wholeRoot)
		}
		return
	}
	watched, isKnown := kernel.folders[descriptor]
	if !isKnown || !kernel.isWatching[watched.root] {
		return
	}
	switch {
	case mask&unix.IN_IGNORED != 0:
		delete(kernel.folders, descriptor)
		delete(kernel.descriptors, watched)
		return
	case mask&(unix.IN_DELETE_SELF|unix.IN_MOVE_SELF) != 0 && watched.folder == wholeRoot:
		kernel.unwatchFolder(ctx, watched.root, errors.New("the library folder itself was removed or moved"))
		kernel.watcher.saw(ctx, watched.root, wholeRoot)
		return
	case mask&unix.IN_ISDIR != 0 && mask&(unix.IN_CREATE|unix.IN_MOVED_TO) != 0:
		if err := kernel.addTree(watched.root, path.Join(watched.folder, name)); err != nil &&
			!errors.Is(err, unix.ENOENT) {
			kernel.unwatchFolder(ctx, watched.root, err)
		}
	case mask&unix.IN_ISDIR != 0 && mask&(unix.IN_MOVED_FROM|unix.IN_DELETE) != 0:
		kernel.removeTree(watched.root, path.Join(watched.folder, name))
	}
	kernel.watcher.saw(ctx, watched.root, watched.folder)
}

// isAtOrBelow reports whether folder is below, or is, above, both relative to one root.
func isAtOrBelow(folder, above string) bool {
	return above == wholeRoot || folder == above || strings.HasPrefix(folder, above+"/")
}
