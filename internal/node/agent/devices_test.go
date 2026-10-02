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
	node.work.hold(t.Context(), background.onDevice(1, nodeproto.SlotMeasure, "zfs:tank", false))
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

// A paused prepare, which says nothing of itself while FFmpeg is stopped, is said paused at each
// status tick, so the core neither waits on it for a slot nor gives it up for silence (D240 §6).
func TestAgent_APausedPrepareIsSaidPausedAtEachTick(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	node := New(socket, nodeproto.Hello{Name: "local", Capacity: testCapacity}, nil, Workers{},
		clocktest.NewFake(time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)), slog.New(slog.DiscardHandler))
	blocking := newBlockingWork()
	prepare := blocking.onDevice(4, nodeproto.SlotRemux, "zfs:tank", false)
	prepare.saysHowFar = true
	node.work.hold(t.Context(), prepare)
	testutil.Receive(t, blocking.started, "the prepare's start")
	node.work.mutex.Lock()
	node.work.find(tokenOf(4)).pausing.Pause()
	node.work.mutex.Unlock()
	node.sayHeld(t.Context(), 7)
	if said := testutil.Receive(t, core.progress, "the tick's word"); said.JobID != 4 ||
		said.State != nodeproto.ProgressPaused {
		t.Errorf("the tick said %+v of the paused prepare, want job 4 paused", said)
	}
	close(blocking.release)
}

// A reprioritize command gives the attempt it names its new priority, play-driven no more (D234 §3).
func TestAgent_AReprioritizeCommandGivesTheAttemptItsNewPriority(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	preparer := releasedPreparer{started: make(chan struct{}), release: make(chan struct{}), stopped: &atomic.Bool{}}
	var node *Agent
	root, _ := startAgentWithWorkers(t, socket, Workers{Scanner: fakeScanner{}, Preparer: preparer},
		func(started *Agent) { node = started })
	if err := os.WriteFile(filepath.Join(root.Path(), "film.mkv"), []byte("a film"), 0o600); err != nil {
		t.Fatal(err)
	}
	core.commands <- nodeproto.Command{
		CommandSeq: 1, JobID: 5, Priority: 1000, IsPlayDriven: true,
		Work: nodeproto.Work{Type: nodeproto.CommandPrepare, Prepare: &nodeproto.Prepare{
			Video: nodeproto.BytesFile{
				RootPath: root.Path(), Path: []byte("film.mkv"), PayloadHash: nodetest.PayloadHashHex(t, "a film"),
			},
			Profile: nodeproto.ProfileRemux, RenditionDir: "3/remux/0123456789abcdef",
		}},
	}
	testutil.Receive(t, preparer.started, "the prepare's start")
	core.commands <- nodeproto.Command{
		CommandSeq: 2, JobID: 5, Work: nodeproto.Work{Type: nodeproto.CommandReprioritize},
	}
	var priority int64 = 1000
	isPlayDriven := true
	for range 100 {
		node.work.mutex.Lock()
		if work := node.work.find(tokenOf(5)); work != nil {
			priority, isPlayDriven = work.priority, work.isPlayDriven
		}
		node.work.mutex.Unlock()
		if !isPlayDriven {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if priority != 0 || isPlayDriven {
		t.Errorf("after the reprioritize the prepare is at %d, play-driven %v; want 0, not play-driven",
			priority, isPlayDriven)
	}
	close(preparer.release)
}
