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
	"unsafe"

	"golang.org/x/sys/unix"

	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// watchedEvents are what a folder's watch reports: an entry made, removed, moved, written or changed,
// and the folder itself removed or moved. IN_MODIFY keeps a copy in progress from looking quiet.
// Links are not followed, as a scan follows them only into roots, which have their own watches.
const watchedEvents = unix.IN_CREATE | unix.IN_DELETE | unix.IN_MOVED_FROM | unix.IN_MOVED_TO |
	unix.IN_CLOSE_WRITE | unix.IN_MODIFY | unix.IN_ATTRIB | unix.IN_DELETE_SELF | unix.IN_MOVE_SELF |
	unix.IN_ONLYDIR | unix.IN_DONT_FOLLOW | unix.IN_EXCL_UNLINK

// readBufferBytes holds many events at once: each is 16 bytes and its name.
const readBufferBytes = 64 << 10

// watchedFolder is a folder a watch descriptor names: its root, and its path relative to it.
type watchedFolder struct {
	root   string
	folder string
}

// kernelWatch is the node's inotify instance and the folders it watches.
type kernelWatch struct {
	watcher *Watcher
	fd      int
	// folders are the watched folders by their descriptor, and descriptors by root and folder.
	folders     map[int32]watchedFolder
	descriptors map[watchedFolder]int32
	// isWatching are the roots watched whole.
	isWatching map[string]bool
}

// watchKernel watches every root not on a network filesystem with inotify, and reports each change
// to the watcher until ctx ends. A root it cannot watch whole it does not watch at all, and says why.
func watchKernel(ctx context.Context, watcher *Watcher) {
	kernel, file := openKernel(ctx, watcher)
	if kernel == nil {
		return
	}
	stop := context.AfterFunc(ctx, func() { _ = file.Close() }) //nolint:errcheck // ending the read is all it is for
	defer stop()
	kernel.read(ctx, file)
}

// openKernel makes the inotify instance and watches the roots, or says why it cannot and returns nil.
func openKernel(ctx context.Context, watcher *Watcher) (*kernelWatch, *os.File) {
	instance, err := unix.InotifyInit1(unix.IN_CLOEXEC | unix.IN_NONBLOCK)
	if err != nil {
		for _, root := range watcher.roots {
			watcher.setState(ctx, nodeproto.RootWatch{
				RootPath: root, Cause: nodeproto.WatchCauseUnavailable,
				Reason: fmt.Sprintf("the kernel gave no inotify instance: %v (fs.inotify.max_user_instances)", err),
			})
		}
		return nil, nil
	}
	// Non-blocking, so the runtime's poller waits on it and closing it ends a read in progress.
	file := os.NewFile(uintptr(instance), "inotify")
	kernel := &kernelWatch{
		watcher: watcher, fd: instance, folders: map[int32]watchedFolder{}, descriptors: map[watchedFolder]int32{},
		isWatching: map[string]bool{},
	}
	for _, root := range watcher.roots {
		if slices.Contains(watcher.options.NetworkRoots, root) {
			watcher.setState(ctx, nodeproto.RootWatch{
				RootPath: root, Cause: nodeproto.WatchCauseNetworkFilesystem,
				Reason: "changes made elsewhere to a network or shared filesystem (another machine, or the " +
					"Windows side of a WSL drive) never reach this kernel",
			})
			continue
		}
		kernel.watchRoot(ctx, root)
	}
	return kernel, file
}

// watchRoot watches every folder of a root, or none, saying which.
func (kernel *kernelWatch) watchRoot(ctx context.Context, root string) {
	if err := kernel.addTree(root, wholeRoot); err != nil {
		kernel.unwatchRoot(ctx, root, err)
		return
	}
	kernel.isWatching[root] = true
	kernel.watcher.setState(ctx, nodeproto.RootWatch{RootPath: root, IsWatching: true})
}

// unwatchRoot stops watching a root that cannot be watched whole, and says why: partly watched, it
// would miss changes and look as if it did not.
func (kernel *kernelWatch) unwatchRoot(ctx context.Context, root string, cause error) {
	kernel.removeTree(root, wholeRoot)
	delete(kernel.isWatching, root)
	state := nodeproto.RootWatch{RootPath: root, Cause: nodeproto.WatchCauseUnreadable, Reason: cause.Error()}
	if errors.Is(cause, unix.ENOSPC) {
		state.Cause = nodeproto.WatchCauseWatchLimit
		state.Reason = "the root has more folders than the kernel lets this user watch " +
			"(fs.inotify.max_user_watches): " + cause.Error()
	}
	kernel.watcher.setState(ctx, state)
}

// addTree watches folder, under root, and every folder below it.
func (kernel *kernelWatch) addTree(root, folder string) error {
	media := kernel.watcher.mediaRoots[root]
	descriptor, err := unix.InotifyAddWatch(kernel.fd, path.Join(root, folder), watchedEvents)
	if err != nil {
		return fmt.Errorf("watch %s: %w", path.Join(root, folder), err)
	}
	watched := watchedFolder{root: root, folder: folder}
	kernel.folders[int32(descriptor)] = watched     //nolint:gosec // a descriptor is an int32 on the wire
	kernel.descriptors[watched] = int32(descriptor) //nolint:gosec // as above
	entries, err := media.ReadDir(folder)
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

// removeTree stops watching folder, under root, and every folder below it.
func (kernel *kernelWatch) removeTree(root, folder string) {
	for watched, descriptor := range kernel.descriptors {
		if watched.root != root || !isAtOrBelow(watched.folder, folder) {
			continue
		}
		//nolint:errcheck,gosec // a descriptor the kernel gave, gone already if its folder is
		_, _ = unix.InotifyRmWatch(kernel.fd, uint32(descriptor))
		delete(kernel.descriptors, watched)
		delete(kernel.folders, descriptor)
	}
}

// read reads events until the instance is closed, and reports each change.
func (kernel *kernelWatch) read(ctx context.Context, file *os.File) {
	buffer := make([]byte, readBufferBytes)
	for {
		count, err := file.Read(buffer)
		if err != nil {
			if ctx.Err() == nil {
				for root := range kernel.isWatching {
					kernel.unwatchRoot(ctx, root, fmt.Errorf("reading the kernel's events failed: %w", err))
				}
			}
			return
		}
		kernel.take(ctx, buffer[:count])
	}
}

// take reports the changes a read's events say.
func (kernel *kernelWatch) take(ctx context.Context, events []byte) {
	for offset := 0; offset+unix.SizeofInotifyEvent <= len(events); {
		event := (*unix.InotifyEvent)(unsafe.Pointer(&events[offset])) //nolint:gosec // the kernel's layout
		nameStart := offset + unix.SizeofInotifyEvent
		name := string(bytes.TrimRight(events[nameStart:nameStart+int(event.Len)], "\x00"))
		offset = nameStart + int(event.Len)
		kernel.takeEvent(ctx, event.Wd, event.Mask, name)
	}
}

// takeEvent reports one event's change, and follows folders made, moved in, moved out and removed.
func (kernel *kernelWatch) takeEvent(ctx context.Context, descriptor int32, mask uint32, name string) {
	if mask&unix.IN_Q_OVERFLOW != 0 {
		// Events were lost: every watched root is noticed whole.
		for root := range kernel.isWatching {
			kernel.watcher.saw(ctx, root, wholeRoot)
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
		kernel.unwatchRoot(ctx, watched.root, errors.New("the root itself was removed or moved"))
		kernel.watcher.saw(ctx, watched.root, wholeRoot)
		return
	case mask&unix.IN_ISDIR != 0 && mask&(unix.IN_CREATE|unix.IN_MOVED_TO) != 0:
		if err := kernel.addTree(watched.root, path.Join(watched.folder, name)); err != nil &&
			!errors.Is(err, unix.ENOENT) {
			kernel.unwatchRoot(ctx, watched.root, err)
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
