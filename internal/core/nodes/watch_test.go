// SPDX-License-Identifier: AGPL-3.0-or-later

package nodes

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// takenChanges keeps the watchers' notices the channel hands on, and says the folders a node watches.
type takenChanges struct {
	mutex   sync.Mutex
	notices []nodeproto.Changed
	folders []string
}

func (taken *takenChanges) WatchedFolders(context.Context, int64) ([]string, error) {
	taken.mutex.Lock()
	defer taken.mutex.Unlock()
	return append([]string{}, taken.folders...), nil
}

// setFolders are the folders a node watches from now.
func (taken *takenChanges) setFolders(folders ...string) {
	taken.mutex.Lock()
	defer taken.mutex.Unlock()
	taken.folders = folders
}

func (taken *takenChanges) TakeChange(_ context.Context, _ int64, root, folder string) error {
	taken.mutex.Lock()
	defer taken.mutex.Unlock()
	taken.notices = append(taken.notices, nodeproto.Changed{RootPath: root, Path: folder})
	return nil
}

// A node watcher's notice is handed on, and each root's watch its status says is kept (D272 §9).
func TestChanged_AWatchersNoticeIsHandedOnAndItsRootsWatchesKept(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Millisecond)
	taken := &takenChanges{}
	harness.service.SetChanges(taken)
	reply := harness.hello(t, nodeproto.Hello{Name: "local", RootPaths: []string{"/media"}})
	watch := nodeproto.RootWatch{RootPath: "/media", IsWatching: true}
	harness.postStatus(t, reply.NodeID, nodeproto.Status{SentAt: 1, Watches: []nodeproto.RootWatch{watch}})
	if got := harness.service.RootWatches(reply.NodeID); len(got) != 1 || got["/media"] != watch {
		t.Errorf("the node's roots' watches are %+v, want %+v", got, watch)
	}
	response := harness.post(t, nodeproto.PathChanged, reply.NodeID,
		strings.NewReader(`{"rootPath":"/media","path":"Shows/Dag"}`))
	taken.mutex.Lock()
	defer taken.mutex.Unlock()
	want := nodeproto.Changed{RootPath: "/media", Path: "Shows/Dag"}
	if response.status != http.StatusOK || len(taken.notices) != 1 || taken.notices[0] != want {
		t.Errorf("a notice answered %d and handed on %+v; want 200 and %+v", response.status, taken.notices, want)
	}
}

// The hello's reply and each poll's answer name the library folders the node watches, and a change of
// them answers a waiting poll at once, and the next poll when none waits, never after a poll's hold
// (D272 §9, D359's amendment of 2026-10-27).
func TestHelloAndCommands_NameTheFoldersTheNodeWatchesAtOnceWhenTheyChange(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Hour)
	settingstest.Set(t, harness.settings, settings.ShouldScanOnNodeHello, "false")
	taken := &takenChanges{folders: []string{"/media/movies"}}
	harness.service.SetChanges(taken)
	reply := harness.hello(t, nodeproto.Hello{Name: "local", RootPaths: []string{"/media"}})
	if !slices.Equal(reply.WatchFolders, []string{"/media/movies"}) {
		t.Errorf("the hello's reply names %v to watch; want the library's folder", reply.WatchFolders)
	}
	got := make(chan nodeproto.Commands)
	go func() {
		commands, _ := harness.poll(t, reply.NodeID, 0)
		got <- commands
	}()
	taken.setFolders("/media/movies", "/media/shows")
	harness.service.WatchedFoldersChanged()
	answer := testutil.Receive(t, got, "the waiting poll's answer")
	if len(answer.Commands) != 0 || !slices.Equal(answer.WatchFolders, []string{"/media/movies", "/media/shows"}) {
		t.Errorf("the poll answered %+v; want no command and both folders to watch", answer)
	}
	taken.setFolders("/media/shows")
	harness.service.WatchedFoldersChanged()
	if next, _ := harness.poll(t, reply.NodeID, 0); !slices.Equal(next.WatchFolders, []string{"/media/shows"}) {
		t.Errorf("the next poll answered %+v; want the folder left to watch", next)
	}
}
