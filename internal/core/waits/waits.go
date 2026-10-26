// SPDX-License-Identifier: AGPL-3.0-or-later

package waits

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/core/readthrough"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/cachegen"
)

// Wait is how long one piece of work a person waits on will take.
type Wait struct {
	// IsRunning is whether a node is doing it now, rather than it waiting its turn.
	IsRunning bool
	// Left is about how long until it is done, when IsKnown.
	Left time.Duration
	// IsKnown is whether there is anything to estimate Left from.
	IsKnown bool
}

// Waits are the work a file's viewer waits on: nil for work not open.
type Waits struct {
	// ReadThrough is its read-through's, for its loudness and its picture's timeline.
	ReadThrough *Wait
	// Prepare is its prepared copy's, for a device that plays one.
	Prepare *Wait
}

// Attempts say since when the node holding a job's attempt has run it: nodes.Exchanges.
type Attempts interface {
	RunningSinceOf(jobID int64) (time.Time, bool)
}

// Nodes say what each node can run at once and how fast it reads at the slowest: nodes.Service.
type Nodes interface {
	Resources(ctx context.Context, nodeID int64) (nodes.Resources, bool, error)
	SlowestReadBytesPerSecond(nodeID int64) int64
}

// ReadThroughs are what tells the estimator each full read's time: readthrough.Service.
type ReadThroughs interface {
	TimeReadsBy(timings readthrough.Timings)
}

// Parts are what the estimator reads, and the read-throughs it times.
type Parts struct {
	Database     *store.Database
	Cache        *store.Cache
	Attempts     Attempts
	Nodes        Nodes
	ReadThroughs ReadThroughs
	Clock        clock.Clock
}

// ErrMissingPart is an estimator built without one of its parts.
var ErrMissingPart = errors.New("the estimator is missing a part")

// readsMeasured is how many of a node's latest read-throughs its read speed is the median of: enough
// that one odd file moves it little, few enough that a disk that slowed shows within the hour.
const readsMeasured = 5

// Estimator says how long the work a person waits on will take (D324 §6).
type Estimator struct {
	parts Parts

	mutex sync.Mutex
	// readSpeeds are each node's latest read-throughs' speeds, in bytes a second, the newest last.
	readSpeeds map[int64][]float64 // guarded by mutex
}

// NewEstimator returns the estimator, knowing no read yet, timing the read-throughs as they finish;
// ErrMissingPart for parts that leave one out.
func NewEstimator(parts Parts) (*Estimator, error) {
	for _, part := range []struct {
		name      string
		isMissing bool
	}{
		{"Database", parts.Database == nil},
		{"Cache", parts.Cache == nil},
		{"Attempts", parts.Attempts == nil},
		{"Nodes", parts.Nodes == nil},
		{"ReadThroughs", parts.ReadThroughs == nil},
		{"Clock", parts.Clock == nil},
	} {
		if part.isMissing {
			return nil, fmt.Errorf("%s: %w", part.name, ErrMissingPart)
		}
	}
	estimator := &Estimator{parts: parts, readSpeeds: map[int64][]float64{}}
	parts.ReadThroughs.TimeReadsBy(estimator)
	return estimator, nil
}

// ReadThroughFinished measures a node's read speed by a read-through that read bytes in took.
func (estimator *Estimator) ReadThroughFinished(nodeID, bytes int64, took time.Duration) {
	if bytes <= 0 || took <= 0 {
		return
	}
	estimator.mutex.Lock()
	defer estimator.mutex.Unlock()
	speeds := estimator.readSpeeds[nodeID]
	speeds = append(speeds, float64(bytes)/took.Seconds())
	estimator.readSpeeds[nodeID] = slices.Clone(speeds[max(0, len(speeds)-readsMeasured):])
}

// readSpeed is a node's read speed in bytes a second: the median of its latest read-throughs, which
// one read an outage held for an hour moves little, as it made "about 59 minutes" of a mean (the
// owner's test, 2026-10-21); else its slowest read as its configuration says; zero when neither is
// known.
func (estimator *Estimator) readSpeed(nodeID int64) float64 {
	estimator.mutex.Lock()
	speeds := slices.Clone(estimator.readSpeeds[nodeID])
	estimator.mutex.Unlock()
	if len(speeds) == 0 {
		return float64(estimator.parts.Nodes.SlowestReadBytesPerSecond(nodeID))
	}
	slices.Sort(speeds)
	middle := len(speeds) / 2
	if len(speeds)%2 == 0 {
		return (speeds[middle-1] + speeds[middle]) / 2
	}
	return speeds[middle]
}

// Of is the work a file's viewer waits on, and how long each will take.
func (estimator *Estimator) Of(ctx context.Context, fileID int64) (Waits, error) {
	read, err := estimator.readThroughOf(ctx, fileID)
	if err != nil {
		return Waits{}, err
	}
	prepare, err := estimator.prepareOf(ctx, fileID)
	if err != nil {
		return Waits{}, err
	}
	return Waits{ReadThrough: read, Prepare: prepare}, nil
}

// sizedFile is a file's node, size and length.
type sizedFile struct {
	nodeID, sizeBytes, durationMs int64
}

// filesOf are the node, size and length of each live file named.
func (estimator *Estimator) filesOf(ctx context.Context, fileIDs []int64) (map[int64]sizedFile, error) {
	idsJSON, err := json.Marshal(fileIDs)
	if err != nil {
		return nil, fmt.Errorf("encode the files: %w", err)
	}
	rows, err := estimator.parts.Database.Read().ListFileSizesAndNodes(ctx, string(idsJSON))
	if err != nil {
		return nil, fmt.Errorf("read the files' sizes: %w", err)
	}
	files := make(map[int64]sizedFile, len(rows))
	for _, row := range rows {
		files[row.ID] = sizedFile{
			nodeID: row.NodeID, sizeBytes: ptr.ValueOrZero(row.SizeBytes), durationMs: row.DurationMs,
		}
	}
	return files, nil
}

// readThroughOf is how long the file's read-through will take: nil when none is open.
func (estimator *Estimator) readThroughOf(ctx context.Context, fileID int64) (*Wait, error) {
	ownJobs, err := estimator.parts.Cache.Jobs().OpenJobsOfFile(ctx, jobs.KindReadThrough, fileID)
	if err != nil {
		return nil, err
	}
	if _, _, isOpen := fullReadsAndOwn(ownJobs, fileID); !isOpen {
		return nil, nil //nolint:nilnil // no read-through of its own is open: nothing to wait for
	}
	// Only now every other's, for the bytes ahead of it: most files a viewer looks at have none.
	open, err := estimator.parts.Cache.Jobs().OpenReadThroughs(ctx)
	if err != nil {
		return nil, err
	}
	fullReads, own, isOpen := fullReadsAndOwn(open, fileID)
	if !isOpen {
		return nil, nil //nolint:nilnil // no read-through is open: nothing to wait for
	}
	fileIDs := make([]int64, 0, len(fullReads))
	for _, row := range fullReads {
		fileIDs = append(fileIDs, row.FileID)
	}
	files, err := estimator.filesOf(ctx, fileIDs)
	if err != nil {
		return nil, err
	}
	ownFile, isLive := files[fileID]
	wait := &Wait{IsRunning: own.IsAssigned}
	speed := estimator.readSpeed(ownFile.nodeID)
	if !isLive || speed <= 0 {
		return wait, nil
	}
	if wait.IsRunning {
		wait.Left, wait.IsKnown = estimator.runningLeft(own, ownFile, speed), true
		return wait, nil
	}
	together, err := estimator.readsAtOnce(ctx, ownFile.nodeID)
	if err != nil {
		return nil, err
	}
	aheadBytes := estimator.bytesAhead(fullReads, own, ownFile.nodeID, files)
	wait.Left = durationOf(aheadBytes, speed*float64(together)) + durationOf(float64(ownFile.sizeBytes), speed)
	wait.IsKnown = true
	return wait, nil
}

// fullReadsAndOwn are the open read-throughs that read their files through, an index read taking a
// fraction of a second whether ahead or the file's own, and the file's own, its highest, if any.
func fullReadsAndOwn(open []store.OpenJob, fileID int64) ([]store.OpenJob, store.OpenJob, bool) {
	var fullReads []store.OpenJob
	var own store.OpenJob
	isOpen := false
	for _, row := range open {
		if row.FileID == 0 || readthrough.IsIndexRead(row.ArgsJSON) {
			continue
		}
		if row.FileID == fileID && (!isOpen || row.Priority > own.Priority) {
			own, isOpen = row, true
		}
		fullReads = append(fullReads, row)
	}
	return fullReads, own, isOpen
}

// bytesAhead are what the read-throughs ahead of own on its node have still to read.
func (estimator *Estimator) bytesAhead(
	fullReads []store.OpenJob, own store.OpenJob, nodeID int64, files map[int64]sizedFile,
) float64 {
	var ahead float64
	for _, row := range fullReads {
		file, isLive := files[row.FileID]
		if row.ID != own.ID && isLive && file.nodeID == nodeID && isAhead(row, own) {
			ahead += estimator.bytesLeft(row, file)
		}
	}
	return ahead
}

// isAhead is whether a read-through goes before own on their node: one running that own would not
// bump, a person's being the higher tier than the background's (D255 §2); one waiting above it, or
// level with it and older.
func isAhead(row, own store.OpenJob) bool {
	if row.IsAssigned {
		isBumpedByOwn := own.Priority >= jobs.PriorityPersonsRequest && row.Priority < jobs.PriorityPersonsRequest
		return !isBumpedByOwn
	}
	return row.Priority > own.Priority || (row.Priority == own.Priority && row.ID < own.ID)
}

// bytesLeft are what a read-through has still to read: all of them before its node runs it, and,
// while it runs, what is left at its node's speed since it began.
func (estimator *Estimator) bytesLeft(row store.OpenJob, file sizedFile) float64 {
	since, isRunning := estimator.parts.Attempts.RunningSinceOf(row.ID)
	if !row.IsAssigned || !isRunning {
		return float64(file.sizeBytes)
	}
	read := estimator.readSpeed(file.nodeID) * estimator.parts.Clock.Now().Sub(since).Seconds()
	return max(0, float64(file.sizeBytes)-read)
}

// runningLeft is how long a running read-through has left: at its own pace once its node says how
// far into the film it is, which a disk slower than the node's measured speed is read at; before,
// its bytes left at that speed.
func (estimator *Estimator) runningLeft(own store.OpenJob, file sizedFile, speed float64) time.Duration {
	since, isRunning := estimator.parts.Attempts.RunningSinceOf(own.ID)
	ran := estimator.parts.Clock.Now().Sub(since)
	if done := own.DoneFraction; isRunning && done > 0 && ran > 0 {
		return time.Duration(float64(ran) * (1 - done) / done)
	}
	return durationOf(estimator.bytesLeft(own, file), speed)
}

// readsAtOnce is how many read-throughs a node runs at once: its read-through slots, at least one;
// those it is given beyond them wait there, and run no sooner.
func (estimator *Estimator) readsAtOnce(ctx context.Context, nodeID int64) (int, error) {
	resources, isConnected, err := estimator.parts.Nodes.Resources(ctx, nodeID)
	if err != nil {
		return 0, fmt.Errorf("read node %d's slots: %w", nodeID, err)
	}
	if !isConnected {
		return 1, nil
	}
	return max(1, resources.Capacity.SlotCount(nodeproto.SlotReadThrough)), nil
}

// prepareOf is how long the file's prepared copy will take: nil when no prepare of it is open.
func (estimator *Estimator) prepareOf(ctx context.Context, fileID int64) (*Wait, error) {
	open, err := estimator.parts.Cache.Jobs().OpenPreparesOfFile(ctx, fileID)
	if err != nil {
		return nil, err
	}
	if len(open) == 0 {
		return nil, nil //nolint:nilnil // no prepare is open: nothing to wait for
	}
	prepare := open[0]
	wait := &Wait{IsRunning: prepare.IsAssigned}
	since, isRunning := estimator.parts.Attempts.RunningSinceOf(prepare.ID)
	ran := time.Duration(0)
	if wait.IsRunning && isRunning {
		ran = estimator.parts.Clock.Now().Sub(since)
	}
	if done := prepare.DoneFraction; done > 0 && ran > 0 {
		wait.Left, wait.IsKnown = time.Duration(float64(ran)*(1-done)/done), true
		return wait, nil
	}
	factor, err := store.OrNotFound(estimator.parts.Cache.Read().GetLatestRealtimeFactor(ctx,
		cachegen.GetLatestRealtimeFactorParams{NodeID: prepare.NodeID, Profile: prepare.Profile}))
	if errors.Is(err, store.ErrNotFound) {
		return wait, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read node %d's last realtime factor: %w", prepare.NodeID, err)
	}
	files, err := estimator.filesOf(ctx, []int64{fileID})
	if err != nil {
		return nil, err
	}
	if file := files[fileID]; file.durationMs > 0 && factor > 0 {
		whole := time.Duration(float64(file.durationMs)/factor) * time.Millisecond
		wait.Left, wait.IsKnown = max(0, whole-ran), true
	}
	return wait, nil
}

// durationOf is how long bytes take at speed bytes a second.
func durationOf(bytes, speed float64) time.Duration {
	return time.Duration(bytes / speed * float64(time.Second))
}
