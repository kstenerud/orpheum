// SPDX-License-Identifier: AGPL-3.0-or-later

package agent

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/scan"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// The core lists a root, then asks for the files it needs read: the node answers each as a result
// of the job (D240 §2, §4). The listing's start and end are logged with how long each took.
func TestAgent_ListsARootAndExaminesTheFilesTheCoreAsksFor(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	logged := &testutil.Log{}
	root, _ := startAgentWithWorkers(t, socket, Workers{
		Scanner: fakeScanner{}, Artworker: fakeArtworker{}, Preparer: fakePreparer{}, Subtitler: fakeSubtitler{},
		ThroughReader: fakeThroughReader{}, ChunkMaker: fakeChunkMaker{},
	}, func(node *Agent) { node.logger = slog.New(slog.NewTextHandler(logged, nil)) })
	if err := os.WriteFile(filepath.Join(root.Path(), "Wing.mkv"), []byte("alien"), 0o600); err != nil {
		t.Fatal(err)
	}
	core.commands <- nodeproto.Command{CommandSeq: 1, JobID: 1, Work: nodeproto.Work{
		Type: nodeproto.CommandList, List: &nodeproto.List{RootPath: root.Path(), Path: "."},
	}}
	listed := testutil.Receive(t, core.scans, "the listing")
	if listed.JobID != 1 || listed.Listing == nil || !slices.ContainsFunc(listed.Listing.Records,
		func(record nodeproto.ScanRecord) bool {
			return record.Listed != nil && record.Listed.Path.Text == "Wing.mkv"
		}) {
		t.Errorf("job 1's answer = %+v, want a listing naming Wing.mkv", listed)
	}
	logged.AwaitLine(t, "a listing started", "jobId=1", "waitedMs=")
	logged.AwaitLine(t, "a listing ended", "jobId=1", "recordCount=", "tookMs=")
	core.commands <- nodeproto.Command{CommandSeq: 2, JobID: 2, Work: nodeproto.Work{
		Type: nodeproto.CommandExamine, Examine: &nodeproto.Examine{
			RootPath: root.Path(),
			Files:    []nodeproto.ExamineFile{{Path: nodeproto.Path{Text: "Wing.mkv"}, Kind: nodeproto.ExamineVideo}},
		},
	}}
	examined := testutil.Receive(t, core.scans, "the examining")
	if examined.JobID != 2 || examined.Examined == nil || len(examined.Examined.Records) != 1 ||
		examined.Examined.Records[0].Observed == nil {
		t.Errorf("job 2's answer = %+v, want Wing.mkv observed", examined)
	}
}

// A listing or an examining under a root the node was not given is refused, whatever the core asks
// (D107).
func TestAgent_RefusesToListOrExamineARootItWasNotGiven(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	startAgent(t, socket)
	core.commands <- nodeproto.Command{CommandSeq: 3, JobID: 3, Work: nodeproto.Work{
		Type: nodeproto.CommandList, List: &nodeproto.List{RootPath: "/etc", Path: "."},
	}}
	if refused := testutil.Receive(t, core.scans, "the refused listing"); refused.Listing == nil ||
		refused.Listing.Refusal == "" || len(refused.Listing.Records) != 0 {
		t.Errorf("listing /etc = %+v, want it refused: the node was not given it", refused.Listing)
	}
	core.commands <- nodeproto.Command{CommandSeq: 4, JobID: 4, Work: nodeproto.Work{
		Type: nodeproto.CommandExamine, Examine: &nodeproto.Examine{RootPath: "/etc", Files: []nodeproto.ExamineFile{
			{Path: nodeproto.Path{Text: "passwd"}, Kind: nodeproto.ExamineNFO},
		}},
	}}
	if refused := testutil.Receive(t, core.scans, "the refused examining"); refused.Examined == nil ||
		refused.Examined.Refusal == "" || len(refused.Examined.Records) != 0 {
		t.Errorf("examining under /etc = %+v, want it refused", refused.Examined)
	}
}

// List lists as the node's scanner does.
func (fakeScanner) List(ctx context.Context, root *confine.MediaRoot, command nodeproto.List) (
	[]nodeproto.ScanRecord, error,
) {
	return realScanner().List(ctx, root, command)
}

// Examine examines as the node's scanner does, with a prober that finds every video a Matroska.
func (fakeScanner) Examine(
	ctx context.Context, root *confine.MediaRoot, files []nodeproto.ExamineFile, between func() error,
) ([]nodeproto.ScanRecord, error) {
	return realScanner().Examine(ctx, root, files, between)
}

// realScanner is the node's scanner over no other roots, probing nothing.
func realScanner() *scan.Scanner {
	return scan.NewScanner(nil, matroskaProber{}, scan.Options{})
}

// matroskaProber finds every file a Matroska.
type matroskaProber struct{}

func (matroskaProber) Probe(context.Context, string) (nodeproto.Probe, error) {
	return nodeproto.Probe{Format: nodeproto.ProbeFormat{FormatNames: []string{"matroska"}}}, nil
}

// hangingScanner lists nothing until released, as a listing of a disk that does not answer, which
// no cancel stops: it says when it starts, and when it returns. The test's end releases it too.
type hangingScanner struct {
	fakeScanner
	started, release, returned chan struct{}
	testCtx                    context.Context //nolint:containedctx // the test's, which bounds the hang
}

func (scanner hangingScanner) List(context.Context, *confine.MediaRoot, nodeproto.List) (
	[]nodeproto.ScanRecord, error,
) {
	close(scanner.started)
	select {
	case <-scanner.release:
	case <-scanner.testCtx.Done():
	}
	close(scanner.returned)
	return nil, nil
}

// A listing whose reading does not move for the node's read window is answered as stalled on its
// disk, and its reading, which the node cannot stop, is listed as stuck until it returns; what it
// returns then is dropped (D233 §10).
func TestAgent_AListingThatStopsMovingIsAnsweredStalledAndListedAsStuck(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	scanner := hangingScanner{
		started: make(chan struct{}), release: make(chan struct{}), returned: make(chan struct{}),
		testCtx: t.Context(),
	}
	var clockSource *clocktest.Fake
	root, tick := startAgentWithWorkers(t, socket, Workers{Scanner: scanner, ReadStallWindow: time.Minute},
		func(node *Agent) {
			fake, isFake := node.clock.(*clocktest.Fake)
			if !isFake {
				t.Fatal("the test agent's clock is not a fake")
			}
			clockSource = fake
		})
	tick <- time.Time{}
	testutil.Receive(t, core.statuses, "the first status")
	core.commands <- nodeproto.Command{CommandSeq: 1, JobID: 1, Work: nodeproto.Work{
		Type: nodeproto.CommandList, List: &nodeproto.List{RootPath: root.Path(), Path: "."},
	}}
	testutil.Receive(t, scanner.started, "the listing's start")
	clockSource.Advance(time.Minute)
	tick <- time.Time{}
	stalled := testutil.Receive(t, core.scans, "the stalled listing's answer")
	if stalled.JobID != 1 || stalled.Listing == nil || stalled.Listing.Stall == nil ||
		stalled.Listing.Stall.Condition != nodeproto.ConditionWaitingOnDisk || !stalled.Listing.Stall.IsLeftRunning {
		t.Fatalf("job 1's answer = %+v; want a listing stalled on its disk, left running", stalled)
	}
	stuck := statusUntil(t, core, tick, func(status nodeproto.Status) bool { return len(status.Stuck) > 0 }).Stuck
	if len(stuck) != 1 || stuck[0].Tool != stuckReadTool || stuck[0].Paths[0] != root.Path() {
		t.Errorf("the status's stuck = %+v; want the listing's reading of %s", stuck, root.Path())
	}
	close(scanner.release)
	testutil.Receive(t, scanner.returned, "the listing's return")
	statusUntil(t, core, tick, func(status nodeproto.Status) bool { return len(status.Stuck) == 0 })
	select {
	case late := <-core.scans:
		t.Errorf("the stuck listing answered again once it returned: %+v", late)
	default:
	}
}

// statusUntil ticks until the node posts a status that holds, and returns it.
func statusUntil(
	t *testing.T, core *fakeCore, tick chan time.Time, holds func(nodeproto.Status) bool,
) nodeproto.Status {
	t.Helper()
	for range 100 {
		tick <- time.Time{}
		if status := testutil.Receive(t, core.statuses, "a status"); holds(status) {
			return status
		}
	}
	t.Fatal("no status held")
	return nodeproto.Status{}
}
