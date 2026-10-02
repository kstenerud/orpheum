// SPDX-License-Identifier: GPL-3.0-or-later

package agent

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// Every kind of work says where it is (D234 §5): running when it starts, waiting at once when its
// slot is taken, and each again at every status tick, so the core hears the node still holds it.
func TestAgent_EveryKindOfWorkSaysWhereItIsWhenItStartsAndAtEachTick(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	artworker := blockingArtworker{holding: make(chan struct{}, 1), stopped: make(chan struct{}, 1)}
	_, tick := startAgentWithWorkers(t, socket, Workers{Scanner: fakeScanner{}, Artworker: artworker})
	held := func(seq int64) nodeproto.Command {
		return nodeproto.Command{
			CommandSeq: seq,
			JobID:      seq,
			Work: nodeproto.Work{Type: nodeproto.CommandEncodeArtwork, EncodeArtwork: oneImage(&nodeproto.Artwork{
				SourceImage: []byte("hold"), Shape: nodeproto.ShapePortrait,
				SizeNames: []nodeproto.ArtSizeName{nodeproto.PortraitS},
			})},
		}
	}
	core.commands <- held(1)
	testutil.Receive(t, artworker.holding, "job 1's re-encode starting")
	if running := nextProgress(t, core); running.JobID != 1 || running.AttemptToken != tokenOf(1) ||
		running.State != nodeproto.ProgressRunning {
		t.Fatalf("the first word = %+v, want job 1's attempt running", running)
	}
	core.commands <- held(2)
	if waiting := nextProgress(t, core); waiting.JobID != 2 || waiting.State != nodeproto.ProgressWaiting {
		t.Fatalf("the next word = %+v, want job 2 waiting for the artwork slot", waiting)
	}
	tick <- time.Time{}
	heard := map[int64]nodeproto.ProgressState{}
	for range 2 {
		progress := nextProgress(t, core)
		heard[progress.JobID] = progress.State
	}
	if heard[1] != nodeproto.ProgressRunning || heard[2] != nodeproto.ProgressWaiting {
		t.Errorf("at the tick the core heard %v, want job 1 running and job 2 waiting", heard)
	}
}

// receiveProgress hands the test the node's words on where its attempts are, dropping one the test
// has no room for: progress is best effort, as the core's is.
func (core *fakeCore) receiveProgress(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(writer http.ResponseWriter, request *http.Request) {
		var progress nodeproto.Progress
		if err := json.NewDecoder(request.Body).Decode(&progress); err != nil {
			if isMalformed(err) {
				t.Error(err)
			}
			return
		}
		select {
		case core.progress <- progress:
		default:
		}
		_, _ = writer.Write([]byte("{}")) //nolint:errcheck // a node that went away has no use for it
	}
}
