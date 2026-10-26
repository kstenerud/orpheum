// SPDX-License-Identifier: AGPL-3.0-or-later

package jobs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
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
	// KindExtractSubtitle extracts the text subtitle tracks of one file not yet extracted, a video's
	// from one read of it, or a subtitle file's one; its target is the file (D217 §3, D340).
	KindExtractSubtitle Kind = "extract_subtitle"
	// KindConvertFetchedSubtitle converts a subtitle fetched on request; its target is the
	// fetched subtitle (D224 §5).
	KindConvertFetchedSubtitle Kind = "convert_fetched_subtitle"
	// KindReadThrough reads a video through once, for its audio tracks' loudness and its picture's
	// timeline, whichever its bytes lack; its target is the file (D226 §2, D247 §10).
	KindReadThrough Kind = "read_through"
	// KindMakeChunks makes one run of chunks of one stream of a film, on its node, for a player
	// waiting for them; its target is the chunk store, the stream of the film on its grid, so the runs
	// of a film's streams queue side by side, and its arguments name the chunks (D247 §10).
	KindMakeChunks Kind = "make_chunks"
	// KindScan lists a root for its scan and compares what it found; its target is the scan (D240).
	KindScan Kind = "scan"
	// KindExamine reads a batch of the files a scan found new or changed; its target is the batch
	// (D240 §8).
	KindExamine Kind = "examine"
	// KindBackup backs the persistent directory up; it names no target (D243 §11).
	KindBackup Kind = "backup"
	// KindRenewCertificate obtains or renews the server's HTTPS certificate; it names no target
	// (D243 §6).
	KindRenewCertificate Kind = "renew_certificate"
)

// Kinds are every kind of job, as the job view documents them: the one list, which the API's
// description of a job is made from.
func Kinds() []Kind {
	return []Kind{
		KindFetchMetadata, KindMakeArtworkSizes, KindExtractSubtitle, KindConvertFetchedSubtitle,
		KindReadThrough, KindMakeChunks, KindScan, KindExamine, KindBackup, KindRenewCertificate,
	}
}

// Target is what a job works on, by its kind (D227 §2): an item for fetch and artwork, a file
// for extract_subtitle, a fetched subtitle for
// convert_fetched_subtitle, a file for read_through, a chunk store for make_chunks, a scan for scan
// and a scan's batch for examine. A job is queued with exactly one set and the others zero, but a backup and the
// certificate's renewal, which work on the whole server and name none (D243 §6, §11): one of each
// at a time, since every such job of a kind is one target. Enqueue refuses any other target with ErrNotOneTarget.
type Target = store.JobTarget

// ErrNotOneTarget is a job that names no target, or more than one; or a backup that names one.
var ErrNotOneTarget = errors.New("a job works on one target, and a backup on none")

// isWholeServer is whether a kind works on the whole server and names no target: a backup and the
// certificate's renewal, one of each at a time.
func isWholeServer(kind Kind) bool {
	return kind == KindBackup || kind == KindRenewCertificate
}

// requireOneTarget refuses a target that names no target or several, but for a backup, which
// names none.
func requireOneTarget(kind Kind, target Target) error {
	setCount := 0
	for _, id := range []int64{
		target.ItemID, target.StreamID, target.FetchedSubtitleID, target.FileID,
		target.ScanID, target.ScanBatchID, target.ChunkStoreID,
	} {
		if id != 0 {
			setCount++
		}
	}
	want := 1
	if isWholeServer(kind) {
		want = 0
	}
	if setCount != want {
		return fmt.Errorf("%w: %s %+v", ErrNotOneTarget, kind, target)
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
	PriorityPlayNow int64 = 1000
	// PriorityBackground is work no person waits for: a sweep's, and play-driven work past its
	// "wanted until".
	PriorityBackground int64 = 0
	// PriorityAheadOfSweep is work no person waits for that goes before a sweep's: a film whose file
	// has no index of its keyframes, read through for them, so most are known before anyone plays
	// them (D269 §4).
	PriorityAheadOfSweep int64 = 1
	// PriorityIndexFirst is the read of a file's index alone, milliseconds, before any full read:
	// the fast tier, so most films are cut on their keyframes before anyone plays them (D269 §5).
	PriorityIndexFirst int64 = 2
	// PriorityBringsBack is the scan of a library root that is unavailable, which brings its films
	// back: everyone using the library waits on it, so it goes before the queue's own work. Behind a
	// routine scan of a larger root on the same disk, it waited a minute for the disk's one reader
	// (staging, 2026-10-21).
	PriorityBringsBack     int64 = 10
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
	// IsDerived is a request a sweep made (Sweep): it looks at no parked destination, whatever
	// its priority, since no one is waiting on it now.
	IsDerived bool
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
	queue    *jobqueue.Queue[Target]
	clock    clock.Clock
	settings Settings

	reportedProgressMutex sync.Mutex
	reportedProgress      ReportedProgress // guarded by reportedProgressMutex: nil until set

	// wanted are the plays' wishes for their work not yet written: KeepWanted keeps each here, so
	// a play's request writes nothing (D303, DL10), and DemoteUnwanted writes them first.
	wantedMutex sync.Mutex
	wanted      map[wantedWork]time.Time // guarded by wantedMutex: until when each is wanted
}

// wantedWork is the work a play waits for: a kind's job on a target.
type wantedWork struct {
	kind   Kind
	target Target
}

// NewQueue returns the queue. A person's request probes its parked destination at once, and
// cache.db and persistent.db failing for want of room or with an I/O error park the core's storage
// (D231 §5, §6).
func NewQueue(parts QueueParts) *Queue {
	return newQueue(parts.Cache.Jobs(), parts)
}

// newQueue is the queue over storage: cache.db's job table, or a test's.
func newQueue(storage jobqueue.Storage[Target], parts QueueParts) *Queue {
	queue := jobqueue.New(storage, parts.Clock, jobqueue.Config{
		Policy: settingsPolicy{settings: parts.Settings}, Probes: settingsPolicy{settings: parts.Settings},
		Random: parts.Random, Logger: parts.Logger, ArgsMerges: parts.ArgsMerges,
		PersonsRequestPriority: PriorityPersonsRequest,
		StorageDestination:     DestinationCoreStorage, IsStorageFailure: store.IsStorageFailure,
	})
	return &Queue{clock: parts.Clock, settings: parts.Settings, queue: queue}
}

// Enqueue queues a job, or folds it into the one already open for its kind and target
// (jobqueue.Queue.Enqueue says how), and reports whether it added one.
func (queue *Queue) Enqueue(ctx context.Context, job Request) (bool, error) {
	if err := requireOneTarget(job.Kind, job.Target); err != nil {
		return false, err
	}
	action, err := queue.queue.Enqueue(ctx, jobqueue.Request[Target]{
		Kind: job.Kind, Target: job.Target, ArgsJSON: job.ArgsJSON, Priority: job.Priority, Reason: job.Reason,
		RunnableAt: job.RunnableAt, Destination: job.Destination, IsDerived: job.IsDerived,
	})
	if err != nil {
		return false, fmt.Errorf("enqueue: %w", err)
	}
	if job.Priority >= PriorityPlayNow {
		if err := queue.KeepWanted(ctx, job.Kind, job.Target); err != nil {
			return false, err
		}
	}
	return action == jobqueue.ActionInsert, nil
}

// EnqueueBatch is Enqueue for each job, a batch of them in each write: a sweep queues one per file
// of a library, each of which took the writer apart (the audit of 2026-10-25, S4). It says how many
// it queued anew.
func (queue *Queue) EnqueueBatch(ctx context.Context, batch []Request) (int, error) {
	requests := make([]jobqueue.Request[Target], 0, len(batch))
	for _, job := range batch {
		if err := requireOneTarget(job.Kind, job.Target); err != nil {
			return 0, err
		}
		requests = append(requests, jobqueue.Request[Target]{
			Kind: job.Kind, Target: job.Target, ArgsJSON: job.ArgsJSON, Priority: job.Priority, Reason: job.Reason,
			RunnableAt: job.RunnableAt, Destination: job.Destination, IsDerived: job.IsDerived,
		})
	}
	actions, err := queue.queue.EnqueueBatch(ctx, requests)
	inserted := 0
	for index, action := range actions {
		if action == jobqueue.ActionInsert {
			inserted++
		}
		if job := batch[index]; job.Priority >= PriorityPlayNow {
			if keepErr := queue.KeepWanted(ctx, job.Kind, job.Target); keepErr != nil {
				return inserted, keepErr
			}
		}
	}
	if err != nil {
		return inserted, fmt.Errorf("enqueue: %w", err)
	}
	return inserted, nil
}

// KeepWanted says a person still waits for kind's open job on target: it keeps its play priority
// for play_priority_kept_seconds more (D234 §3). A play asks again each time it looks for the work.
//
// It writes nothing: a play asks each time it is served a chunk, several times a second, and the
// wish is kept in memory until DemoteUnwanted writes it, every few seconds against a wait of
// minutes (D303, DL10).
func (queue *Queue) KeepWanted(ctx context.Context, kind Kind, target Target) error {
	wanted, err := queue.settings.Duration(ctx, settings.PlayPriorityKeptSeconds)
	if err != nil {
		return fmt.Errorf("read play_priority_kept_seconds: %w", err)
	}
	until := queue.clock.Now().Add(wanted)
	queue.wantedMutex.Lock()
	defer queue.wantedMutex.Unlock()
	if queue.wanted == nil {
		queue.wanted = map[wantedWork]time.Time{}
	}
	work := wantedWork{kind: kind, target: target}
	if until.After(queue.wanted[work]) {
		queue.wanted[work] = until
	}
	return nil
}

// writeWanted writes the plays' wishes kept since the last time; one it could not write is kept
// for the next.
func (queue *Queue) writeWanted(ctx context.Context) error {
	queue.wantedMutex.Lock()
	pending := queue.wanted
	queue.wanted = nil
	queue.wantedMutex.Unlock()
	var failed error
	for work, until := range pending {
		err := queue.queue.KeepWanted(ctx, work.kind, work.target, until)
		if err == nil {
			continue
		}
		failed = err
		queue.wantedMutex.Lock()
		if queue.wanted == nil {
			queue.wanted = map[wantedWork]time.Time{}
		}
		if until.After(queue.wanted[work]) {
			queue.wanted[work] = until
		}
		queue.wantedMutex.Unlock()
	}
	return failed
}

// ReleaseWanted says no person waits for kind's open job on target any more, as one who left the
// page that asked for it: it falls to background priority at the next demotion (D258 §2).
func (queue *Queue) ReleaseWanted(ctx context.Context, kind Kind, target Target) error {
	// A wish not yet written is written first: the release ends a wait the job has a record of,
	// and a job queued at play priority with none would never be demoted.
	work := wantedWork{kind: kind, target: target}
	queue.wantedMutex.Lock()
	until, isPending := queue.wanted[work]
	delete(queue.wanted, work)
	queue.wantedMutex.Unlock()
	if isPending {
		if err := queue.queue.KeepWanted(ctx, kind, target, until); err != nil {
			return err
		}
	}
	return queue.queue.ReleaseWanted(ctx, kind, target)
}

// Demoted is a job no person waits for any more, fallen to background priority.
type Demoted struct {
	ID int64
	// IsAssigned is a job given to a node, which is told its new priority.
	IsAssigned bool
}

// DemoteUnwanted gives the open jobs no person waits for any more the rung above the background's,
// PriorityShownInterest: each was wanted by someone (D234 §3, S87; D324), and returns them.
func (queue *Queue) DemoteUnwanted(ctx context.Context) ([]Demoted, error) {
	if err := queue.writeWanted(ctx); err != nil {
		return nil, err
	}
	rows, err := queue.queue.DemoteUnwanted(ctx, PriorityShownInterest)
	if err != nil {
		return nil, err
	}
	demoted := make([]Demoted, 0, len(rows))
	for _, row := range rows {
		demoted = append(demoted, Demoted{ID: row.ID, IsAssigned: row.State == jobqueue.StateAssigned})
	}
	return demoted, nil
}

// PruneFinished forgets the jobs that ended longer ago than finished_jobs_kept_days, and says how
// many (D233 §8).
func (queue *Queue) PruneFinished(ctx context.Context) (int64, error) {
	keptFor, err := queue.settings.Duration(ctx, settings.FinishedJobsKeptDays)
	if err != nil {
		return 0, fmt.Errorf("read finished_jobs_kept_days: %w", err)
	}
	return queue.queue.PruneFinished(ctx, keptFor)
}

// RequeueRunning returns the jobs a stop left running to the queue: their output is a cache, so
// they restart from zero (cut-a §5.3). Each keeps its counted attempt (S86).
func (queue *Queue) RequeueRunning(ctx context.Context) (int64, error) {
	return queue.queue.RequeueRunning(ctx)
}

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

// LongestRetryDelay is job_longest_retry_delay_hours.
func (policy settingsPolicy) LongestRetryDelay(ctx context.Context) (time.Duration, error) {
	longest, err := policy.settings.Duration(ctx, settings.JobLongestRetryDelayHours)
	if err != nil {
		return 0, fmt.Errorf("read job_longest_retry_delay_hours: %w", err)
	}
	return longest, nil
}

// MarkStalled marks a handler's error as a tool that stalled on the job, a hang that says nothing
// of the target: the job is tried again after job_first_retry_delay_seconds at no cost of an
// attempt, and its next attempt knows the last one's tool failed (Job.PreviousAttemptToolFailed),
// so its handler can make a second stall in a row the target's failure. D231's item facts
// replace that.
func MarkStalled(err error) error { return jobqueue.ToolFailed(fmt.Errorf("stalled: %w", err)) }

// StalledTwice begins the failure of a target whose work stalled on the second of two attempts in a
// row whose tool failed (D233 §6).
const StalledTwice = "stalled on two attempts in a row: "

// MarkPermanent marks a handler's error as one no retry can fix.
func MarkPermanent(err error) error { return jobqueue.Permanent(err) }

// IsMarkedPermanent is whether a handler's error was marked as one no retry can fix.
func IsMarkedPermanent(err error) bool { return jobqueue.IsPermanent(err) }

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
