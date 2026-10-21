// SPDX-License-Identifier: AGPL-3.0-or-later

package waits

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library/librarytest"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/core/readthrough"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
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
}

func newBench(t *testing.T) *bench {
	t.Helper()
	database := storetest.OpenMigrated(t)
	librarytest.NodeWithLibrary(t, database, librarytest.Library{Name: "Films", Kind: "movies", MetadataSource: "nfo"})
	storetest.ExecRaw(t, database.Path(), `INSERT INTO node (id, name, enrolled_at) VALUES (2, 'attic', 1)`)
	cachePath := filepath.Join(t.TempDir(), "cache.db")
	cache, err := store.OpenCache(t.Context(), cachePath, storetest.OpenOptions())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cache.Close() }) //nolint:errcheck // the test is over
	now := clocktest.NewFake(time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC))
	running, reads := runningSince{}, &timedReads{}
	estimator, err := NewEstimator(Parts{
		Database: database, Cache: cache, Attempts: running, Nodes: fakeNodes{}, ReadThroughs: reads, Clock: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	return &bench{
		estimator: estimator, database: database, cachePath: cachePath, clock: now, running: running,
		reads: reads,
	}
}

// file shelves a file of sizeMiB, minutes long, on a node.
func (bench *bench) file(t *testing.T, id, nodeID, sizeMiB, minutes int64) {
	t.Helper()
	storetest.ExecRaw(t, bench.database.Path(),
		fmt.Sprintf(`INSERT INTO file (id, library_id, node_id, path, size_bytes, payload_hash, filing,
		 first_seen_at, last_seen_at) VALUES (%d, 1, %d, '/m/%d.mkv', %d, X'%02x', 'unidentified', 1, 1)`,
			id, nodeID, id, sizeMiB*mib, id),
		fmt.Sprintf(`INSERT INTO file_probe (file_id, probe_json, duration_ms) VALUES (%d, '{}', %d)`,
			id, minutes*60_000))
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
		bench.file(t, file.id, file.node, file.mib, 90)
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

// A node's read speed is the mean of its latest five read-throughs, each its bytes over its time
// once it ran, timed as the read-throughs end; before any, its slowest read (D324 §6).
func TestEstimator_AReadThroughsNodeSpeedIsWhatItsLastReadsMeasured(t *testing.T) {
	t.Parallel()
	bench := newBench(t)
	bench.file(t, 100, 1, 600, 90)
	bench.readThrough(t, 1, 100, jobs.PriorityBackground, "queued", "{}")
	if left, _ := bench.left(t); left != time.Minute {
		t.Errorf("before any read, file 100's read-through takes %v; want 1m0s at the slowest read", left)
	}
	bench.reads.timings.ReadThroughFinished(1, 1000*mib, time.Second) // the oldest, forgotten
	for _, seconds := range []int64{10, 20, 30, 15, 12} {
		bench.reads.timings.ReadThroughFinished(1, 600*mib, time.Duration(seconds)*time.Second)
	}
	// 60, 30, 20, 40 and 50 MiB a second: 40 on average.
	if left, _ := bench.left(t); left != 15*time.Second {
		t.Errorf("after five reads, file 100's read-through takes %v; want 15s at their mean", left)
	}
}

// renditionOf queues a prepare of a file's h264 rendition on node 1, assigned or waiting, done so far.
func (bench *bench) renditionOf(t *testing.T, renditionID, fileID int64, state string, done float64) {
	t.Helper()
	storetest.ExecRaw(t, bench.cachePath,
		fmt.Sprintf(`INSERT INTO prepared_rendition (id, source_file, source_payload_hash, profile, node_id,
		 segment_length_seconds, run_id, created_at) VALUES (%d, %d, X'01', 'h264', 1, 6, 'r%d', 1)`,
			renditionID, fileID, renditionID),
		fmt.Sprintf(`INSERT INTO job (kind, rendition_id, priority, state, runnable_at, created_at,
		 progress_fraction) VALUES ('prepare', %d, 1000, '%s', 1, 1, %g)`, renditionID, state, done))
}

// A prepare says what is left at its own rate once it says how far it is; before, the film's length
// over its node's last realtime factor for the profile; with neither, that it cannot say (D324 §6).
func TestEstimator_APrepareGoesAtItsOwnRateOrItsNodesLast(t *testing.T) {
	t.Parallel()
	bench := newBench(t)
	bench.file(t, 100, 1, 1000, 40)
	bench.renditionOf(t, 1, 100, "queued", 0)
	prepare := func() Wait {
		t.Helper()
		waits, err := bench.estimator.Of(t.Context(), 100)
		if err != nil || waits.Prepare == nil {
			t.Fatalf("file 100's prepare wait is %+v (%v); want one", waits.Prepare, err)
		}
		return *waits.Prepare
	}
	if wait := prepare(); wait.IsKnown {
		t.Errorf("with no prepare finished on its node, the wait is %+v; want it unknown", wait)
	}
	storetest.ExecRaw(t, bench.cachePath, `INSERT INTO prepared_rendition (id, source_file, source_payload_hash,
	 profile, node_id, state, segment_length_seconds, run_id, realtime_factor, created_at, finished_at)
	 VALUES (9, 999, X'09', 'h264', 1, 'complete', 6, 'r9', 4, 1, 2)`)
	if wait := prepare(); !wait.IsKnown || wait.Left != 10*time.Minute || wait.IsRunning {
		t.Errorf("before it starts, the wait is %+v; want 10m0s, 40 minutes at 4 times playback", wait)
	}
	storetest.ExecRaw(t, bench.cachePath, `UPDATE job SET state = 'assigned', progress_fraction = 0.25
	 WHERE kind = 'prepare'`)
	jobID := int64(1)
	bench.running[jobID] = bench.clock.Now().Add(-time.Minute)
	if wait := prepare(); !wait.IsKnown || wait.Left != 3*time.Minute || !wait.IsRunning {
		t.Errorf("a quarter done in a minute, the wait is %+v; want 3m0s", wait)
	}
}

// An estimator built without one of its parts is refused.
func TestNewEstimator_RefusesPartsThatLeaveOneOut(t *testing.T) {
	t.Parallel()
	if _, err := NewEstimator(Parts{}); !errors.Is(err, ErrMissingPart) {
		t.Errorf("NewEstimator of no parts = %v; want ErrMissingPart", err)
	}
}
