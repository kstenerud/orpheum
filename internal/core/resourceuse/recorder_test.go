// SPDX-License-Identifier: AGPL-3.0-or-later

package resourceuse_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/core/resourceuse"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/platform/selfuse"
	"github.com/kstenerud/orpheum/internal/store/storetest"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// bench is a recorder whose readings the test sets.
type bench struct {
	recorder *resourceuse.Recorder
	clock    *clocktest.Fake
	own      selfuse.Sample
	ownErr   error
	nodes    []nodes.NodeSelfUse
	running  map[int64]map[jobs.Kind]int
	plays    int
}

func (bench *bench) SelfUses(context.Context) []nodes.NodeSelfUse { return bench.nodes }
func (bench *bench) RunningKinds() map[int64]map[jobs.Kind]int    { return bench.running }

// newBench returns a recorder at the top of an hour, on a fresh cache.
func newBench(t *testing.T) *bench {
	t.Helper()
	b := &bench{clock: clocktest.NewFake(time.Date(2026, 11, 3, 20, 0, 0, 0, time.UTC))}
	b.recorder = resourceuse.NewRecorder(storetest.OpenCache(t), b.clock, slog.New(slog.DiscardHandler),
		resourceuse.Sources{
			ReadOwn: func() (selfuse.Sample, error) { return b.own, b.ownErr },
			Nodes:   b, Work: b, Plays: func() int { return b.plays },
		})
	return b
}

// record records, failing the test on an error.
func (bench *bench) record(t *testing.T) {
	t.Helper()
	if err := bench.recorder.Record(t.Context()); err != nil {
		t.Fatal(err)
	}
}

// report reads the report, failing the test on an error.
func (bench *bench) report(t *testing.T) resourceuse.Report {
	t.Helper()
	report, err := bench.recorder.Report(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return report
}

func TestRecord_CountsEachProcesssCPUTimeSinceItsLastReading(t *testing.T) {
	t.Parallel()
	bench := newBench(t)
	bench.own = selfuse.Sample{Scope: selfuse.ScopeGroup, CPUMicros: 1_000_000, HeldBytes: 100, CacheBytes: 10}
	bench.nodes = []nodes.NodeSelfUse{{NodeID: 3, Name: "nas", SentAt: 1_000, Use: nodeproto.SelfUse{
		Scope: "cgroup", CPUMicros: 5_000_000, HeldBytes: 500, PeakBytes: 900,
	}}}
	bench.record(t)
	if summaries := bench.report(t).Summaries; len(summaries) != 2 || summaries[0].HasCPU || summaries[1].HasCPU {
		t.Fatalf("after one reading, summaries = %+v; want the core's and nas's, with no CPU time yet", summaries)
	}

	bench.clock.Advance(time.Minute)
	bench.own.CPUMicros += 30_000_000 // half of one core over the minute
	bench.own.HeldBytes = 300
	bench.plays = 2
	bench.nodes[0].SentAt += 60_000
	bench.nodes[0].Use.CPUMicros += 120_000_000 // two cores' worth
	bench.running = map[int64]map[jobs.Kind]int{3: {jobs.KindMakeChunks: 2}}
	bench.record(t)

	checkHalfACoreAndTwo(t, bench.report(t))
}

// checkHalfACoreAndTwo checks a report of the core using half a core and holding 300 bytes, beside
// two plays, and nas using two cores, beside two make_chunks.
func checkHalfACoreAndTwo(t *testing.T, report resourceuse.Report) {
	t.Helper()
	summaries := report.Summaries
	if len(summaries) != 2 {
		t.Fatalf("summaries = %+v; want the core's and nas's", summaries)
	}
	core, node := summaries[0], summaries[1]
	if core.IsNode || !core.HasCPU || core.CPUPercent != 50 || core.BusiestCPUPercent != 50 || core.HeldBytes != 300 {
		t.Errorf("the core's summary = %+v; want 50%% of a core, holding 300 bytes at most", core)
	}
	if !node.IsNode || node.NodeName != "nas" || node.CPUPercent != 200 || node.PeakBytes != 900 {
		t.Errorf("nas's summary = %+v; want 200%% of a core and a peak of 900 bytes", node)
	}
	hours := report.Hours
	if len(hours) != 2 || hours[0].Plays != 2 || hours[1].Running[string(jobs.KindMakeChunks)] != 2 {
		t.Errorf("hours = %+v; want the core's with 2 plays, then nas's running 2 make_chunks", hours)
	}
}

func TestRecord_CountsNoCPUTimeAcrossARestartOrANodeThatSaidNothingNew(t *testing.T) {
	t.Parallel()
	bench := newBench(t)
	bench.own = selfuse.Sample{Scope: selfuse.ScopeProcess, CPUMicros: 9_000_000}
	bench.nodes = []nodes.NodeSelfUse{{NodeID: 3, Name: "nas", SentAt: 1_000, Use: nodeproto.SelfUse{
		Scope: "cgroup", CPUMicros: 9_000_000,
	}}}
	bench.record(t)
	bench.clock.Advance(time.Minute)
	bench.own.CPUMicros = 1_000 // the counter went back: the process restarted
	bench.record(t)             // nas said nothing new: no row of its
	bench.clock.Advance(time.Minute)
	bench.own.CPUMicros += 6_000_000
	bench.record(t)

	hours := bench.report(t).Hours
	if len(hours) != 2 {
		t.Fatalf("hours = %+v; want the core's and nas's", hours)
	}
	if core := hours[0]; !core.HasCPU || core.CPUPercent != 10 {
		t.Errorf("the core's hour = %+v; want only the minute after the restart counted, 10%% of a core", core)
	}
	if node := hours[1]; node.HasCPU {
		t.Errorf("nas's hour = %+v; want no CPU time from its one reading", node)
	}
}

func TestRecord_RecordsTheNodesWhenTheCoreCannotReadItsOwn(t *testing.T) {
	t.Parallel()
	bench := newBench(t)
	bench.ownErr = errors.New("no /proc")
	bench.nodes = []nodes.NodeSelfUse{{NodeID: 3, Name: "nas", SentAt: 1_000, Use: nodeproto.SelfUse{Scope: "cgroup"}}}
	bench.record(t)
	if summaries := bench.report(t).Summaries; len(summaries) != 1 || summaries[0].NodeName != "nas" {
		t.Errorf("summaries = %+v; want nas's alone", summaries)
	}
}

func TestRecord_ForgetsRowsOlderThanAMonth(t *testing.T) {
	t.Parallel()
	bench := newBench(t)
	bench.own = selfuse.Sample{Scope: selfuse.ScopeGroup, HeldBytes: 1}
	bench.record(t)
	firstAt := bench.clock.Now()
	bench.clock.Advance(resourceuse.KeptFor + time.Minute)
	bench.record(t)
	summaries := bench.report(t).Summaries
	if len(summaries) != 1 || !summaries[0].Since.After(firstAt) {
		t.Errorf("summaries = %+v; want the core's from its last reading alone", summaries)
	}
}

func TestRun_RecordsAtEachTickUntilItsContextEnds(t *testing.T) {
	t.Parallel()
	bench := newBench(t)
	bench.own = selfuse.Sample{Scope: selfuse.ScopeGroup}
	tick := make(chan time.Time)
	ctx, cancel := context.WithCancel(t.Context())
	ended := make(chan struct{})
	go func() {
		bench.recorder.Run(ctx, tick)
		close(ended)
	}()
	tick <- bench.clock.Now()
	tick <- bench.clock.Now() // the second tick is taken once the first is recorded
	cancel()
	testutil.Receive(t, ended, "the recorder to stop as its context ends")
	if summaries := bench.report(t).Summaries; len(summaries) != 1 {
		t.Errorf("summaries = %+v; want the core's, recorded at the tick", summaries)
	}
}
