// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/storetest"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// queuedJobs keeps the jobs the scans queue.
type queuedJobs struct{ requests []jobs.Request }

func (queued *queuedJobs) Enqueue(_ context.Context, job jobs.Request) (bool, error) {
	queued.requests = append(queued.requests, job)
	return true, nil
}

// last is the last job queued of kind, as a runner hands it to its handler.
func (queued *queuedJobs) last(t *testing.T, kind jobs.Kind) jobs.Job {
	t.Helper()
	for _, request := range slices.Backward(queued.requests) {
		if request.Kind == kind {
			return jobs.Job{Kind: kind, Target: request.Target, Priority: request.Priority}
		}
	}
	t.Fatalf("no %s job was queued; queued %+v", kind, queued.requests)
	return jobs.Job{}
}

// count is how many jobs of kind were queued.
func (queued *queuedJobs) count(kind jobs.Kind) int {
	count := 0
	for _, request := range queued.requests {
		if request.Kind == kind {
			count++
		}
	}
	return count
}

// sentWork keeps the work given to nodes.
type sentWork struct{ commands []nodeproto.Command }

func (sent *sentWork) Assign(_ context.Context, _ int64, _ jobs.Job, command nodeproto.Command) error {
	sent.commands = append(sent.commands, command)
	return nil
}

// scanBench is the scans over a harness's library, with the jobs they queue and the work they give.
type scanBench struct {
	*harness
	cache *store.Cache
	scans *Scans
	jobs  *queuedJobs
	sent  *sentWork
}

func newScanBench(t *testing.T) *scanBench {
	t.Helper()
	world := newHarness(t)
	bench := &scanBench{harness: world, cache: storetest.OpenCache(t), jobs: &queuedJobs{}, sent: &sentWork{}}
	bench.scans = bench.restart()
	return bench
}

// restart is the scans as a core that has just started builds them, over the same cache.db.
func (bench *scanBench) restart() *Scans {
	return NewScans(ScansParts{
		Cache: bench.cache, Ingester: bench.ingester, Jobs: bench.jobs, Assigner: bench.sent,
		Settings: bench.settings, Clock: bench.clock, Logger: slog.New(slog.DiscardHandler),
	})
}

// list runs the last scan job queued and answers it with the listing of records.
func (bench *scanBench) list(records ...nodeproto.ScanRecord) {
	bench.t.Helper()
	ctx, job := bench.t.Context(), bench.jobs.last(bench.t, jobs.KindScan)
	bench.clock.Advance(time.Minute)
	if err := bench.scans.HandleScan(ctx, job); err != nil {
		bench.t.Fatal(err)
	}
	if sent := bench.sent.commands[len(bench.sent.commands)-1]; sent.List == nil || sent.List.RootPath != root {
		bench.t.Fatalf("the scan job sent %+v, want a listing of %s", sent, root)
	}
	listing := append([]nodeproto.ScanRecord{{Type: nodeproto.RecordScanBegin, ScanBegin: &nodeproto.ScanBegin{
		RootPath: root, FilesystemDevice: 7, IsMountPoint: true,
	}}}, records...)
	listing = append(listing, nodeproto.ScanRecord{
		Type: nodeproto.RecordScanComplete, ScanComplete: &nodeproto.ScanComplete{RootPath: root},
	})
	err := bench.scans.ScanAssignee(nil).Answer(ctx, job, nodes.Attempt{NodeID: bench.nodeID},
		nodeproto.Result{Listing: &nodeproto.Listing{Records: listing}})
	if err != nil {
		bench.t.Fatal(err)
	}
}

// examine runs the last examine job queued, and returns the files it asked the node to read.
func (bench *scanBench) examine() (jobs.Job, []nodeproto.ExamineFile) {
	bench.t.Helper()
	job := bench.jobs.last(bench.t, jobs.KindExamine)
	if err := bench.scans.HandleExamine(bench.t.Context(), job); err != nil {
		bench.t.Fatal(err)
	}
	sent := bench.sent.commands[len(bench.sent.commands)-1]
	if sent.Examine == nil || sent.Examine.RootPath != root {
		bench.t.Fatalf("the examine job sent %+v, want an examining under %s", sent, root)
	}
	return job, sent.Examine.Files
}

// isScanning is whether a scan of the root is in progress.
func (bench *scanBench) isScanning() bool {
	bench.t.Helper()
	scans, err := bench.cache.Read().ListScans(bench.t.Context())
	if err != nil {
		bench.t.Fatal(err)
	}
	return len(scans) > 0
}

// A scan lists its root, has the node read only what is new or changed, and completes once that is
// read: what was not seen is removed then, and not before (D39, D240).
func TestScans_AScanListsReadsWhatIsNewAndCompletesOnceItIsRead(t *testing.T) {
	t.Parallel()
	bench := newScanBench(t)
	bench.scan([]nodeproto.ScanRecord{observedRecord("Gone.mkv", "gone")}, true)
	ctx := t.Context()
	if err := bench.scans.Request(ctx, bench.nodeID, root, jobs.PriorityPersonsRequest); err != nil {
		t.Fatal(err)
	}
	bench.list(listedDirectory("Wing"), listedFile("Wing/Wing.mkv", 5, 1), listedFile("Wing/Wing.nfo", 8, 1))
	job, files := bench.examine()
	if len(files) != 2 || files[0].Path.Text != "Wing/Wing.mkv" || files[1].Kind != nodeproto.ExamineNFO {
		t.Fatalf("the node was asked to read %+v, want the film and its NFO", files)
	}
	if _, isLive := bench.liveFilesByPath()[root+"/Gone.mkv"]; !isLive {
		t.Error("a file was removed before the scan completed")
	}
	err := bench.scans.ExamineAssignee(nil).Answer(ctx, job, nodes.Attempt{NodeID: bench.nodeID},
		nodeproto.Result{Examined: &nodeproto.Examined{Records: []nodeproto.ScanRecord{
			observedRecord("Wing/Wing.mkv", "alien"),
			sidecarRecord("Wing/Wing.nfo", nodeproto.SidecarNFO, "<movie/>"),
		}}})
	if err != nil {
		t.Fatal(err)
	}
	live := bench.liveFilesByPath()
	if _, isLive := live[root+"/Gone.mkv"]; isLive {
		t.Error("the file the completed scan did not see is still live")
	}
	if alien := live[root+"/Wing/Wing.mkv"]; alien.PayloadHash == nil || live[root+"/Wing/Wing.nfo"].Filing !=
		FilingAttached {
		t.Errorf("the files read are %+v, want the film and its attached NFO", live)
	}
	state, err := bench.database.Read().GetRootState(ctx, rootStateParams(bench.nodeID))
	if err != nil || state.LastScanOutcome != "complete" {
		t.Errorf("the root's state is %+v (%v), want its scan complete", state, err)
	}
	if bench.isScanning() {
		t.Error("the scan is still in progress after its last batch was read")
	}
}

// A scan asked for while one of the root runs starts none, however many ask; one more runs after
// it (D236 §3).
// A scan's listing stops at the errors scan_root_error_limit says, which the node is told with it
// (D40; the constants sweep, 2026-10-21).
func TestScans_AListingSaysTheErrorLimitTheSettingSays(t *testing.T) {
	t.Parallel()
	bench := newScanBench(t)
	settingstest.Set(t, bench.settings, settings.ScanRootErrorLimit, "25")
	if err := bench.scans.Request(t.Context(), bench.nodeID, root, jobs.PriorityPersonsRequest); err != nil {
		t.Fatal(err)
	}
	bench.list()
	if sent := bench.sent.commands[len(bench.sent.commands)-1]; sent.List.ErrorLimit != 25 {
		t.Errorf("the listing's error limit is %d; want the setting's 25", sent.List.ErrorLimit)
	}
}

func TestScans_AScanAskedForWhileOneRunsRunsOnceMoreAfterIt(t *testing.T) {
	t.Parallel()
	bench := newScanBench(t)
	ctx := t.Context()
	for range 3 {
		if err := bench.scans.Request(ctx, bench.nodeID, root, jobs.PriorityPersonsRequest); err != nil {
			t.Fatal(err)
		}
	}
	if count := bench.jobs.count(jobs.KindScan); count != 1 {
		t.Fatalf("three requests queued %d scans, want one", count)
	}
	first := bench.jobs.last(t, jobs.KindScan)
	bench.list() // an empty root: nothing to read, so the scan completes at once
	if count := bench.jobs.count(jobs.KindScan); count != 2 {
		t.Fatalf("after the scan, %d scans were queued, want the one more asked for", count)
	}
	if again := bench.jobs.last(t, jobs.KindScan); again.ScanID == first.ScanID {
		t.Errorf("the scan asked for again is scan %d, the one that ended", again.ScanID)
	}
	bench.list()
	if count := bench.jobs.count(jobs.KindScan); count != 2 || bench.isScanning() {
		t.Errorf("%d scans queued, and one in progress %v; want no third", count, bench.isScanning())
	}
}

// A batch the node could not read is given up: its files are ones the scan could not read, filed
// errored and not removed (D40), and the scan completes.
func TestScans_ABatchGivenUpRemovesNoneOfItsFiles(t *testing.T) {
	t.Parallel()
	bench := newScanBench(t)
	bench.scan([]nodeproto.ScanRecord{observedRecord("Metropolis.mkv", "heat")}, true)
	ctx := t.Context()
	if err := bench.scans.Request(ctx, bench.nodeID, root, jobs.PriorityBackground); err != nil {
		t.Fatal(err)
	}
	bench.list(listedFile("Metropolis.mkv", int64(len("heat")), 2))
	job, _ := bench.examine()
	if err := bench.scans.RecordExamineFailure(ctx, job, "its machine did not say how it ended"); err != nil {
		t.Fatal(err)
	}
	heat, isLive := bench.liveFilesByPath()[root+"/Metropolis.mkv"]
	if !isLive || heat.Filing != FilingErrored {
		t.Errorf("Metropolis.mkv, which could not be read, is %+v (live %v), want live and errored", heat, isLive)
	}
	if bench.isScanning() {
		t.Error("the scan did not complete after its last batch was given up")
	}
}

// A scan whose listing the core took before it stopped starts again: what the ingest compared is
// lost with it. One not yet listed is queued again.
func TestScans_AScanTheCoreStoppedDuringStartsAgain(t *testing.T) {
	t.Parallel()
	bench := newScanBench(t)
	ctx := t.Context()
	if err := bench.scans.Request(ctx, bench.nodeID, root, jobs.PriorityBackground); err != nil {
		t.Fatal(err)
	}
	listed := bench.jobs.last(t, jobs.KindScan)
	bench.list(listedFile("Wing.mkv", 5, 1))
	bench.scans = bench.restart()
	if err := bench.scans.Resume(ctx); err != nil {
		t.Fatal(err)
	}
	again := bench.jobs.last(t, jobs.KindScan)
	if again.ScanID == listed.ScanID {
		t.Fatalf("after a restart the scan job is scan %d's, the one the core lost", again.ScanID)
	}
	if _, err := store.OrNotFound(bench.cache.Read().GetScan(ctx, listed.ScanID)); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("the lost scan is still in progress: %v", err)
	}
	stale := bench.jobs.last(t, jobs.KindExamine)
	if err := bench.scans.HandleExamine(ctx, stale); err != nil || len(bench.sent.commands) != 1 {
		t.Errorf("the lost scan's batch ran (%v), sending %d commands; want nothing sent", err,
			len(bench.sent.commands))
	}
	bench.scans = bench.restart()
	if err := bench.scans.Resume(ctx); err != nil {
		t.Fatal(err)
	}
	if requeued := bench.jobs.last(t, jobs.KindScan); requeued.ScanID != again.ScanID {
		t.Errorf("a scan not yet listed was queued as scan %d, want its own %d again", requeued.ScanID, again.ScanID)
	}
}

// A listing refused, a root the node was not given, fails its job; a scan whose listing fails is given
// up, removing nothing.
func TestScans_AScanWhoseListingFailsIsGivenUp(t *testing.T) {
	t.Parallel()
	bench := newScanBench(t)
	ctx := t.Context()
	if err := bench.scans.Request(ctx, bench.nodeID, root, jobs.PriorityBackground); err != nil {
		t.Fatal(err)
	}
	job := bench.jobs.last(t, jobs.KindScan)
	err := bench.scans.ScanAssignee(nil).Answer(ctx, job, nodes.Attempt{NodeID: bench.nodeID},
		nodeproto.Result{Listing: &nodeproto.Listing{Refusal: "this node was not given the root"}})
	if err == nil || !strings.Contains(err.Error(), "refused scan") {
		t.Errorf("a refused listing = %v, want the job failed with the refusal", err)
	}
	if err := bench.scans.RecordScanFailure(ctx, job, "refused"); err != nil {
		t.Fatal(err)
	}
	if bench.isScanning() {
		t.Error("a scan whose listing failed is still in progress")
	}
}

// A listing whose reading stalled is the tool's failure, not the root's contents: none of its records
// are taken, the job is tried again at no cost the first time, and fails the second, its disk
// unreadable (D233 §6, §10).
func TestScans_AStalledListingIsTriedAgainAndNeverTakenAsTheRootsContents(t *testing.T) {
	t.Parallel()
	bench := newScanBench(t)
	ctx := t.Context()
	if err := bench.scans.Request(ctx, bench.nodeID, root, jobs.PriorityBackground); err != nil {
		t.Fatal(err)
	}
	job := bench.jobs.last(t, jobs.KindScan)
	stalled := nodeproto.Result{Listing: &nodeproto.Listing{Stall: &nodeproto.Stall{
		Cause: nodeproto.StallStoodStill, Condition: nodeproto.ConditionWaitingOnDisk, IsLeftRunning: true,
		Detail: "a listing of /media did not move for 5m0s",
	}}}
	err := bench.scans.ScanAssignee(nil).Answer(ctx, job, nodes.Attempt{NodeID: bench.nodeID}, stalled)
	if err == nil || !strings.HasPrefix(err.Error(), "node ") || !strings.Contains(err.Error(), "stalled: ") ||
		!strings.Contains(err.Error(), "waiting_on_disk") {
		t.Errorf("a first stalled listing = %v; want it tried again as a stall, saying it waited on the disk", err)
	}
	if !bench.isScanning() {
		t.Error("a scan whose listing stalled once is no longer in progress; want it listed again")
	}
	job.PreviousAttemptToolFailed = true
	err = bench.scans.ScanAssignee(nil).Answer(ctx, job, nodes.Attempt{NodeID: bench.nodeID}, stalled)
	if err == nil || !strings.Contains(err.Error(), nodes.UnreadableOnDisk) {
		t.Errorf("a second stalled listing = %v; want the job's own failure", err)
	}
}

// Work that stalls waiting on a root's disk makes the root unavailable from then, so what is under it
// is said to be so, and asks for a scan of it; the scan that reads it brings it back (D233 §10).
func TestScans_AStallOnARootsDiskMakesItUnavailableUntilAScanReadsIt(t *testing.T) {
	t.Parallel()
	bench := newScanBench(t)
	bench.scan([]nodeproto.ScanRecord{observedRecord("Wing.mkv", "alien")}, true)
	ctx := t.Context()
	queuedBefore := len(bench.jobs.requests)
	stalledAt := bench.clock.Now()
	since, err := bench.scans.MarkRootStalled(ctx, bench.nodeID, root)
	if err != nil || !since.Equal(stalledAt) {
		t.Fatalf("MarkRootStalled = %v, %v; want unavailable since now, %v", since, err, stalledAt)
	}
	missing, isMissing, err := UnavailableUnder(ctx, bench.database.Read(), bench.nodeID, []byte(root+"/Wing.mkv"))
	if err != nil || !isMissing || missing.Path != root || !missing.Since.Equal(stalledAt) {
		t.Errorf("a file under the stalled root: %+v, %t, %v; want it under the root unavailable since the stall",
			missing, isMissing, err)
	}
	if len(bench.jobs.requests) != queuedBefore+1 || bench.jobs.requests[queuedBefore].Kind != jobs.KindScan {
		t.Fatal("no scan of the stalled root was asked for")
	}
	bench.clock.Advance(time.Minute)
	if again, err := bench.scans.MarkRootStalled(ctx, bench.nodeID, root); err != nil || !again.Equal(stalledAt) {
		t.Errorf("a second stall = %v, %v; want it unavailable since the first, %v", again, err, stalledAt)
	}
	bench.list(listedFile("Wing.mkv", 5, 1))
	if _, isMissing, err := UnavailableUnder(ctx, bench.database.Read(), bench.nodeID,
		[]byte(root+"/Wing.mkv")); err != nil || isMissing {
		t.Errorf("after a scan read the root, a file under it is missing: %t, %v; want it back", isMissing, err)
	}
}

// A scan's listing going to its node is logged with how long it waited in the core's queue: where a
// root's minute to come back went was not in the log (staging, 2026-10-21).
func TestScans_AListingGoingToItsNodeSaysHowLongItWaited(t *testing.T) {
	t.Parallel()
	bench := newScanBench(t)
	logged := &testutil.Log{}
	bench.scans = NewScans(ScansParts{
		Cache: bench.cache, Ingester: bench.ingester, Jobs: bench.jobs, Assigner: bench.sent,
		Settings: bench.settings, Clock: bench.clock, Logger: slog.New(slog.NewTextHandler(logged, nil)),
	})
	ctx := t.Context()
	if err := bench.scans.Request(ctx, bench.nodeID, root, jobs.PriorityBackground); err != nil {
		t.Fatal(err)
	}
	bench.clock.Advance(90 * time.Second)
	if err := bench.scans.HandleScan(ctx, bench.jobs.last(t, jobs.KindScan)); err != nil {
		t.Fatal(err)
	}
	logged.AwaitLine(t, "a scan's listing goes to its node", "rootPath="+root, "waitedMs=90000")
}

// unavailableRootBench is a scan bench whose root was read whole and is then unavailable, with no
// scan of it in progress, which a request would only merge into; found are the paths found since.
func unavailableRootBench(t *testing.T) (bench *scanBench, found *[]string, isUnavailable func() bool) {
	t.Helper()
	bench = newScanBench(t)
	found = &[]string{}
	bench.ingester.SetOnPathsFound(func(_ context.Context, _ int64, paths []string) {
		*found = append(*found, paths...)
	})
	bench.scan([]nodeproto.ScanRecord{observedRecord("Wing.mkv", "alien")}, true)
	if err := bench.scans.MarkRootNotNamed(t.Context(), bench.nodeID, root); err != nil {
		t.Fatal(err)
	}
	*found = nil
	return bench, found, func() bool {
		t.Helper()
		_, isMissing, err := UnavailableUnder(t.Context(), bench.database.Read(), bench.nodeID,
			[]byte(root+"/Wing.mkv"))
		if err != nil {
			t.Fatal(err)
		}
		return isMissing
	}
}

// An unavailable root comes back at once when its disk answers again as the one its last complete
// scan read: what is under it is no longer missing, the work parked on it resumes, and a scan reads
// what changed meanwhile in the background. A share back took a minute behind the listing of every
// root on its disk (staging, 2026-10-21).
func TestScans_AnUnavailableRootWhoseDiskAnswersAgainAsItWasIsBroughtBackAtOnce(t *testing.T) {
	t.Parallel()
	bench, found, isUnavailable := unavailableRootBench(t)
	ctx := t.Context()
	queuedBefore := len(bench.jobs.requests)
	isBack, err := bench.scans.BringBackRoot(ctx, bench.nodeID, root, 7, true)
	if err != nil || !isBack || isUnavailable() || !slices.Equal(*found, []string{root}) {
		t.Errorf("brought back %t, %v, found %q; want it available, its work resumed", isBack, err, *found)
	}
	asked := bench.jobs.requests[queuedBefore:]
	if len(asked) != 1 || asked[0].Priority != jobs.PriorityBackground {
		t.Errorf("scans asked for after it came back: %+v; want one in the background", asked)
	}
	if isBack, err := bench.scans.BringBackRoot(ctx, bench.nodeID, root, 7, true); err != nil || isBack {
		t.Errorf("a root available already: brought back %t, %v; want nothing done", isBack, err)
	}
}

// A root whose disk answers on another device than its last complete scan read, or no longer as a
// mount point, is the empty directory an unmounted disk leaves, and stays unavailable (D208).
func TestScans_AnUnavailableRootOnAnotherDiskIsNotBroughtBack(t *testing.T) {
	t.Parallel()
	bench, found, isUnavailable := unavailableRootBench(t)
	for _, other := range []struct {
		device       uint64
		isMountPoint bool
	}{{8, true}, {7, false}} {
		isBack, err := bench.scans.BringBackRoot(t.Context(), bench.nodeID, root, other.device, other.isMountPoint)
		if err != nil || isBack || !isUnavailable() || len(*found) != 0 {
			t.Errorf("on device %d, mount point %t: brought back %t, %v, found %q; want it left unavailable",
				other.device, other.isMountPoint, isBack, err, *found)
		}
	}
}

// A library root its node's hello no longer names is unavailable from then, nothing under it
// removed, until a scan reads it again (D241 §2).
func TestScans_ARootTheHelloNoLongerNamesIsUnavailableUntilAScanReadsIt(t *testing.T) {
	t.Parallel()
	bench := newScanBench(t)
	bench.scan([]nodeproto.ScanRecord{observedRecord("Wing.mkv", "alien")}, true)
	ctx := t.Context()
	goneAt := bench.clock.Now()
	if err := bench.scans.MarkRootNotNamed(ctx, bench.nodeID, root); err != nil {
		t.Fatal(err)
	}
	missing, isMissing, err := UnavailableUnder(ctx, bench.database.Read(), bench.nodeID, []byte(root+"/Wing.mkv"))
	if err != nil || !isMissing || missing.Path != root || !missing.Since.Equal(goneAt) {
		t.Errorf("a file under the root no longer named: %+v, %t, %v; want it under the root unavailable since then",
			missing, isMissing, err)
	}
	if _, isLive := bench.liveFilesByPath()[root+"/Wing.mkv"]; !isLive {
		t.Error("a file under a root no longer named was removed; want it kept")
	}
}

// inProgressOf is the scan of the bench's root in progress, as InProgress shows it, and whether
// there is one.
func (bench *scanBench) inProgressOf() (ScanProgress, bool) {
	bench.t.Helper()
	progress, err := bench.scans.InProgress(bench.t.Context())
	if err != nil {
		bench.t.Fatal(err)
	}
	scan, isScanning := progress[ScanRoot{NodeID: bench.nodeID, RootPath: root}]
	return scan, isScanning
}

// A scan in progress shows how far it is, which the libraries page shows an admin (D236 §2):
// not listed yet; then the files it listed, and how many of them there are to read and have been
// read; and whether another scan was asked for while it ran, which starts, fresh, when it ends.
func TestScans_AScanInProgressShowsHowFarItIs(t *testing.T) {
	t.Parallel()
	bench := newScanBench(t)
	ctx := t.Context()
	for range 2 {
		if err := bench.scans.Request(ctx, bench.nodeID, root, jobs.PriorityPersonsRequest); err != nil {
			t.Fatal(err)
		}
	}
	if scan, isScanning := bench.inProgressOf(); !isScanning || scan.IsListed || !scan.IsAnotherScanQueued {
		t.Errorf("before its listing, the scan shows %+v (%v); want unlisted, with another asked for", scan,
			isScanning)
	}
	bench.list(listedDirectory("Wing"), listedFile("Wing/Wing.mkv", 5, 1))
	scan, isScanning := bench.inProgressOf()
	if !isScanning || !scan.IsListed || scan.ListedCount != 1 || scan.ToExamineCount != 1 || scan.ExaminedCount != 0 {
		t.Errorf("once listed, the scan shows %+v (%v); want its one file listed and to read", scan, isScanning)
	}
	job, _ := bench.examine()
	err := bench.scans.ExamineAssignee(nil).Answer(ctx, job, nodes.Attempt{NodeID: bench.nodeID},
		nodeproto.Result{Examined: &nodeproto.Examined{Records: []nodeproto.ScanRecord{
			observedRecord("Wing/Wing.mkv", "alien"),
		}}})
	if err != nil {
		t.Fatal(err)
	}
	if scan, isScanning := bench.inProgressOf(); !isScanning || scan.IsListed || scan.IsAnotherScanQueued {
		t.Errorf("once it ended, the root's scan shows %+v (%v); want the one asked for again, fresh", scan,
			isScanning)
	}
}

// rootsOnQuickPass are the bench's node's roots on their quick pass.
func (bench *scanBench) rootsOnQuickPass() []string {
	bench.t.Helper()
	roots, err := bench.scans.RootsOnQuickPass(bench.t.Context(), bench.nodeID)
	if err != nil {
		bench.t.Fatal(err)
	}
	return roots
}

// A root is on its scan's quick pass while the scan lists it and reads the files it found new or
// changed, and off it once they are read, so a background read of whole films on its disk waits for
// that and no longer (D255). Another node's roots are not its node's.
func TestScans_ARootIsOnItsQuickPassUntilItsNewFilesAreRead(t *testing.T) {
	t.Parallel()
	bench := newScanBench(t)
	ctx := t.Context()
	if err := bench.scans.Request(ctx, bench.nodeID, root, jobs.PriorityBackground); err != nil {
		t.Fatal(err)
	}
	if roots := bench.rootsOnQuickPass(); !slices.Equal(roots, []string{root}) {
		t.Errorf("before its listing, the roots on their quick pass = %v; want %s", roots, root)
	}
	if roots, err := bench.scans.RootsOnQuickPass(ctx, bench.nodeID+1); err != nil || len(roots) != 0 {
		t.Errorf("another node's roots on their quick pass = %v (%v); want none", roots, err)
	}
	bench.list(listedDirectory("Wing"), listedFile("Wing/Wing.mkv", 5, 1))
	if roots := bench.rootsOnQuickPass(); !slices.Equal(roots, []string{root}) {
		t.Errorf("listed, its file not read, the roots on their quick pass = %v; want %s", roots, root)
	}
	job, _ := bench.examine()
	err := bench.scans.ExamineAssignee(nil).Answer(ctx, job, nodes.Attempt{NodeID: bench.nodeID},
		nodeproto.Result{Examined: &nodeproto.Examined{Records: []nodeproto.ScanRecord{
			observedRecord("Wing/Wing.mkv", "alien"),
		}}})
	if err != nil {
		t.Fatal(err)
	}
	if roots := bench.rootsOnQuickPass(); len(roots) != 0 {
		t.Errorf("once its file was read, the roots on their quick pass = %v; want none", roots)
	}
}

// A person who asks to play a film still being read has the batch that reads it raised to a
// person's priority, so it is read before what else waits; with no scan holding it, a scan of its
// folder is asked for at that priority (D304 §4).
func TestReadNow_RaisesTheBatchThatReadsAFileOrScansItsFolder(t *testing.T) {
	t.Parallel()
	bench := newScanBench(t)
	ctx := t.Context()
	if err := bench.scans.Request(ctx, bench.nodeID, root, jobs.PriorityBackground); err != nil {
		t.Fatal(err)
	}
	bench.list(listedDirectory("Wing"), listedFile("Wing/Wing.mkv", 5, 1),
		listedDirectory("Metropolis"), listedFile("Metropolis/Metropolis.mkv", 5, 1))
	heat := bench.liveFilesByPath()[root+"/Metropolis/Metropolis.mkv"]
	if err := bench.scans.ReadNow(ctx, heat.LibraryID, bench.nodeID, heat.ReportedPathRaw); err != nil {
		t.Fatal(err)
	}
	raised := bench.jobs.last(t, jobs.KindExamine)
	batch, err := bench.cache.Read().GetScanBatch(ctx, raised.ScanBatchID)
	if err != nil {
		t.Fatal(err)
	}
	if raised.Priority != jobs.PriorityPersonsRequest || !strings.Contains(batch.FilesJSON, "Metropolis/Metropolis.mkv") {
		t.Errorf("raised a batch of %s at priority %d; want Metropolis's batch at a person's", batch.FilesJSON,
			raised.Priority)
	}

	idle := newScanBench(t) // no scan in progress holds the file
	stray := []byte(root + "/Elsewhere/Nosferatu.mkv")
	if err = idle.scans.ReadNow(ctx, heat.LibraryID, idle.nodeID, stray); err != nil {
		t.Fatal(err)
	}
	scans, err := idle.cache.Read().ListScans(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(scans) != 1 || scans[0].ScopePath != "Elsewhere" || scans[0].Priority != jobs.PriorityPersonsRequest {
		t.Errorf("a file no scan holds asked for the scans %+v; want one of its folder, at a person's priority", scans)
	}
}
