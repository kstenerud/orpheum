// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build linux

package watch

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"

	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// openWatched watches root's folders, with the kernel's events read until the test ends; the
// changes it sees arrive on the watcher's seen channel.
func openWatched(t *testing.T, options Options, roots ...string) (*Watcher, *kernelWatch) {
	t.Helper()
	watcher, _ := newWatcher(t, options, roots...)
	ctx, cancel := context.WithCancel(t.Context())
	kernel, file := openKernel(ctx, watcher)
	if kernel == nil {
		testutil.Require(t, "inotify", fmt.Errorf("no inotify instance: %+v", watcher.States()))
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		kernel.read(ctx, file)
	}()
	t.Cleanup(func() {
		cancel()
		_ = file.Close() //nolint:errcheck // the read has ended or ends now
		testutil.Receive(t, done, "the kernel's reading to end")
	})
	return watcher, kernel
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
	kernel.takeEvent(t.Context(), -1, unix.IN_Q_OVERFLOW, "")
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
	watcher, kernel := openWatched(t, Options{NetworkRoots: []string{network}}, local, network)
	states := watcher.States()
	if !states[0].IsWatching || states[1].IsWatching || states[1].Cause != nodeproto.WatchCauseNetworkFilesystem {
		t.Errorf("the roots' watches are %+v; want the local one watched, the network one not, saying why", states)
	}
	// The reason holds for a WSL drive, whose changes come from Windows on the same machine (D314 §5).
	if !strings.Contains(states[1].Reason, "WSL") {
		t.Errorf("the network root's reason is %q; want it to name a WSL drive's Windows side too", states[1].Reason)
	}
	if kernel.isWatching[network] {
		t.Error("the network root is watched")
	}
}

func TestKernel_ARootThatCannotBeWatchedWholeIsNotWatchedAndSaysWhy(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "Shows", "Dag"))
	watcher, kernel := openWatched(t, Options{}, root)
	kernel.unwatchRoot(t.Context(), root, fmt.Errorf("watch %s: %w", root, unix.ENOSPC))
	if state := watcher.States()[0]; state.IsWatching || state.Cause != nodeproto.WatchCauseWatchLimit {
		t.Errorf("at the watch limit the root's watch is %+v, want not watched, at the limit", state)
	}
	for watched := range kernel.descriptors {
		if watched.root == root {
			t.Errorf("the root's folder %q is still watched; want none of it", watched.folder)
		}
	}
	kernel.takeEvent(t.Context(), -1, unix.IN_Q_OVERFLOW, "")
	select {
	case seen := <-watcher.seen:
		t.Errorf("a root not watched was noticed: %+v", seen)
	default:
	}
}
