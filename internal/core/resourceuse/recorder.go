// SPDX-License-Identifier: AGPL-3.0-or-later

package resourceuse

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/platform/selfuse"
	"github.com/kstenerud/orpheum/internal/platform/slogx"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// Every is how often the figures are recorded: a minute, short enough to show a play's or a run's
// load, and a row a process a minute keeps a month's in a few megabytes.
const Every = time.Minute

// KeptFor is how long a row is kept: a month, long enough to see the busiest day of a household's
// weeks, against what the server needs.
const KeptFor = 30 * 24 * time.Hour

// The processes a row is of.
const (
	processCore = "core"
	processNode = "node"
)

// Nodes are the connected nodes' readings of what they use: nodes.Service.
type Nodes interface {
	SelfUses(ctx context.Context) []nodes.NodeSelfUse
}

// Work is the work running on each node, by kind: nodes.Exchanges.
type Work interface {
	RunningKinds() map[int64]map[jobs.Kind]int
}

// Sources are where the recorder reads what each process uses and what it was doing.
type Sources struct {
	// ReadOwn reads what the core uses: selfuse.Read.
	ReadOwn func() (selfuse.Sample, error)
	Nodes   Nodes
	Work    Work
	// Plays is how many playbacks are playing now: len(playback.Service.NowPlaying()).
	Plays func() int
}

// Recorder records, once a minute, what the core and each connected node used of their machines,
// beside what they were doing, and reads it back for the System page.
type Recorder struct {
	database *store.Database
	clock    clock.Clock
	logger   *slog.Logger
	sources  Sources
	failing  slogx.Failing
	// ownFailing says once that the core cannot read what it uses, which does not change.
	ownFailing sync.Once

	mutex sync.Mutex
	// previous is each process's last reading, which the next one's CPU time is counted from.
	previous map[processKey]reading // guarded by mutex
}

// processKey names one process: the core, or a node by its name.
type processKey struct {
	process, nodeName string
}

// reading is one reading of a process's counters, at readAt by the clock that read it.
type reading struct {
	scope     selfuse.Scope
	cpuMicros int64
	readAt    int64
}

// NewRecorder returns a recorder writing to database.
func NewRecorder(database *store.Database, clockSource clock.Clock, logger *slog.Logger, sources Sources) *Recorder {
	return &Recorder{
		database: database, clock: clockSource, logger: logger, sources: sources,
		previous: map[processKey]reading{},
	}
}

// Run records at each tick until ctx ends, saying once a minute while a record fails.
func (recorder *Recorder) Run(ctx context.Context, tick <-chan time.Time) {
	for {
		var tickedAt time.Time
		select {
		case <-ctx.Done():
			return
		case tickedAt = <-tick:
		}
		if err := recorder.Record(ctx); err != nil {
			if isSaid, lasting := recorder.failing.Fail(tickedAt); isSaid {
				recorder.logger.WarnContext(ctx, "could not record what the core and nodes use; trying again "+
					"at the next minute", append([]any{"err", err}, lasting...)...)
			}
			continue
		}
		if isEnded, lasted := recorder.failing.Succeed(tickedAt); isEnded {
			recorder.logger.InfoContext(ctx, "what the core and nodes use is recorded again", lasted...)
		}
	}
}

// Record records what the core and each connected node use now, and forgets the rows older than
// KeptFor. A node with no reading since the last record has no row: its figures would repeat.
func (recorder *Recorder) Record(ctx context.Context) error {
	now := recorder.clock.Now().UnixMilli()
	rows := recorder.rows(ctx, now)
	err := recorder.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		if err := queries.DeleteResourceUseBefore(ctx, now-KeptFor.Milliseconds()); err != nil {
			return fmt.Errorf("forget the old rows: %w", err)
		}
		for _, row := range rows {
			if err := queries.InsertResourceUse(ctx, row); err != nil {
				return fmt.Errorf("record what %s %s uses: %w", row.Process, row.NodeName, err)
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("record what the core and nodes use: %w", err)
	}
	return nil
}

// rows are the rows to record at now: the core's, when it can read its own, and each node's that
// read anew since the last.
func (recorder *Recorder) rows(ctx context.Context, now int64) []sqlcgen.InsertResourceUseParams {
	own, ownErr := recorder.sources.ReadOwn()
	running := recorder.sources.Work.RunningKinds()
	uses := recorder.sources.Nodes.SelfUses(ctx)
	plays := int64(recorder.sources.Plays())
	recorder.mutex.Lock()
	defer recorder.mutex.Unlock()
	var rows []sqlcgen.InsertResourceUseParams
	if ownErr == nil {
		core := recorder.row(processKey{process: processCore}, now, now, own)
		core.Plays = plays
		rows = append(rows, core)
	} else {
		recorder.ownFailing.Do(func() {
			recorder.logger.WarnContext(ctx, "cannot read what the core uses of its machine; only the nodes' "+
				"use is recorded", "err", ownErr)
		})
	}
	for _, node := range uses {
		key := processKey{process: processNode, nodeName: node.Name}
		if last, found := recorder.previous[key]; found && last.readAt == node.SentAt {
			continue
		}
		row := recorder.row(key, now, node.SentAt, sampleOf(node.Use))
		if kinds := running[node.NodeID]; len(kinds) > 0 {
			encoded, err := json.Marshal(kinds)
			if err == nil {
				row.RunningJSON = string(encoded)
			}
		}
		rows = append(rows, row)
	}
	return rows
}

// row is a process's row at now for a sample read at readAt, with the CPU time it used since its
// last reading: none for its first, or when its counter went back, its process having restarted.
func (recorder *Recorder) row(
	key processKey, now, readAt int64, sample selfuse.Sample,
) sqlcgen.InsertResourceUseParams {
	row := sqlcgen.InsertResourceUseParams{
		At: now, Process: key.process, NodeName: key.nodeName, Scope: string(sample.Scope),
		HeldBytes: sample.HeldBytes, CacheBytes: sample.CacheBytes, RunningJSON: "{}",
	}
	if sample.PeakBytes > 0 {
		row.PeakBytes = &sample.PeakBytes
	}
	last, found := recorder.previous[key]
	if found && last.scope == sample.Scope && sample.CPUMicros >= last.cpuMicros && readAt > last.readAt {
		used, elapsed := sample.CPUMicros-last.cpuMicros, readAt-last.readAt
		row.CpuMicros, row.ElapsedMs = &used, &elapsed
	}
	recorder.previous[key] = reading{scope: sample.Scope, cpuMicros: sample.CPUMicros, readAt: readAt}
	return row
}

// sampleOf is a node's reading as a sample.
func sampleOf(use nodeproto.SelfUse) selfuse.Sample {
	return selfuse.Sample{
		Scope: selfuse.Scope(use.Scope), CPUMicros: use.CPUMicros, HeldBytes: use.HeldBytes,
		CacheBytes: use.CacheBytes, PeakBytes: use.PeakBytes,
	}
}
