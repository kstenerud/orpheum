// SPDX-License-Identifier: GPL-3.0-or-later

package watch

import (
	"log/slog"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/nodetest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
)

const quiet = 30 * time.Second

// newWatcher is a watcher of directories, each a root, on a fake clock, with a quiet spell of quiet.
func newWatcher(t *testing.T, options Options, directories ...string) (*Watcher, *clocktest.Fake) {
	t.Helper()
	roots := make([]*confine.MediaRoot, 0, len(directories))
	for _, directory := range directories {
		roots = append(roots, nodetest.OpenMediaRoot(t, directory))
	}
	fake := clocktest.NewFake(time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC))
	options.Quiet = quiet
	return New(roots, options, fake, slog.New(slog.DiscardHandler)), fake
}

// noticed is every notice waiting, without waiting for more.
func noticed(watcher *Watcher) []nodeproto.Changed {
	var all []nodeproto.Changed
	for {
		select {
		case change := <-watcher.changes:
			all = append(all, change)
		default:
			return all
		}
	}
}

func TestWatcher_NoticesTheFolderHoldingARootsChangesOnceItGoesQuiet(t *testing.T) {
	t.Parallel()
	first, second := t.TempDir(), t.TempDir()
	watcher, fake := newWatcher(t, Options{}, first, second)
	pending := map[string]*pendingRoot{}
	watcher.fold(pending, seenChange{root: first, folder: "Shows/Dag/Season 1"})
	fake.Advance(quiet / 2)
	watcher.fold(pending, seenChange{root: first, folder: "Shows/Dag/Season 2"})
	watcher.fold(pending, seenChange{root: second, folder: "Films"})
	fake.Advance(quiet - time.Second)
	watcher.noticeDue(t.Context(), pending)
	if got := noticed(watcher); len(got) != 0 {
		t.Fatalf("noticed %v before the roots went quiet", got)
	}
	fake.Advance(time.Second)
	watcher.noticeDue(t.Context(), pending)
	got := noticed(watcher)
	want := []nodeproto.Changed{{RootPath: first, Path: "Shows/Dag"}, {RootPath: second, Path: "Films"}}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("noticed %v, want %v: each root once, by the folder holding its changes", got, want)
	}
	if wait, isAny := watcher.nextWait(pending); isAny {
		t.Errorf("a root is still held, due in %s", wait)
	}
}

func TestWatcher_NoticesAChangeThatNeverPausesAfterTheLongestHold(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	watcher, fake := newWatcher(t, Options{}, root)
	pending := map[string]*pendingRoot{}
	for range maxHoldQuietSpells * 2 {
		watcher.fold(pending, seenChange{root: root, folder: "Films/Copying"})
		fake.Advance(quiet / 2)
		watcher.noticeDue(t.Context(), pending)
	}
	if got := noticed(watcher); len(got) != 1 || got[0].Path != "Films/Copying" {
		t.Errorf("a copy writing for %d quiet spells was noticed %v; want once, at the longest hold",
			maxHoldQuietSpells, got)
	}
}

func TestCleanFolder_IsRelativeCleanAndUTF8(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ folder, want string }{
		{"Shows/./Dag/", "Shows/Dag"},
		{"", wholeRoot},
		{"/etc", wholeRoot},
		{"../out", wholeRoot},
		{"Shows/\xffbad/Season 1", "Shows"},
		{"\xffbad", wholeRoot},
	} {
		if got := cleanFolder(tc.folder); got != tc.want {
			t.Errorf("cleanFolder(%q) = %q, want %q", tc.folder, got, tc.want)
		}
	}
	for _, tc := range []struct{ first, second, want string }{
		{"Shows/Dag", "Shows/Dagger", "Shows"},
		{"Shows/Dag", "Shows/Dag/Season 1", "Shows/Dag"},
		{"Shows", "Films", wholeRoot},
	} {
		if got := widerFolder(tc.first, tc.second); got != tc.want {
			t.Errorf("widerFolder(%q, %q) = %q, want %q", tc.first, tc.second, got, tc.want)
		}
	}
}
