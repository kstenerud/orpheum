// SPDX-License-Identifier: GPL-3.0-or-later

package agent

import (
	"log/slog"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/nodetest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// A command's work reads the device its file is on, as the hello found the devices, and is
// play-driven as the command says: a play-driven prepare pauses the background work on that
// device, and the status says it is paused (D240 §6).
func TestAgent_ACommandsWorkReadsItsFilesDeviceAndPlayDrivenWorkPausesTheBackground(t *testing.T) {
	t.Parallel()
	root := nodetest.OpenMediaRoot(t, t.TempDir())
	if err := os.WriteFile(filepath.Join(root.Path(), "film.mkv"), []byte("a film"), 0o600); err != nil {
		t.Fatal(err)
	}
	preparer := releasedPreparer{started: make(chan struct{}), release: make(chan struct{}), stopped: &atomic.Bool{}}
	node := New("/nowhere", nodeproto.Hello{Name: "local", Capacity: testCapacity}, []*confine.MediaRoot{root},
		Workers{Preparer: preparer}, clocktest.NewFake(time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)),
		slog.New(slog.DiscardHandler))
	node.devices = []nodeproto.Device{{Key: "zfs:tank", MountPaths: []string{root.Path()}}}
	background := newBlockingWork()
	node.work.hold(t.Context(), background.onDevice(1, slotMeasure, "zfs:tank", false))
	testutil.Receive(t, background.started, "the measurement's start")

	node.holdCommand(t.Context(), t.Context(), 7, nodeproto.Command{
		JobID: 2, AttemptToken: tokenOf(2), Priority: 1000, IsPlayDriven: true,
		Work: nodeproto.Work{Type: nodeproto.CommandPrepare, Prepare: &nodeproto.Prepare{
			Video: nodeproto.BytesFile{
				RootPath: root.Path(), Path: []byte("film.mkv"), PayloadHash: nodetest.PayloadHashHex(t, "a film"),
			},
			Profile: nodeproto.ProfileRemux, RenditionDir: "3/remux/0123456789abcdef",
		}},
	})
	testutil.Receive(t, preparer.started, "the play-driven prepare's start")
	node.work.mutex.Lock()
	prepare := node.work.find(tokenOf(2))
	device, isPlayDriven := prepare.device, prepare.isPlayDriven
	node.work.mutex.Unlock()
	if device != "zfs:tank" || !isPlayDriven {
		t.Errorf("the prepare reads %q, play-driven %v; want zfs:tank, its file's device, and play-driven",
			device, isPlayDriven)
	}
	states := map[int64]nodeproto.HeldState{}
	for _, held := range node.held() {
		states[held.JobID] = held.State
	}
	if states[1] != nodeproto.HeldPaused || states[2] != nodeproto.HeldRunning {
		t.Errorf("the status holds %v; want the measurement paused and the prepare running", states)
	}
	close(preparer.release)
	close(background.release)
}
