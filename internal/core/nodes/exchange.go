// SPDX-License-Identifier: GPL-3.0-or-later

package nodes

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
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
	// Enqueue queues a command as it is: a cancel.
	Enqueue(nodeID int64, command nodeproto.Command)
	// Unanswered parks a job whose node went away while it waited, and is err otherwise.
	Unanswered(ctx context.Context, nodeID int64, err error) error
}

// Exchanges are the core's one way to ask a node for work and wait for what it says (cut-a
// §5.3): the command sent, named by an attempt token of its own, the node's results handed to the
// job that asked by that token, a wait bounded by the core's clock, and the node told to stop on
// every way out that leaves it working (audit-2026-09-30 §3).
type Exchanges struct {
	sender     Sender
	clock      clock.Clock
	random     io.Reader
	logger     *slog.Logger
	cache      *store.Cache
	settings   Settings
	maxSilence time.Duration

	mutex     sync.Mutex
	waiting   map[string]*waiter                 // by attempt token; guarded by mutex
	recorders map[nodeproto.CommandType]Recorder // guarded by mutex
	// attempts are the jobs assigned to nodes, by attempt token, mirroring cache.db's (Assign).
	attempts  map[string]*heldAttempt // guarded by mutex
	assignees map[jobs.Kind]Assignee  // guarded by mutex
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

// ErrNodeRestarted is a node that started again while it held a job's work, which it held in
// memory and has lost (D238 §3).
var ErrNodeRestarted = errors.New("the node started again and lost the work it held")

// waiter is one job waiting on a node: the node it asked, its results and its progress as they
// come, and a channel closed when that node starts again.
type waiter struct {
	nodeID      int64
	results     chan nodeproto.Result
	progress    chan nodeproto.Progress
	interrupted chan struct{}
}

// NewExchanges returns the exchanges with nodes.
func NewExchanges(parts ExchangeParts) *Exchanges {
	return &Exchanges{
		sender: parts.Sender, clock: parts.Clock, random: parts.Random, logger: parts.Logger, cache: parts.Cache,
		settings: parts.Settings, maxSilence: parts.MaxSilence,
		waiting: map[string]*waiter{}, recorders: map[nodeproto.CommandType]Recorder{},
		attempts: map[string]*heldAttempt{}, assignees: map[jobs.Kind]Assignee{},
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

// Ask sends a node one job's command, named by the job's id and an attempt token made for this
// sending, and carrying the job's priority, by which the node orders what waits (D234 §3); it
// waits for the results that name the token, from that node only, which Next takes. The work is named
// by about in errors and the log ("prepare 3/remux/ab12"). The caller closes the exchange it
// returns; a command that could not be sent returns Send's error and nothing
// to close.
func (all *Exchanges) Ask(
	ctx context.Context, nodeID int64, job jobs.Job, command nodeproto.Command, about string,
) (*Exchange, error) {
	token, err := all.newToken()
	if err != nil {
		return nil, fmt.Errorf("ask for %s: %w", about, err)
	}
	// A job's result is one, its end; its progress comes every few seconds and is taken as it
	// comes. A buffer of a few lets the node's post return without waiting on the job's loop.
	waiting := &waiter{
		nodeID: nodeID, results: make(chan nodeproto.Result, 1), progress: make(chan nodeproto.Progress, 8),
		interrupted: make(chan struct{}),
	}
	command.JobID, command.AttemptToken, command.Priority = job.ID, token, job.Priority
	all.mutex.Lock()
	all.waiting[token] = waiting
	all.mutex.Unlock()
	if err := all.sender.Send(ctx, nodeID, command); err != nil {
		all.forget(token)
		return nil, err
	}
	return &Exchange{all: all, nodeID: nodeID, jobID: job.ID, token: token, about: about, waiter: waiting}, nil
}

// Deliver records a node's result by what it is, then hands it to the job waiting for it: the one
// its attempt token names, waiting on that node (S83, S84). The node's result is acknowledged once
// Deliver returns nil, so it is recorded first, whether or not a job still waits (S81); an error
// leaves it unacknowledged, and the node sends it again. A refusal, the node taking no part, is
// recorded by nothing. A result no job waits for, from another node or another attempt, reaches no
// job.
func (all *Exchanges) Deliver(ctx context.Context, nodeID int64, result nodeproto.Result) error {
	if result.Work == nil {
		return fmt.Errorf("job %d's result from node %d: %w", result.JobID, nodeID, ErrMalformedResult)
	}
	all.mutex.Lock()
	record, isRecorded := all.recorders[result.Work.Type]
	waiting, found := all.waiting[result.AttemptToken]
	all.mutex.Unlock()
	if isRecorded && result.Refusal == "" {
		if err := record(ctx, nodeID, result); err != nil {
			return fmt.Errorf("record job %d's result from node %d: %w", result.JobID, nodeID, err)
		}
	}
	held, isHeld, err := all.attemptOf(nodeID, result.AttemptToken)
	if err != nil {
		return err
	}
	if isHeld {
		return all.resolveResult(ctx, held.Attempt, held.assignee, result)
	}
	if !found || waiting.nodeID != nodeID {
		all.logger.InfoContext(ctx, "a node's result came for no job waiting on it", "nodeId", nodeID,
			"jobId", result.JobID, "attemptToken", result.AttemptToken)
		return nil
	}
	select {
	case waiting.results <- result:
	default:
		all.logger.DebugContext(ctx, "a node's result was dropped: its job has one", "jobId", result.JobID)
	}
	return nil
}

// DeliverProgress hands a node's word on where an attempt is to the job waiting for it, by its
// token and from its node, as Deliver does a result. Progress is best effort: one nothing waits
// for, or one that finds the job's progress unread, is dropped, since the newest supersedes it.
func (all *Exchanges) DeliverProgress(ctx context.Context, nodeID int64, progress nodeproto.Progress) {
	if all.takeProgress(ctx, nodeID, progress) {
		return
	}
	all.mutex.Lock()
	waiting, found := all.waiting[progress.AttemptToken]
	all.mutex.Unlock()
	if !found || waiting.nodeID != nodeID {
		all.logger.DebugContext(ctx, "a node's progress came for no job waiting on it", "nodeId", nodeID,
			"jobId", progress.JobID, "attemptToken", progress.AttemptToken)
		return
	}
	select {
	case waiting.progress <- progress:
	default:
	}
}

// Interrupt tells every job waiting on a node that it started again, losing the work it held in
// memory (D238 §3): its hello named another start than the last one this core heard.
func (all *Exchanges) Interrupt(ctx context.Context, nodeID int64) {
	all.unanswered(ctx, func(held heldAttempt) bool { return held.NodeID == nodeID }, func(attempt Attempt) error {
		all.logger.InfoContext(ctx, "a job's node started again, losing its work", "nodeId", nodeID,
			"jobId", attempt.JobID, "attemptToken", attempt.Token)
		return fmt.Errorf("node %d, during %s job %d: %w", nodeID, attempt.Kind, attempt.JobID, ErrNodeRestarted)
	}, false)
	all.mutex.Lock()
	defer all.mutex.Unlock()
	for token, waiting := range all.waiting {
		if waiting.nodeID == nodeID {
			all.logger.InfoContext(ctx, "a job's node started again, losing its work", "nodeId", nodeID,
				"attemptToken", token)
			close(waiting.interrupted)
			delete(all.waiting, token)
		}
	}
}

func (all *Exchanges) forget(token string) {
	all.mutex.Lock()
	defer all.mutex.Unlock()
	delete(all.waiting, token)
}

// Exchange is one job's work on a node: its results as they come, and the end of the wait.
type Exchange struct {
	all           *Exchanges
	nodeID, jobID int64
	token         string
	about         string
	waiter        *waiter
	silenceTimer  clock.Timer // made at the first Next
	isEndedByNode bool
	// OnProgress, when set, takes each word on where the work is; an error from it ends the wait
	// (Next), with the node told to stop the work when the exchange closes. Progress keeps the wait
	// from running out whether or not it is set.
	OnProgress func(progress nodeproto.Progress) error
}

// Next waits for how the job's work ended, for at most silence on the core's clock without a word
// from the node: each progress it hears starts the silence again, and goes to OnProgress. A silence
// that runs out is ErrNodeSilent, parked on the node when it went away (Sender.Unanswered); a context
// that ends is its error, which the caller reads to tell a cancel from a shutdown; a node that
// refused the command, not knowing it, parks the job until it is updated (D233 §9); a node that
// started again, losing the work, is ErrNodeRestarted, the tool's failure the handler answers.
func (exchange *Exchange) Next(ctx context.Context, silence time.Duration) (nodeproto.Result, error) {
	for {
		exchange.startSilence(silence)
		result, progress, err := exchange.nextWord(ctx, silence)
		if progress == nil {
			return result, err
		}
		if exchange.OnProgress != nil {
			if err := exchange.OnProgress(*progress); err != nil {
				return nodeproto.Result{}, err
			}
		}
	}
}

// startSilence starts the wait for the node's next word.
func (exchange *Exchange) startSilence(silence time.Duration) {
	if exchange.silenceTimer == nil {
		exchange.silenceTimer = exchange.all.clock.NewTimer(silence)
		return
	}
	exchange.silenceTimer.Stop()
	exchange.silenceTimer.Reset(silence)
}

// nextWord is the node's next word on the job: its progress, or its result, or why the wait ended.
func (exchange *Exchange) nextWord(
	ctx context.Context, silence time.Duration,
) (nodeproto.Result, *nodeproto.Progress, error) {
	select {
	case progress := <-exchange.waiter.progress:
		return nodeproto.Result{}, &progress, nil
	case result := <-exchange.waiter.results:
		// However it ended, the node is done with the work: nothing to stop.
		exchange.markEndedByNode()
		if result.Refusal != "" {
			// The node took no part of the command, and it needs an update.
			return nodeproto.Result{}, nil, jobs.ParkOnNodeNeedingUpdate(exchange.nodeID,
				fmt.Errorf("it refused %s: %s", exchange.about, result.Refusal))
		}
		return result, nil, nil
	case <-exchange.waiter.interrupted:
		// The node lost the work with its restart: nothing to stop.
		exchange.markEndedByNode()
		return nodeproto.Result{}, nil, fmt.Errorf("node %d, during %s: %w", exchange.nodeID, exchange.about,
			ErrNodeRestarted)
	case <-exchange.silenceTimer.C():
		silent := fmt.Errorf("node %d said nothing about %s for %s: %w",
			exchange.nodeID, exchange.about, silence, ErrNodeSilent)
		return nodeproto.Result{}, nil, exchange.all.sender.Unanswered(ctx, exchange.nodeID, silent)
	case <-ctx.Done():
		return nodeproto.Result{}, nil, fmt.Errorf("wait for %s: %w", exchange.about, ctx.Err())
	}
}

// markEndedByNode records that the node is done with the work, so Close has nothing to stop.
func (exchange *Exchange) markEndedByNode() {
	exchange.isEndedByNode = true
}

// Close ends the wait. Unless the node said how the work ended, it tells the node to stop it: a
// cancel, a silence, a failure after the send or a wait given up all leave the node no work the
// core no longer waits for, which it would otherwise finish and post to nothing, or run twice
// when the job is tried again.
func (exchange *Exchange) Close() {
	exchange.all.forget(exchange.token)
	if exchange.silenceTimer != nil {
		exchange.silenceTimer.Stop()
	}
	if !exchange.isEndedByNode {
		exchange.all.sender.Enqueue(exchange.nodeID, CancelCommand(exchange.jobID, exchange.token))
	}
}

// CancelCommand tells a node to stop the attempt at a job it is working on (cut-a §5.3), named by
// its token: Close sends it.
func CancelCommand(jobID int64, token string) nodeproto.Command {
	return nodeproto.Command{JobID: jobID, AttemptToken: token, Work: nodeproto.Work{Type: nodeproto.CommandCancel}}
}
