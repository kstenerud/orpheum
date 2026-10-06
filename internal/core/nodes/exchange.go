// SPDX-License-Identifier: GPL-3.0-or-later

package nodes

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path"
	"sync"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/platform/randbytes"
	"github.com/kstenerud/orpheum/internal/store"
)

// Sender sends a node its work and says what a wait that ran out means: nodes.Service.
type Sender interface {
	// Send queues work for a node, and parks its job on one that is not connected.
	Send(ctx context.Context, nodeID int64, command nodeproto.Command) error
	// Ready is nil for a node work can be sent to, and parks the job on one that is not connected.
	ParkUnlessConnected(ctx context.Context, nodeID int64) error
	// Enqueue queues a command as it is: a cancel.
	Enqueue(nodeID int64, command nodeproto.Command)
	// Withdraw takes an attempt's commands the node has not taken out of its queue, and says whether
	// its work was among them: then the node never had it (S27).
	Withdraw(nodeID int64, token string) bool
	// Unanswered parks a job whose node went away while it waited, and is err otherwise.
	Unanswered(ctx context.Context, nodeID int64, err error) error
	// StartOf is the start of a node its last hello named, which each attempt given to it keeps
	// (D238 §3).
	StartOf(nodeID int64) string
}

// Rooms are what each node can run at once, and the work waiting for room on it: Service (D240 §5).
type Rooms interface {
	// Resources are what a connected node can run at once, and false for one not connected.
	Resources(ctx context.Context, nodeID int64) (Resources, bool, error)
	// Unpark brings back the work parked on a destination: work waiting for room on a node's slot or
	// device, once one of its jobs there ended.
	Unpark(ctx context.Context, destination jobs.Destination)
}

// Resources are what a node can run at once (D240 §5): its slots, the devices its roots are on, and
// how many may read each.
type Resources struct {
	Capacity     nodeproto.Capacity
	Devices      nodeproto.Devices
	ReaderLimits map[string]int
}

// readerLimit is how many may read a device at once: one for a device the core named no limit for.
func (resources Resources) readerLimit(device string) int {
	if limit, isNamed := resources.ReaderLimits[device]; isNamed {
		return limit
	}
	return 1
}

// deviceOf is the key of the device a piece of work reads, or "" for work that reads none of the
// node's media, or a node whose devices are not known.
func (resources Resources) deviceOf(work nodeproto.Work) string {
	file, isMedia := work.MediaFile()
	if !isMedia {
		return ""
	}
	return resources.Devices.KeyOf(path.Join(file.RootPath, string(file.Path)))
}

// Exchanges are the core's one way to give a node work and take its word on it (cut-a §5.3, D234
// §7): the command sent, named by an attempt token of its own, the job assigned and nothing
// waiting, the node's results recorded and resolving the job by that token, its silence bounded by
// the core's clock, and the node told to stop on every way out that leaves it working
// (audit-2026-09-30 §3).
type Exchanges struct {
	sender     Sender
	rooms      Rooms
	clock      clock.Clock
	random     io.Reader
	logger     *slog.Logger
	cache      *store.Cache
	settings   Settings
	maxSilence time.Duration

	mutex     sync.Mutex
	recorders map[nodeproto.CommandType]Recorder // guarded by mutex
	// attempts are the jobs assigned to nodes, by attempt token, mirroring cache.db's (Assign).
	attempts  map[string]*heldAttempt // guarded by mutex
	assignees map[jobs.Kind]Assignee  // guarded by mutex
	// brokenInvariants counts, by node, the differences reconciled between what it was given and held.
	brokenInvariants map[int64]int // guarded by mutex
	disks            DiskStalls    // guarded by mutex
}

// ExchangeParts are what the exchanges are built from.
type ExchangeParts struct {
	// Sender is how a node is sent its work: Service.
	Sender Sender
	Clock  clock.Clock
	// Random is where attempt tokens come from.
	Random io.Reader
	Logger *slog.Logger
	// Cache keeps the attempts given to nodes through a restart.
	Cache    *store.Cache
	Settings Settings
	// MaxSilence is how long an assigned job's node may say nothing of it before it is unanswered.
	MaxSilence time.Duration
	// Rooms are what each node can run at once: Service.
	Rooms Rooms
}

// Recorder records what a node's result on one kind of work establishes, by the content its echoed
// work names (Result.Work), and only if that is still current: a rendition by its run, a
// text by its source's hash. It is the result's one recording, whether or not a job still waits
// for it (D238 §2): a job that waits only learns how the work ended. A result whose content is no
// longer current is not recorded, and is no error.
type Recorder func(ctx context.Context, nodeID int64, result nodeproto.Result) error

// ErrMalformedResult is a result the core cannot take as it is: it names no work, or work its
// recorder cannot read.
var ErrMalformedResult = errors.New("a result names the work it answers")

// attemptTokenBytes is an attempt token's random bytes before hex: sixteen never repeat, across
// restarts of the core and a cache.db discarded, which reissues job ids (D238 §2, S84).
const attemptTokenBytes = 16

// ErrWorkLost is a node that no longer holds work the core gave it: it started again, or lost it
// otherwise. The tool's failure, not the work's (D233 §6, D238 §3).
var ErrWorkLost = errors.New("the node no longer holds the work it was given")

// ErrNotMoving is work its node says has not moved for the core's window, longer than any of the
// node's own: the node's watchdog did not catch it, so the core stops it (D233 §5). The tool's
// failure, as a stall is.
var ErrNotMoving = errors.New("the node says the work has not moved")

// MovementWindow is how long the core holds work its node says has not moved: past every window
// the node judges its own work by, the longest a subtitle track's ten minutes, and its own reading's
// five past ffprobe's timeout, so it catches only what the node's watchdog did not.
const MovementWindow = 30 * time.Minute

// UnansweredOutcome is a handler's answer to a wait for its node that ended in err. Work that
// stopped moving (ErrNotMoving) is a stall (StallOutcome). A node that no longer holds the work,
// having started again or lost it otherwise (ErrWorkLost), says nothing of the file: an update, a
// power cut, the machine's memory running out, the node's own bug lose work as surely as a file
// does, and FFmpeg failing on a file is said as FFmpeg's failure, not as work lost. So it is an
// attempt like any other's failure, tried again after a while, never kept as the file's, and a job
// its node loses on every attempt fails as the job's alone (D248, revising D233 §6 and D238 §3).
// Any other err is as it is.
func UnansweredOutcome(job jobs.Job, err error, keepFailure func(failure string) error) error {
	if errors.Is(err, ErrNotMoving) {
		return StallOutcome(job, err, jobs.StalledTwice, keepFailure)
	}
	return err
}

// StallOutcome is a handler's answer to a node's word that the work stalled and was stopped: a
// hang says nothing of the target the first time, so the job is tried again at no cost of an
// attempt (jobs.MarkStalled), and when its last attempt's tool failed too it is the target's
// failure, begun with secondStallPrefix and kept by keepFailure when it is not nil (D233 §6). A second stall on
// its disk is begun with UnreadableOnDisk instead (D233 §10).
func StallOutcome(job jobs.Job, stalled error, secondStallPrefix string, keepFailure func(failure string) error) error {
	if !job.PreviousAttemptToolFailed {
		return jobs.MarkStalled(stalled)
	}
	if errors.Is(stalled, ErrStalledOnDisk) {
		secondStallPrefix = UnreadableOnDisk
	}
	if keepFailure == nil {
		return jobs.MarkPermanent(fmt.Errorf("%s%w", secondStallPrefix, stalled))
	}
	failure := secondStallPrefix + stalled.Error()
	return jobs.MarkPermanent(errors.Join(errors.New(failure), keepFailure(failure)))
}

// ErrNodeRestarted is a node that started again while it held a job's work, which it held in
// memory and has lost (D238 §3).
var ErrNodeRestarted = fmt.Errorf("the node started again: %w", ErrWorkLost)

// NewExchanges returns the exchanges with nodes.
func NewExchanges(parts ExchangeParts) *Exchanges {
	return &Exchanges{
		sender: parts.Sender, clock: parts.Clock, random: parts.Random, logger: parts.Logger, cache: parts.Cache,
		settings: parts.Settings, maxSilence: parts.MaxSilence, rooms: parts.Rooms,
		recorders: map[nodeproto.CommandType]Recorder{},
		attempts:  map[string]*heldAttempt{}, assignees: map[jobs.Kind]Assignee{},
		brokenInvariants: map[int64]int{},
	}
}

// newToken is a new attempt token.
func (all *Exchanges) newToken() (string, error) {
	raw, err := randbytes.Read(all.random, attemptTokenBytes)
	if err != nil {
		return "", fmt.Errorf("name an attempt: %w", err)
	}
	return hex.EncodeToString(raw), nil
}

// Record names the recorder of one kind of work's results. Set each before the channel serves.
func (all *Exchanges) Record(kind nodeproto.CommandType, recorder Recorder) {
	all.mutex.Lock()
	defer all.mutex.Unlock()
	all.recorders[kind] = recorder
}

// Deliver records a node's result by what it is, then resolves the job it is an attempt at: the one
// its attempt token names, given to that node (S83, S84). The node's result is acknowledged once
// Deliver returns nil, so it is recorded first, whether or not its job is still assigned (S81); an
// error leaves it unacknowledged, and the node sends it again. A refusal, the node taking no part,
// is recorded by nothing. A result for no attempt this core gave, from another node or another
// attempt, resolves nothing.
func (all *Exchanges) Deliver(ctx context.Context, nodeID int64, result nodeproto.Result) error {
	if result.Work == nil {
		return fmt.Errorf("job %d's result from node %d: %w", result.JobID, nodeID, ErrMalformedResult)
	}
	all.mutex.Lock()
	record, isRecorded := all.recorders[result.Work.Type]
	all.mutex.Unlock()
	if isRecorded && result.Refusal == "" && result.Crashed == "" {
		if err := record(ctx, nodeID, result); err != nil {
			return fmt.Errorf("record job %d's result from node %d: %w", result.JobID, nodeID, err)
		}
	}
	held, isHeld, err := all.attemptOf(nodeID, result.AttemptToken)
	if err != nil {
		return err
	}
	if !isHeld {
		all.logger.InfoContext(ctx, "a node's result came for no attempt given to it", "nodeId", nodeID,
			"jobId", result.JobID, "attemptToken", result.AttemptToken)
		return nil
	}
	return all.resolveResult(ctx, held.Attempt, held.assignee, result)
}

// DeliverProgress takes a node's word on where an attempt is, by its token and from its node, as
// Deliver does a result. Progress is best effort: one for no attempt given to the node is dropped.
func (all *Exchanges) DeliverProgress(ctx context.Context, nodeID int64, progress nodeproto.Progress) {
	if !all.takeProgress(ctx, nodeID, progress) {
		all.logger.DebugContext(ctx, "a node's progress came for no attempt given to it", "nodeId", nodeID,
			"jobId", progress.JobID, "attemptToken", progress.AttemptToken)
	}
}

// Interrupt resolves every attempt given to a node under another start than startID, the one its
// hello names now: the node started again, losing the work it held in memory (D238 §3). Each
// attempt keeps its node's start in cache.db, so this holds whether or not the core started again
// too. Their commands not yet taken are withdrawn, so the node that started again is not sent work
// whose job ended (G4).
func (all *Exchanges) Interrupt(ctx context.Context, nodeID int64, startID string) {
	all.unanswered(ctx, func(held heldAttempt) bool {
		return held.NodeID == nodeID && held.NodeStartID != startID
	}, func(attempt Attempt) error {
		all.sender.Withdraw(attempt.NodeID, attempt.Token)
		all.logger.InfoContext(ctx, "a job's node started again, losing its work", "nodeId", nodeID,
			"jobId", attempt.JobID, "attemptToken", attempt.Token)
		return fmt.Errorf("node %d, during %s job %d: %w", nodeID, attempt.Kind, attempt.JobID, ErrNodeRestarted)
	}, false)
}

// ReprioritizeCommand gives an attempt its job's new priority, play-driven or not.
func ReprioritizeCommand(jobID int64, token string, priority int64) nodeproto.Command {
	return nodeproto.Command{
		JobID: jobID, AttemptToken: token, Priority: priority, IsPlayDriven: priority >= jobs.PriorityPlayNow,
		Work: nodeproto.Work{Type: nodeproto.CommandReprioritize},
	}
}

// CancelCommand tells a node to stop the attempt at a job it is working on (cut-a §5.3), named by
// its token: Close sends it.
func CancelCommand(jobID int64, token string) nodeproto.Command {
	return nodeproto.Command{JobID: jobID, AttemptToken: token, Work: nodeproto.Work{Type: nodeproto.CommandCancel}}
}
