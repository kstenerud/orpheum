// SPDX-License-Identifier: AGPL-3.0-or-later

package waits

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library/librarytest"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/core/readthrough"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/platform/querytally"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// mib is a mebibyte.
const mib = 1 << 20

// fakeNodes are two nodes that read at 10 MiB a second at the slowest, a read-through at a time.
type fakeNodes struct{}

func (fakeNodes) Resources(context.Context, int64) (nodes.Resources, bool, error) {
	return nodes.Resources{Capacity: nodeproto.Capacity{EncodeSlotCount: 1, RemuxSlotCount: 2}}, true, nil
}

func (fakeNodes) SlowestReadBytesPerSecond(int64) int64 { return 10 * mib }

// runningSince are when each job's node first said it runs.
type runningSince map[int64]time.Time

func (since runningSince) RunningSinceOf(jobID int64) (time.Time, bool) {
	at, isRunning := since[jobID]
	return at, isRunning
}

// timedReads keeps the timings the estimator registered.
type timedReads struct{ timings readthrough.Timings }

func (reads *timedReads) TimeReadsBy(timings readthrough.Timings) { reads.timings = timings }

// bench is an estimator over a library on node 1, with a node 2 beside it, and cache.db at its path.
type bench struct {
	estimator *Estimator
	database  *store.Database
	cachePath string
	clock     *clocktest.Fake
	running   runningSince
	reads     *timedReads
	// playing are the files played now.
	playing *[]int64
}

func newBench(t *testing.T) *bench {
	t.Helper()
	database := storetest.OpenMigrated(t)
	librarytest.NodeWithLibrary(t, database,
		librarytest.Library{Name: "Films", Kind: "movie", MetadataSource: "nfo", Root: "/m"})
	storetest.ExecRaw(t, database.Path(), `INSERT INTO node (id, name, enrolled_at) VALUES (2, 'attic', 1)`,
		`INSERT INTO library_root (library_id, node_id, root_path) VALUES (1, 2, '/m')`)
	cachePath := filepath.Join(t.TempDir(), "cache.db")
	cache, err := store.OpenCache(t.Context(), cachePath, storetest.OpenOptions())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cache.Close() }) //nolint:errcheck // the test is over
	now := clocktest.NewFake(time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC))
	running, reads, playing := runningSince{}, &timedReads{}, &[]int64{}
	estimator, err := NewEstimator(Parts{
		Database: database, Cache: cache, Attempts: running, Nodes: fakeNodes{}, ReadThroughs: reads,
		Playing: func() []int64 { return *playing }, Clock: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	return &bench{
		estimator: estimator, database: database, cachePath: cachePath, clock: now, running: running,
		reads: reads, playing: playing,
	}
}

// file shelves a file of sizeMiB, an hour and a half long, on a node.
func (bench *bench) file(t *testing.T, id, nodeID, sizeMiB int64) {
	t.Helper()
	librarytest.File(t, bench.database, librarytest.FileSpec{
		ID: id, LibraryID: 1, NodeID: nodeID, Path: fmt.Sprintf("/m/%d.mkv", id), SizeBytes: sizeMiB * mib,
		PayloadHash: []byte{byte(id)}, Filing: "unidentified", //nolint:gosec // a test's small id, under 256
	})
	librarytest.Probe(t, bench.database, librarytest.ProbeSpec{FileID: id, DurationMs: 90 * 60_000})
}

// readThrough queues a read-through job of a file, assigned or waiting, with args.
func (bench *bench) readThrough(t *testing.T, jobID, fileID, priority int64, state, args string) {
	t.Helper()
	storetest.ExecRaw(t, bench.cachePath, fmt.Sprintf(`INSERT INTO job (id, kind, file_id, args_json, priority,
	 state, runnable_at, created_at) VALUES (%d, 'read_through', %d, '%s', %d, '%s', 1, 1)`,
		jobID, fileID, args, priority, state))
}

// left is how long file 100's read-through will take, failing when it has none or none known.
func (bench *bench) left(t *testing.T) (time.Duration, bool) {
	t.Helper()
	const fileID = 100
	waits, err := bench.estimator.Of(t.Context(), fileID)
	if err != nil {
		t.Fatal(err)
	}
	if waits.ReadThrough == nil || !waits.ReadThrough.IsKnown {
		t.Fatalf("file %d's read-through wait is %+v; want one known", fileID, waits.ReadThrough)
	}
	return waits.ReadThrough.Left, waits.ReadThrough.IsRunning
}

// A read-through waiting adds the bytes ahead of it on its node, over the node's read-throughs at
// once, to its own: running work it would not bump and waiting work above it; a running one is
// what it has left (D324 §6).
func TestEstimator_AReadThroughWaitsForTheBytesAheadOfItOnItsNode(t *testing.T) {
	t.Parallel()
	bench := newBench(t)
	for _, file := range []struct{ id, node, mib int64 }{
		{100, 1, 200}, {101, 1, 100}, {102, 1, 300}, {103, 1, 400}, {104, 2, 500}, {105, 1, 600},
	} {
		bench.file(t, file.id, file.node, file.mib)
	}
	bench.readThrough(t, 1, 100, jobs.PriorityBackground, "queued", "{}")
	bench.readThrough(t, 2, 101, jobs.PriorityBackground, "assigned", "{}")
	bench.running[2] = bench.clock.Now().Add(-2 * time.Second) // 20 MiB of it read
	bench.readThrough(t, 3, 102, jobs.PriorityShownInterest, "queued", "{}")
	bench.readThrough(t, 4, 103, jobs.PriorityBackground, "queued", "{}") // newer than file 100's
	bench.readThrough(t, 5, 104, jobs.PriorityPlayNow, "queued", "{}")    // on node 2
	bench.readThrough(t, 6, 105, jobs.PriorityPlayNow, "queued", `{"isFromIndexOnly":true}`)
	// 80 MiB left of 101 and 300 of 102, then its own 200, at 10 MiB a second.
	if left, isRunning := bench.left(t); left != 58*time.Second || isRunning {
		t.Errorf("waiting in the background, file 100's read-through takes %v (running %t); want 58s", left,
			isRunning)
	}
	storetest.ExecRaw(t, bench.cachePath, `UPDATE job SET priority = 100 WHERE id = 1`)
	if left, _ := bench.left(t); left != 20*time.Second {
		t.Errorf("looked at, file 100's read-through takes %v; want 20s, the background bumped", left)
	}
	storetest.ExecRaw(t, bench.cachePath, `UPDATE job SET state = 'assigned' WHERE id = 1`)
	bench.running[1] = bench.clock.Now().Add(-5 * time.Second)
	if left, isRunning := bench.left(t); left != 15*time.Second || !isRunning {
		t.Errorf("running for 5s, file 100's read-through takes %v (running %t); want 15s", left, isRunning)
	}
	if waits, err := bench.estimator.Of(t.Context(), 103+1000); err != nil || waits.ReadThrough != nil {
		t.Errorf("a file with no read-through open waits %+v (%v); want nothing", waits.ReadThrough, err)
	}
}

// A node's read speed is the median of its latest five read-throughs, each its bytes over its time
// once it ran, timed as the read-throughs end; before any, its slowest read (D324 §6).
func TestEstimator_AReadThroughsNodeSpeedIsWhatItsLastReadsMeasured(t *testing.T) {
	t.Parallel()
	bench := newBench(t)
	bench.file(t, 100, 1, 600)
	bench.readThrough(t, 1, 100, jobs.PriorityBackground, "queued", "{}")
	if left, _ := bench.left(t); left != time.Minute {
		t.Errorf("before any read, file 100's read-through takes %v; want 1m0s at the slowest read", left)
	}
	bench.reads.timings.ReadThroughFinished(1, 1000*mib, time.Second) // the oldest, forgotten
	for _, seconds := range []int64{10, 3600, 30, 15, 12} {
		bench.reads.timings.ReadThroughFinished(1, 600*mib, time.Duration(seconds)*time.Second)
	}
	// 60, 1/6, 20, 40 and 50 MiB a second, the second held by an outage: 40 their median.
	if left, _ := bench.left(t); left != 15*time.Second {
		t.Errorf("after five reads, one held an hour, file 100's read-through takes %v; want 15s at their median",
			left)
	}
}

// A film played now, its read-through not yet raised to a play's priority, as the interest raiser does
// at its next tick, waits as if it were: ahead of the queue, with the background's running work
// bumped. It said "about 55 minutes" of a read that began 15 s later (VQ50, 2026-11-02).
func TestEstimator_AFilmPlayedWaitsAsTheRaiseWillRankIt(t *testing.T) {
	t.Parallel()
	bench := newBench(t)
	for _, file := range []struct{ id, mib int64 }{{100, 200}, {101, 100}, {102, 300}, {103, 400}} {
		bench.file(t, file.id, 1, file.mib)
	}
	bench.readThrough(t, 1, 103, jobs.PriorityPlayNow, "queued", "{}") // another film played, before it
	bench.readThrough(t, 2, 101, jobs.PriorityBackground, "assigned", "{}")
	bench.readThrough(t, 3, 102, jobs.PriorityShownInterest, "queued", "{}")
	bench.readThrough(t, 4, 100, jobs.PriorityBackground, "queued", "{}")
	*bench.playing = []int64{100}
	// 400 MiB of 103, then its own 200, at 10 MiB a second: 101 bumped, 102 below it.
	if left, _ := bench.left(t); left != time.Minute {
		t.Errorf("played, its job still in the background, file 100's read-through takes %v; want 1m0s", left)
	}
}

// An estimator built without one of its parts is refused.
func TestNewEstimator_RefusesPartsThatLeaveOneOut(t *testing.T) {
	t.Parallel()
	if _, err := NewEstimator(Parts{}); !errors.Is(err, ErrMissingPart) {
		t.Errorf("NewEstimator of no parts = %v; want ErrMissingPart", err)
	}
}

// A running read-through that says how far it is goes at its own pace, where its node's speed said
// it was done: a share slower than the node's slowest read left "less than a minute" for minutes
// (the owner's film on staging, 2026-10-21; D324 §6).
func TestEstimator_ARunningReadThatSaysHowFarItIsGoesAtItsOwnPace(t *testing.T) {
	t.Parallel()
	bench := newBench(t)
	bench.file(t, 100, 1, 200)
	bench.readThrough(t, 1, 100, jobs.PriorityPlayNow, "assigned", "{}")
	bench.running[1] = bench.clock.Now().Add(-3 * time.Minute) // at 10 MiB a second, done long ago
	storetest.ExecRaw(t, bench.cachePath, `UPDATE job SET progress_fraction = 0.75 WHERE id = 1`)
	if left, isRunning := bench.left(t); left != time.Minute || !isRunning {
		t.Errorf("three-quarters read in three minutes, file 100's read-through takes %v (running %t); want 1m0s",
			left, isRunning)
	}
}

// A file with no read-through open costs its waits the same few statements however many other
// files' read-throughs are open: after an import that is the whole unread library, polled by every
// viewer of a film's page (audit 2026-10-25, S5). Only a file with its own open reads the others'.
func TestEstimator_AFileWithNoReadThroughReadsNoOtherFilesRead(t *testing.T) {
	t.Parallel()
	bench := newBench(t)
	bench.file(t, 100, 1, 200)
	for id := int64(1); id <= 50; id++ {
		bench.file(t, 100+id, 1, 100)
		bench.readThrough(t, id, 100+id, jobs.PriorityBackground, "queued", "{}")
	}
	ctx, tally := querytally.New(t.Context())
	waits, err := bench.estimator.Of(ctx, 100)
	if err != nil || waits.ReadThrough != nil {
		t.Fatalf("file 100's waits = %+v, %v; want no read-through", waits, err)
	}
	ran := tally.String()
	if strings.Contains(ran, "ListOpenReadThroughs") || strings.Contains(ran, "ListFileSizes") {
		t.Errorf("the waits of a file with no read-through ran %s; want no other file's read", ran)
	}
}
