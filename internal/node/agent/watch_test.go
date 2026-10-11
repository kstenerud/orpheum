// SPDX-License-Identifier: AGPL-3.0-or-later

package agent

import (
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// fakeWatcher is a watcher whose notices the test sends, and whose roots' watches are fixed.
type fakeWatcher struct {
	changes chan nodeproto.Changed
	states  []nodeproto.RootWatch
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
