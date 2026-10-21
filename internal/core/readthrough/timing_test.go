// SPDX-License-Identifier: AGPL-3.0-or-later

package readthrough

import (
	"context"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library/librarytest"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// timing is one read the timings were told of.
type timing struct {
	nodeID, bytes int64
	took          time.Duration
}

// timings record what they are told.
type timings struct{ told []timing }

func (record *timings) ReadThroughFinished(nodeID, bytes int64, took time.Duration) {
	record.told = append(record.told, timing{nodeID: nodeID, bytes: bytes, took: took})
}

// A full read that ends tells the timings its file's bytes and how long it took from when its node
// said it ran; an index read, or one never said to run, tells nothing (D324 §6).
func TestTimeRead_AFullReadTellsItsBytesAndHowLongItRan(t *testing.T) {
	t.Parallel()
	database := storetest.OpenMigrated(t)
	librarytest.NodeWithLibrary(t, database, librarytest.Library{Name: "Films", Kind: "movies", MetadataSource: "nfo"})
	storetest.ExecRaw(t, database.Path(), `INSERT INTO file (id, library_id, node_id, path, size_bytes,
	 payload_hash, filing, first_seen_at, last_seen_at) VALUES (4, 1, 1, '/m/a.mkv', 4000, X'04', 'unidentified',
	 1, 1)`, `INSERT INTO file_probe (file_id, probe_json) VALUES (4, '{}')`)
	now := clocktest.NewFake(time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC))
	record := &timings{}
	service := &Service{database: database, clock: now}
	service.TimeReadsBy(record)
	job := jobs.Job{ID: 9, Kind: jobs.KindReadThrough, Target: jobs.Target{FileID: 4}}
	read := func(isIndexOnly bool, runningSince time.Time) {
		command := &nodeproto.ReadThrough{LoudnessTracks: []nodeproto.LoudnessTrack{{StreamIndex: 1}}}
		if isIndexOnly {
			command.LoudnessTracks = nil
			command.VideoTimeline = &nodeproto.VideoTimelineAsk{IsFromIndexOnly: true}
		}
		service.timeRead(t.Context(), job, nodes.Attempt{
			NodeID: 1, Work: nodeproto.Work{ReadThrough: command}, RunningSince: runningSince,
		})
	}
	read(true, now.Now().Add(-time.Second))
	read(false, time.Time{})
	read(false, now.Now().Add(-40*time.Second))
	if want := []timing{{nodeID: 1, bytes: 4000, took: 40 * time.Second}}; len(record.told) != 1 ||
		record.told[0] != want[0] {
		t.Errorf("the timings were told %+v; want %+v, the full read alone", record.told, want)
	}
}

// progressing records the progress a read-through's job is given.
type progressing struct {
	Queue
	fractions []float64
}

func (queue *progressing) SetProgress(_ context.Context, _ int64, fraction float64) error {
	queue.fractions = append(queue.fractions, fraction)
	return nil
}

// How far into its film a running read-through is, as its node says, is its job's progress, for how
// long the rest will take; a word without a position, or of a film of no known length, keeps none
// (D324 §6).
func TestTakeProgress_ARunningReadsPositionIsItsJobsProgress(t *testing.T) {
	t.Parallel()
	queue := &progressing{}
	service := &Service{queue: queue}
	attempt := nodes.Attempt{JobID: 9, Work: nodeproto.Work{ReadThrough: &nodeproto.ReadThrough{DurationMs: 40_000}}}
	for _, progress := range []nodeproto.Progress{
		{State: nodeproto.ProgressRunning, ReadThroughMs: 10_000},
		{State: nodeproto.ProgressWaiting, ReadThroughMs: 20_000},
		{State: nodeproto.ProgressRunning},
	} {
		if err := service.takeProgress(t.Context(), attempt, progress); err != nil {
			t.Fatal(err)
		}
	}
	unknownLength := nodes.Attempt{JobID: 9, Work: nodeproto.Work{ReadThrough: &nodeproto.ReadThrough{}}}
	if err := service.takeProgress(t.Context(), unknownLength,
		nodeproto.Progress{State: nodeproto.ProgressRunning, ReadThroughMs: 10_000}); err != nil {
		t.Fatal(err)
	}
	if len(queue.fractions) != 1 || queue.fractions[0] != 0.25 {
		t.Errorf("the progress kept = %v; want a quarter, from the running word with a position alone",
			queue.fractions)
	}
}
