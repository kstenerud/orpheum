// SPDX-License-Identifier: AGPL-3.0-or-later

package agent

import (
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/diskwatch"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/platform/diskspace"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// A file to read that does not open within the open stall window, its disk not answering as a
// network share whose host is gone does not, is answered stalled on its disk, waiting on that file,
// and listed as stuck until the open returns (D233 §10; staging, 2026-10-21).
func TestAgent_AFileThatDoesNotOpenIsAnsweredStalledOnItsDiskAndListedAsStuck(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	var clockSource *clocktest.Fake
	release := make(chan struct{})
	root, tick := startAgentWithWorkers(t, socket, Workers{ChunkMaker: fakeChunkMaker{}, OpenStallWindow: time.Minute},
		func(node *Agent) {
			fake, isFake := node.clock.(*clocktest.Fake)
			if !isFake {
				t.Fatal("the test agent's clock is not a fake")
			}
			clockSource = fake
			node.openFile = func(map[string]*confine.MediaRoot, nodeproto.BytesFile) (*os.File, fs.FileInfo, error) {
				select { // the share's host is gone, until the test brings it back
				case <-release:
				case <-t.Context().Done():
				}
				return nil, nil, errors.New("the share answered at last")
			}
		})
	film := filepath.Join(root.Path(), "film.mkv")
	tick <- time.Time{}
	testutil.Receive(t, core.statuses, "the first status")
	core.sendMakeChunks(1, nodeproto.MakeChunks{
		Video: nodeproto.BytesFile{
			RootPath: root.Path(), Path: []byte("film.mkv"), PayloadHash: strings.Repeat("ab", 32),
		},
		StoreDir: "1/chunks-picture/0123456789abcdef", BoundsMs: []int64{0, 2000},
		Stream: nodeproto.ChunkStream{Kind: nodeproto.StreamPicture},
	})
	var report nodeproto.MakeChunksReport
	for answered := false; !answered; {
		select {
		case result := <-core.chunkRuns:
			report, answered = *result.MakeChunks, true
		case <-t.Context().Done():
			t.Fatal("the run of a file that does not open was never answered")
		case <-time.After(5 * time.Millisecond):
			clockSource.AdvanceToNext() // the open's window, once it is armed
		}
	}
	if report.State != nodeproto.RunStalled || report.Stall == nil ||
		report.Stall.Condition != nodeproto.ConditionWaitingOnDisk || !report.Stall.IsLeftRunning ||
		report.Stall.WaitingOnPath != film {
		t.Fatalf("the run ended %+v (stall %+v); want stalled waiting on its disk at %s, left running", report,
			report.Stall, film)
	}
	stuck := statusUntil(t, core, tick, func(status nodeproto.Status) bool { return len(status.Stuck) > 0 }).Stuck
	if len(stuck) != 1 || stuck[0].Paths[0] != film {
		t.Errorf("the status's stuck = %+v; want the open of %s", stuck, film)
	}
	close(release) // the share answers again
	statusUntil(t, core, tick, func(status nodeproto.Status) bool { return len(status.Stuck) == 0 })
}

// A root holding a reading that has not returned is not read again: a listing of it is answered
// stalled on its disk at once, without listing, so a scan's retries add no reading stuck beside it,
// and the log says so (diskwatch; staging, 2026-10-21).
func TestAgent_AListingOfARootHoldingAStuckReadingIsAnsweredStalledAtOnce(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	gate := diskwatch.NewGate(clocktest.NewFake(time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)))
	logged := &testutil.Log{}
	root, tick := startAgentWithWorkers(t, socket, Workers{Scanner: fakeScanner{}, Disks: gate}, func(node *Agent) {
		node.logger = slog.New(slog.NewTextHandler(logged, nil))
	})
	held, isHeld := gate.Hold(root.Path(), root.Path()+"/film.mkv") // a reading of it that has not returned
	if !isHeld {
		t.Fatal("the root could not be held")
	}
	t.Cleanup(held)
	tick <- time.Time{}
	testutil.Receive(t, core.statuses, "the first status")
	core.commands <- nodeproto.Command{CommandSeq: 1, JobID: 1, Work: nodeproto.Work{
		Type: nodeproto.CommandList, List: &nodeproto.List{RootPath: root.Path(), Path: "."},
	}}
	listed := testutil.Receive(t, core.scans, "the listing's answer")
	if listed.Listing == nil || listed.Listing.Stall == nil ||
		listed.Listing.Stall.Condition != nodeproto.ConditionWaitingOnDisk || len(listed.Listing.Records) != 0 {
		t.Errorf("the listing of a root holding a stuck reading = %+v; want it stalled on its disk, unlisted", listed)
	}
	logged.AwaitEvent(t, "jobNotTriedRootWaiting", "jobId=1")
}

// A disk whose space does not measure leaves the status sent, saying the disk is not answering,
// which the core makes its root unavailable by: the measure blocked the status, and the node fell
// silent (staging, 2026-10-21).
func TestAgent_ADiskThatDoesNotMeasureLeavesTheStatusSentSayingSo(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	var clockSource *clocktest.Fake
	root, tick := startAgentWithWorkers(t, socket, Workers{}, func(node *Agent) {
		fake, isFake := node.clock.(*clocktest.Fake)
		if !isFake {
			t.Fatal("the test agent's clock is not a fake")
		}
		clockSource = fake
		node.measureSpace = func(string) (diskspace.Space, error) {
			select { // the share's host is gone
			case <-release:
			case <-t.Context().Done():
			}
			return diskspace.Space{}, nil
		}
	})
	go func() { tick <- time.Time{} }()
	for {
		select {
		case status := <-core.statuses:
			if len(status.Disks) == 0 || status.Disks[0].Path != root.Path() ||
				!strings.Contains(status.Disks[0].Failure, "not answering") || !status.Disks[0].IsNotAnswering {
				t.Fatalf("the status's disks = %+v; want %s said not to be answering", status.Disks, root.Path())
			}
			return
		case <-t.Context().Done():
			t.Fatal("no status came while a disk did not measure")
		case <-time.After(5 * time.Millisecond):
			clockSource.AdvanceToNext() // the measure's window, once it is armed
		}
	}
}

// A file whose disk fails the read as a network share mounted to give up does is a stall on that
// disk, not a refusal tried again as though the file had changed (D233 §10).
func TestAgent_AFileItsDiskFailsIsAnsweredStalledOnItsDisk(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	root, tick := startAgentWithWorkers(t, socket, Workers{ChunkMaker: fakeChunkMaker{}}, func(node *Agent) {
		node.openFile = func(map[string]*confine.MediaRoot, nodeproto.BytesFile) (*os.File, fs.FileInfo, error) {
			return nil, nil, &fs.PathError{Op: "open", Path: "film.mkv", Err: syscall.EIO}
		}
	})
	tick <- time.Time{}
	testutil.Receive(t, core.statuses, "the first status")
	core.sendMakeChunks(1, nodeproto.MakeChunks{
		Video: nodeproto.BytesFile{
			RootPath: root.Path(), Path: []byte("film.mkv"), PayloadHash: strings.Repeat("ab", 32),
		},
		StoreDir: "1/chunks-picture/0123456789abcdef", BoundsMs: []int64{0, 2000},
		Stream: nodeproto.ChunkStream{Kind: nodeproto.StreamPicture},
	})
	report := receiveChunksReport(t, core)
	if report.State != nodeproto.RunStalled || report.Stall == nil ||
		report.Stall.Cause != nodeproto.StallDiskFailed || report.Stall.Condition != nodeproto.ConditionWaitingOnDisk {
		t.Errorf("the run ended %+v (stall %+v); want stalled, its disk failing the read", report, report.Stall)
	}
}
