// SPDX-License-Identifier: GPL-3.0-or-later

package jobs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/platform/jobqueue"
	"github.com/kstenerud/orpheum/internal/store"
)

// Kind is what a job does; the job table's CHECK names the set.
type Kind = jobqueue.Kind

// The kinds of job (cut-a §5.3).
const (
	// KindFetchMetadata fetches an item's metadata from its provider; its target is the item.
	KindFetchMetadata Kind = "fetch_metadata"
	// KindMakeArtworkSizes makes the sizes of one of an item's images, fetching its source when
	// no stored size can be cut down; its target is the item (D212).
	KindMakeArtworkSizes Kind = "make_artwork_sizes"
	KindPrepare          Kind = "prepare"
	// KindExtractSubtitle extracts one text subtitle track; its target is the stream (D217 §3).
	KindExtractSubtitle Kind = "extract_subtitle"
	// KindConvertFetchedSubtitle converts a subtitle fetched on request; its target is the
	// fetched subtitle (D224 §5).
	KindConvertFetchedSubtitle Kind = "convert_fetched_subtitle"
	// KindMeasureLoudness measures a video's audio tracks; its target is the file (D226 §2).
	KindMeasureLoudness Kind = "measure_loudness"
	KindScan            Kind = "scan"
	KindBackup          Kind = "backup"
)

// Kinds are every kind of job, as the job view documents them: the one list, which the API's
// description of a job is made from.
func Kinds() []Kind {
	return []Kind{
		KindFetchMetadata, KindMakeArtworkSizes, KindPrepare, KindExtractSubtitle, KindConvertFetchedSubtitle,
		KindMeasureLoudness, KindScan, KindBackup,
	}
}

// Target is what a job works on, by its kind (D227 §2): an item for fetch and artwork, a
// rendition for prepare, a stream for extract_subtitle, a fetched subtitle for
// convert_fetched_subtitle, a file for measure_loudness. A job is queued with exactly one set
// and the others zero; Enqueue refuses any other target with ErrNotOneTarget. The job table
// allows at most one, but scan and backup have no target yet, so no job of those kinds can be
// queued until M4 says what they work on.
type Target = store.JobTarget

// ErrNotOneTarget is a job that names no target, or more than one.
var ErrNotOneTarget = errors.New("a job works on one target")

// requireOneTarget refuses a target that names no target or several.
func requireOneTarget(target Target) error {
	setCount := 0
	for _, id := range []int64{
		target.ItemID, target.RenditionID, target.StreamID, target.FetchedSubtitleID, target.FileID,
	} {
		if id != 0 {
			setCount++
		}
	}
	if setCount != 1 {
		return fmt.Errorf("%w: %+v", ErrNotOneTarget, target)
	}
	return nil
}

// Job is one queued piece of work, as a runner receives it.
type Job struct {
	ID   int64
	Kind Kind
	Target
	ArgsJSON string
	// Priority is how urgent it is: a handler tells a provider how much its request matters by it
	// (ImportanceOf).
	Priority      int64
	AttemptNumber int64
	// PreviousAttemptToolFailed is whether the job's last attempt ended with its tool failing: a
	// stall (MarkStalled) or a panic in its handler. Its handler may make a second stall in a row
	// the target's failure; the queue fails the job on a second of either (D233 §6).
	PreviousAttemptToolFailed bool
}

// The priorities that say who is waiting: a job someone is waiting on to watch is the highest
// (cut-a §5.3); one a person asked for, such as a refresh, comes before the queue's own work, and
// looks at its destination at once if it is parked (D231 §6).
const (
	PriorityPlayNow        int64 = 1000
	PriorityPersonsRequest int64 = 20
)

// Request is a job to queue.
type Request struct {
	Kind Kind
	Target
	ArgsJSON string // JSON; "{}" when empty
	Priority int64
	Reason   string // why it exists, shown to the user
	// RunnableAt is when it may first run; now when zero.
	RunnableAt time.Time
	// Destination is what it needs that can be down, when the one queuing it knows (D231 §5); a
	// job that finds out while it runs parks on it then.
	Destination Destination
}

// ArgsMerge is how a kind folds a repeat request's arguments into its open job (D231 §8).
type ArgsMerge = jobqueue.ArgsMerge

// Settings are the values the queue reads when a job fails.
type Settings interface {
	Int(ctx context.Context, key settings.Key) (int64, error)
	Duration(ctx context.Context, key settings.Key) (time.Duration, error)
}

// QueueParts are what the queue is built from.
type QueueParts struct {
	Cache    *store.Cache
	Clock    clock.Clock
	Settings Settings
	// Random is where the jitter on a retry's wait comes from: crypto/rand's Reader.
	Random io.Reader
	Logger *slog.Logger
	// ArgsMerges are the kinds whose arguments merge their own way; the others keep the queued
	// job's (D231 §8).
	ArgsMerges map[Kind]ArgsMerge
}

// Queue is the one work queue in cache.db (schema.md, cache.db): internal/platform/jobqueue
// over the job table, with Orpheum's kinds and targets.
type Queue struct {
	queue *jobqueue.Queue[Target]
}

// NewQueue returns the queue. A person's request probes its parked destination at once, and
// cache.db and library.db failing for want of room or with an I/O error park the core's storage
// (D231 §5, §6).
func NewQueue(parts QueueParts) *Queue {
	return newQueue(parts.Cache.Jobs(), parts)
}

// newQueue is the queue over storage: cache.db's job table, or a test's.
func newQueue(storage jobqueue.Storage[Target], parts QueueParts) *Queue {
	return &Queue{queue: jobqueue.New(storage, parts.Clock, jobqueue.Config{
		Policy: settingsPolicy{settings: parts.Settings}, Probes: settingsPolicy{settings: parts.Settings},
		Random: parts.Random, Logger: parts.Logger, ArgsMerges: parts.ArgsMerges,
		PersonsRequestPriority: PriorityPersonsRequest,
		StorageDestination:     DestinationCoreStorage, IsStorageFailure: store.IsStorageFailure,
	})}
}

// Enqueue queues a job, or folds it into the one already open for its kind and target
// (jobqueue.Queue.Enqueue says how), and reports whether it added one.
func (queue *Queue) Enqueue(ctx context.Context, job Request) (bool, error) {
	if err := requireOneTarget(job.Target); err != nil {
		return false, err
	}
	action, err := queue.queue.Enqueue(ctx, jobqueue.Request[Target]{
		Kind: job.Kind, Target: job.Target, ArgsJSON: job.ArgsJSON, Priority: job.Priority, Reason: job.Reason,
		RunnableAt: job.RunnableAt, Destination: job.Destination,
	})
	if err != nil {
		return false, fmt.Errorf("enqueue: %w", err)
	}
	return action == jobqueue.ActionInsert, nil
}

// EverQueued reports whether a job of this kind was ever queued for the target.
func (queue *Queue) EverQueued(ctx context.Context, kind Kind, target Target) (bool, error) {
	if err := requireOneTarget(target); err != nil {
		return false, err
	}
	return queue.queue.EverQueued(ctx, kind, target)
}

// RequeueRunning returns the jobs a stop left running to the queue: their output is a cache, so
// they restart from zero (cut-a §5.3). Each keeps its counted attempt (S86).
func (queue *Queue) RequeueRunning(ctx context.Context) (int64, error) {
	return queue.queue.RequeueRunning(ctx)
}

// Wake tells every runner there may be work before its next tick.
func (queue *Queue) Wake() { queue.queue.Wake() }

// settingsPolicy reads the queue's retry policy and its probe back-off from the settings.
type settingsPolicy struct {
	settings Settings
}

// AttemptLimit is job_attempt_limit.
func (policy settingsPolicy) AttemptLimit(ctx context.Context) (int64, error) {
	attemptLimit, err := policy.settings.Int(ctx, settings.JobAttemptLimit)
	if err != nil {
		return 0, fmt.Errorf("read job_attempt_limit: %w", err)
	}
	return attemptLimit, nil
}

// FirstRetryDelay is job_first_retry_delay_seconds.
func (policy settingsPolicy) FirstRetryDelay(ctx context.Context) (time.Duration, error) {
	firstRetryDelay, err := policy.settings.Duration(ctx, settings.JobFirstRetryDelaySeconds)
	if err != nil {
		return 0, fmt.Errorf("read job_first_retry_delay_seconds: %w", err)
	}
	return firstRetryDelay, nil
}

// MarkStalled marks a handler's error as a tool that stalled on the job, a hang that says nothing
// of the target: the job is tried again after job_first_retry_delay_seconds at no cost of an
// attempt, and its next attempt knows the last one's tool failed (Job.PreviousAttemptToolFailed),
// so its handler can make a second stall in a row the target's failure. D231's item facts
// replace that.
func MarkStalled(err error) error { return jobqueue.ToolFailed(fmt.Errorf("stalled: %w", err)) }

// MarkPermanent marks a handler's error as one no retry can fix.
func MarkPermanent(err error) error { return jobqueue.Permanent(err) }

// FirstProbeDelay is destination_first_probe_delay_seconds.
func (policy settingsPolicy) FirstProbeDelay(ctx context.Context) (time.Duration, error) {
	delay, err := policy.settings.Duration(ctx, settings.DestinationFirstProbeDelaySeconds)
	if err != nil {
		return 0, fmt.Errorf("read destination_first_probe_delay_seconds: %w", err)
	}
	return delay, nil
}

// LongestProbeDelay is destination_longest_probe_delay_seconds.
func (policy settingsPolicy) LongestProbeDelay(ctx context.Context) (time.Duration, error) {
	delay, err := policy.settings.Duration(ctx, settings.DestinationLongestProbeDelaySeconds)
	if err != nil {
		return 0, fmt.Errorf("read destination_longest_probe_delay_seconds: %w", err)
	}
	return delay, nil
}
