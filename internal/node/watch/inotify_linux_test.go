// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build linux

package watch

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"

	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// openWatched watches folders, each a configured root, with the kernel's events read until the test
// ends; the changes it sees arrive on the watcher's seen channel.
func openWatched(t *testing.T, options Options, folders ...string) (*Watcher, *kernelWatch) {
	t.Helper()
	watcher, kernel := openKernelOf(t, options, folders...)
	watchFolders(t, watcher, kernel, folders...)
	return watcher, kernel
}

// openKernelOf is a watcher of the configured roots, watching nothing yet, with the kernel's events
// read until the test ends.
func openKernelOf(t *testing.T, options Options, configured ...string) (*Watcher, *kernelWatch) {
	t.Helper()
	watcher, _ := newWatcher(t, options, configured...)
	watcher.Watch(nil)
	testutil.Receive(t, watcher.wanted, "the folders the core named first")
	kernel, file, err := openKernel()
	if err != nil {
		testutil.Require(t, "inotify", err)
	}
	kernel.watcher = watcher
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = kernel.read(ctx, file) //nolint:errcheck // the file closes as the test ends
	}()
	t.Cleanup(func() {
		cancel()
		_ = file.Close() //nolint:errcheck // the read has ended or ends now
		testutil.Receive(t, done, "the kernel's reading to end")
		kernel.closeAll(ctx) // ended: it only logs
	})
	return watcher, kernel
}

// watchFolders gives the watcher folders to watch, as the core does, and has the kernel layer take
// them at once.
func watchFolders(t *testing.T, watcher *Watcher, kernel *kernelWatch, folders ...string) {
	t.Helper()
	watcher.Watch(folders)
	kernel.setFolders(t.Context(), testutil.Receive(t, watcher.wanted, "the folders to watch"))
}

// watchedFolders are every folder the kernel layer watches, by library folder and path below it.
func watchedFolders(kernel *kernelWatch) map[watchedFolder]bool {
	kernel.mutex.Lock()
	defer kernel.mutex.Unlock()
	watched := map[watchedFolder]bool{}
	for folder := range kernel.descriptors {
		watched[folder] = true
	}
	return watched
}

// nextSeen is the next change the kernel layer saw.
func nextSeen(t *testing.T, watcher *Watcher) seenChange {
	t.Helper()
	return testutil.Receive(t, watcher.seen, "a change the kernel saw")
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o750); err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// seenUntil is the changes seen until one under folder of root arrives, which it returns.
func seenUntil(t *testing.T, watcher *Watcher, root, folder string) {
	t.Helper()
	for {
		if seen := nextSeen(t, watcher); seen.root == root && seen.folder == folder {
			return
		}
	}
}

func TestKernel_NoticesWhatChangesInEveryFolderAndInFoldersMadeSince(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "Shows", "Dag"))
	watcher, _ := openWatched(t, Options{}, root)
	if states := watcher.States(); len(states) != 1 || !states[0].IsWatching {
		t.Fatalf("the root's watch is %+v, want it watched", states)
	}
	mustWrite(t, filepath.Join(root, "Shows", "Dag", "1.mkv"))
	seenUntil(t, watcher, root, "Shows/Dag")

	// Each folder is made once the one above it is watched: made together, Metropolis could be made after
	// Films was listed and before its own creation was read, and a file written in that gap is seen
	// as Films' change, not its own (the ./dev pre-commit, under load, 2026-10-24).
	mustMkdir(t, filepath.Join(root, "Films"))
	seenUntil(t, watcher, root, wholeRoot) // Films made in the root
	mustMkdir(t, filepath.Join(root, "Films", "Metropolis"))
	seenUntil(t, watcher, root, "Films") // Metropolis made in Films, which is watched
	mustWrite(t, filepath.Join(root, "Films", "Metropolis", "Metropolis.mkv"))
	seenUntil(t, watcher, root, "Films/Metropolis") // a folder made since is watched too

	//nolint:forbidigo // the test's own temporary folder, removed as a person would remove a show
	if err := os.RemoveAll(filepath.Join(root, "Shows", "Dag")); err != nil {
		t.Fatal(err)
	}
	seenUntil(t, watcher, root, "Shows") // a removed folder is noticed by the one it was in
}

func TestKernel_AnOverflowNoticesEveryRootWhole(t *testing.T) {
	t.Parallel()
	first, second := t.TempDir(), t.TempDir()
	watcher, kernel := openWatched(t, Options{}, first, second)
	kernel.mutex.Lock()
	kernel.takeEvent(t.Context(), -1, unix.IN_Q_OVERFLOW, "")
	kernel.mutex.Unlock()
	seen := map[string]string{}
	for range 2 {
		change := nextSeen(t, watcher)
		seen[change.root] = change.folder
	}
	if seen[first] != wholeRoot || seen[second] != wholeRoot {
		t.Errorf("after lost events the roots were noticed as %v, want each whole", seen)
	}
}

func TestKernel_ARootOnANetworkFilesystemIsNotWatchedAndSaysWhy(t *testing.T) {
	t.Parallel()
	local, network := t.TempDir(), t.TempDir()
	watcher, kernel := openWatched(t, Options{
		IsOnNetworkFilesystem: func(path string) bool { return path == network },
	}, local, network)
	states := map[string]nodeproto.RootWatch{}
	for _, state := range watcher.States() {
		states[state.RootPath] = state
	}
	if !states[local].IsWatching || states[network].IsWatching ||
		states[network].Cause != nodeproto.WatchCauseNetworkFilesystem {
		t.Errorf("the roots' watches are %+v; want the local one watched, the network one not, saying why", states)
	}
	// The reason holds for a WSL drive, whose changes come from Windows on the same machine (D314 §5).
	if !strings.Contains(states[network].Reason, "WSL") {
		t.Errorf("the network root's reason is %q; want it to name a WSL drive's Windows side too",
			states[network].Reason)
	}
	for watched := range watchedFolders(kernel) {
		if watched.root == network {
			t.Errorf("the network root's folder %q is watched", watched.folder)
		}
	}
}

func TestKernel_ARootThatCannotBeWatchedWholeIsNotWatchedAndSaysWhy(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "Shows", "Dag"))
	watcher, kernel := openWatched(t, Options{}, root)
	kernel.mutex.Lock()
	kernel.unwatchFolder(t.Context(), root, fmt.Errorf("watch %s: %w", root, unix.ENOSPC))
	kernel.mutex.Unlock()
	if state := watcher.States()[0]; state.IsWatching || state.Cause != nodeproto.WatchCauseWatchLimit {
		t.Errorf("at the watch limit the root's watch is %+v, want not watched, at the limit", state)
	}
	for watched := range watchedFolders(kernel) {
		if watched.root == root {
			t.Errorf("the root's folder %q is still watched; want none of it", watched.folder)
		}
	}
	kernel.mutex.Lock()
	kernel.takeEvent(t.Context(), -1, unix.IN_Q_OVERFLOW, "")
	kernel.mutex.Unlock()
	select {
	case seen := <-watcher.seen:
		t.Errorf("a root not watched was noticed: %+v", seen)
	default:
	}
}

// A configured root wider than its libraries' folders is never walked: only the library folders the
// core names are watched, and a change elsewhere in the root is never seen (staging, 2026-10-27:
// /tank/media watched whole, thousands of watches, every library "has not started" for minutes).
func TestKernel_WatchesOnlyTheLibraryFoldersItIsGivenNeverTheRestOfTheirRoot(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	movies := filepath.Join(root, "video", "movies")
	mustMkdir(t, filepath.Join(movies, "Metropolis"))
	mustMkdir(t, filepath.Join(root, "audio", "Albums"))
	mustMkdir(t, filepath.Join(root, "books"))
	watcher, kernel := openKernelOf(t, Options{}, root)
	if watched := watchedFolders(kernel); len(watched) != 0 {
		t.Fatalf("before the core named a folder the watcher watches %v; want nothing", watched)
	}
	watchFolders(t, watcher, kernel, movies)
	want := map[watchedFolder]bool{{root: movies, folder: wholeRoot}: true, {root: movies, folder: "Metropolis"}: true}
	if watched := watchedFolders(kernel); !maps.Equal(watched, want) {
		t.Errorf("the watcher watches %v; want the library folder and its folders alone, %v", watched, want)
	}
	if states := watcher.States(); len(states) != 1 || states[0].RootPath != movies || !states[0].IsWatching {
		t.Errorf("the watches are %+v; want the library folder's alone, watched", states)
	}
	mustWrite(t, filepath.Join(root, "audio", "Albums", "1.flac"))
	mustWrite(t, filepath.Join(movies, "Metropolis", "Metropolis.mkv"))
	if seen := nextSeen(t, watcher); seen.root != movies || seen.folder != "Metropolis" {
		t.Errorf("the first change seen was %+v; want the library folder's, the root's other folders unwatched",
			seen)
	}
}

// Adding a folder to a library starts its watch; retiring it, or deleting its library, which the core
// says by no longer naming it, stops it, and its watch leaves the status.
func TestKernel_AFolderGivenIsWatchedAndOneNoLongerGivenIsNot(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	movies, shows := filepath.Join(root, "movies"), filepath.Join(root, "shows")
	mustMkdir(t, filepath.Join(movies, "Metropolis"))
	mustMkdir(t, filepath.Join(shows, "Dag"))
	watcher, kernel := openKernelOf(t, Options{}, root)
	watchFolders(t, watcher, kernel, movies)
	watchFolders(t, watcher, kernel, movies, shows)
	if states := watcher.States(); len(states) != 2 || !states[0].IsWatching || !states[1].IsWatching ||
		states[0].RootPath != movies || states[1].RootPath != shows {
		t.Fatalf("after a folder was added the watches are %+v; want both folders watched", states)
	}
	mustWrite(t, filepath.Join(shows, "Dag", "1.mkv"))
	seenUntil(t, watcher, shows, "Dag")

	watchFolders(t, watcher, kernel, shows)
	for watched := range watchedFolders(kernel) {
		if watched.root == movies {
			t.Errorf("the retired folder's %q is still watched", watched.folder)
		}
	}
	if states := watcher.States(); len(states) != 1 || states[0].RootPath != shows {
		t.Errorf("after a folder was retired the watches are %+v; want the other's alone", states)
	}
	watchFolders(t, watcher, kernel)
	if watched := watchedFolders(kernel); len(watched) != 0 {
		t.Errorf("with no library folder named the watcher watches %v; want nothing", watched)
	}
	if states := watcher.States(); len(states) != 0 {
		t.Errorf("with no library folder named the watches are %+v; want none", states)
	}
}

// A folder no configured root holds is refused, whatever the core names (D107).
func TestKernel_AFolderNoConfiguredRootHoldsIsNotWatchedAndSaysWhy(t *testing.T) {
	t.Parallel()
	root, outside := t.TempDir(), t.TempDir()
	watcher, kernel := openKernelOf(t, Options{}, root)
	watchFolders(t, watcher, kernel, outside, root+"/../"+filepath.Base(outside))
	states := watcher.States()
	if len(states) != 2 {
		t.Fatalf("the watches are %+v; want one for each folder named", states)
	}
	for _, state := range states {
		if state.IsWatching || state.Cause != nodeproto.WatchCauseUnreadable {
			t.Errorf("a folder outside the roots has the watch %+v; want it not watched, saying why", state)
		}
	}
	if watched := watchedFolders(kernel); len(watched) != 0 {
		t.Errorf("the watcher watches %v; want nothing outside its roots", watched)
	}
}
