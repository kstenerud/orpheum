// SPDX-License-Identifier: AGPL-3.0-or-later

package agent

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
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
	if report.State != nodeproto.PrepareStalled || report.Stall == nil ||
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
