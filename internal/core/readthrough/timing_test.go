// SPDX-License-Identifier: AGPL-3.0-or-later

package readthrough

import (
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
