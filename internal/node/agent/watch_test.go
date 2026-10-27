// SPDX-License-Identifier: AGPL-3.0-or-later

package agent

import (
	"slices"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// fakeWatcher is a watcher whose notices the test sends, and whose folders' watches are fixed; the
// folders it is given arrive on watched, when the test made it.
type fakeWatcher struct {
	changes chan nodeproto.Changed
	states  []nodeproto.RootWatch
	watched chan []string
}

func (watcher fakeWatcher) Watch(folders []string) {
	select {
	case watcher.watched <- folders:
	default:
	}
}

func (watcher fakeWatcher) Changes() <-chan nodeproto.Changed { return watcher.changes }
func (watcher fakeWatcher) States() []nodeproto.RootWatch     { return watcher.states }

func TestAgent_PostsTheWatchersNoticesAndSaysEachRootsWatchInItsStatus(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	watcher := fakeWatcher{changes: make(chan nodeproto.Changed, 1), states: []nodeproto.RootWatch{
		{RootPath: "/media", Cause: nodeproto.WatchCauseNetworkFilesystem, Reason: "NFS"},
	}}
	_, tick := startAgentWithWorkers(t, socket, Workers{Scanner: fakeScanner{}, Watcher: watcher})
	tick <- time.Time{}
	if status := testutil.Receive(t, core.statuses, "a status"); len(status.Watches) != 1 ||
		status.Watches[0] != watcher.states[0] {
		t.Errorf("the status says the roots' watches are %+v, want %+v", status.Watches, watcher.states)
	}
	notice := nodeproto.Changed{RootPath: "/media", Path: "Shows/Dag"}
	watcher.changes <- notice
	if got := testutil.Receive(t, core.changes, "the watcher's notice"); got != notice {
		t.Errorf("the core heard %+v, want %+v", got, notice)
	}
}

// The library folders the core names, in its reply to the hello and in each poll's answer, are the
// ones the watcher watches (D359, 2026-10-27 amendment).
func TestAgent_HasTheWatcherWatchTheLibraryFoldersTheCoreNames(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	core.mutex.Lock()
	core.watchFolders = []string{"/media/movies"}
	core.mutex.Unlock()
	watcher := fakeWatcher{changes: make(chan nodeproto.Changed), watched: make(chan []string, 4)}
	startAgentWithWorkers(t, socket, Workers{Scanner: fakeScanner{}, Watcher: watcher})
	if got := testutil.Receive(t, watcher.watched, "the hello's folders"); !slices.Equal(got,
		[]string{"/media/movies"}) {
		t.Errorf("after the hello the watcher watches %v; want the folder the reply named", got)
	}
	core.mutex.Lock()
	core.watchFolders = []string{"/media/movies", "/media/shows"}
	core.mutex.Unlock()
	core.commands <- nodeproto.Command{CommandSeq: 1, JobID: 9, Work: nodeproto.Work{Type: nodeproto.CommandCancel}}
	if got := testutil.Receive(t, watcher.watched, "the answer's folders"); !slices.Equal(got,
		[]string{"/media/movies", "/media/shows"}) {
		t.Errorf("after a poll's answer the watcher watches %v; want the folders it named", got)
	}
}
