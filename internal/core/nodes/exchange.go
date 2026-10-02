// SPDX-License-Identifier: GPL-3.0-or-later

package nodes

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock"
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
// §5.3): the command sent, the node's results handed to the job that asked by the job's id, a
// wait bounded by the core's clock, and the node told to stop on every way out that leaves it
// working (audit-2026-09-30 §3).
type Exchanges struct {
	sender Sender
	clock  clock.Clock
	logger *slog.Logger

	mutex   sync.Mutex
	waiting map[int64]*waiter // by job id; guarded by mutex
}

// ErrNodeRestarted is a node that started again while it held a job's work, which it held in
// memory and has lost (D238 §3).
var ErrNodeRestarted = errors.New("the node started again and lost the work it held")

// waiter is one job waiting on a node: the node it asked, its results as they come, and a channel
// closed when that node starts again.
type waiter struct {
	nodeID      int64
	results     chan nodeproto.Result
	interrupted chan struct{}
}

// NewExchanges returns the exchanges with nodes reached through sender, waiting on clockSource.
func NewExchanges(sender Sender, clockSource clock.Clock, logger *slog.Logger) *Exchanges {
	return &Exchanges{sender: sender, clock: clockSource, logger: logger, waiting: map[int64]*waiter{}}
}

// Ask sends a node one job's command, named by the job's id and carrying its priority, by which the
// node orders what waits (D234 §3), and waits for its results, which Next takes. The work is named
// by about in errors and the log ("prepare 3/remux/ab12"). The caller closes the exchange it
// returns; a command that could not be sent returns Send's error and nothing
// to close.
func (all *Exchanges) Ask(
	ctx context.Context, nodeID int64, job jobs.Job, command nodeproto.Command, about string,
) (*Exchange, error) {
	jobID := job.ID
	// A job's results are few (a prepare's progress, then its end), and each is taken as it comes;
	// a buffer of a few lets the node's post return without waiting on the job's loop.
	waiting := &waiter{nodeID: nodeID, results: make(chan nodeproto.Result, 8), interrupted: make(chan struct{})}
	command.JobID, command.Priority = jobID, job.Priority
	all.mutex.Lock()
	all.waiting[jobID] = waiting
	all.mutex.Unlock()
	if err := all.sender.Send(ctx, nodeID, command); err != nil {
		all.forget(jobID)
		return nil, err
	}
	return &Exchange{all: all, nodeID: nodeID, jobID: jobID, about: about, waiter: waiting}, nil
}

// Deliver hands a node's result to the job waiting for it. One nothing waits for, or one that finds
// the job's results unread, is dropped: progress is only ever the latest, and the end is sent once,
// to a job that is reading.
func (all *Exchanges) Deliver(ctx context.Context, nodeID int64, result nodeproto.Result) {
	all.mutex.Lock()
	waiting, found := all.waiting[result.JobID]
	all.mutex.Unlock()
	if !found {
		all.logger.InfoContext(ctx, "a node's result came for no waiting job", "nodeId", nodeID, "jobId", result.JobID)
		return
	}
	select {
	case waiting.results <- result:
	default:
		all.logger.DebugContext(ctx, "a node's result was dropped: its job is behind", "jobId", result.JobID)
	}
}

// Interrupt tells every job waiting on a node that it started again, losing the work it held in
// memory (D238 §3): its hello named another start than the last one this core heard.
func (all *Exchanges) Interrupt(ctx context.Context, nodeID int64) {
	all.mutex.Lock()
	defer all.mutex.Unlock()
	for jobID, waiting := range all.waiting {
		if waiting.nodeID == nodeID {
			all.logger.InfoContext(ctx, "a job's node started again, losing its work", "nodeId", nodeID, "jobId", jobID)
			close(waiting.interrupted)
			delete(all.waiting, jobID)
		}
	}
}

func (all *Exchanges) forget(jobID int64) {
	all.mutex.Lock()
	defer all.mutex.Unlock()
	delete(all.waiting, jobID)
}

// Exchange is one job's work on a node: its results as they come, and the end of the wait.
type Exchange struct {
	all           *Exchanges
	nodeID, jobID int64
	about         string
	waiter        *waiter
	silenceTimer  clock.Timer // made at the first Next
	isEndedByNode bool
}

// Next waits for the job's next result, for at most silence on the core's clock. A silence that
// runs out is ErrNodeSilent, parked on the node when it went away (Sender.Unanswered); a context
// that ends is its error, which the caller reads to tell a cancel from a shutdown; a node that
// refused the command, not knowing it, parks the job until it is updated (D233 §9); a node that
// started again, losing the work, is ErrNodeRestarted, the tool's failure the handler answers.
func (exchange *Exchange) Next(ctx context.Context, silence time.Duration) (nodeproto.Result, error) {
	if exchange.silenceTimer == nil {
		exchange.silenceTimer = exchange.all.clock.NewTimer(silence)
	} else {
		exchange.silenceTimer.Stop()
		exchange.silenceTimer.Reset(silence)
	}
	select {
	case result := <-exchange.waiter.results:
		if result.Refusal != "" {
			// The node took no part of the command: nothing to stop, and it needs an update.
			exchange.MarkEndedByNode()
			return nodeproto.Result{}, jobs.ParkOnNodeNeedingUpdate(exchange.nodeID,
				fmt.Errorf("it refused %s: %s", exchange.about, result.Refusal))
		}
		return result, nil
	case <-exchange.waiter.interrupted:
		// The node lost the work with its restart: nothing to stop.
		exchange.MarkEndedByNode()
		return nodeproto.Result{}, fmt.Errorf("node %d, during %s: %w", exchange.nodeID, exchange.about,
			ErrNodeRestarted)
	case <-exchange.silenceTimer.C():
		silent := fmt.Errorf("node %d said nothing about %s for %s: %w",
			exchange.nodeID, exchange.about, silence, ErrNodeSilent)
		return nodeproto.Result{}, exchange.all.sender.Unanswered(ctx, exchange.nodeID, silent)
	case <-ctx.Done():
		return nodeproto.Result{}, fmt.Errorf("wait for %s: %w", exchange.about, ctx.Err())
	}
}

// MarkEndedByNode records that the node said how the work ended, so Close has nothing to stop.
func (exchange *Exchange) MarkEndedByNode() {
	exchange.isEndedByNode = true
}

// Close ends the wait. Unless the node said how the work ended, it tells the node to stop it: a
// cancel, a silence, a failure after the send or a wait given up all leave the node no work the
// core no longer waits for, which it would otherwise finish and post to nothing, or run twice
// when the job is tried again.
func (exchange *Exchange) Close() {
	exchange.all.forget(exchange.jobID)
	if exchange.silenceTimer != nil {
		exchange.silenceTimer.Stop()
	}
	if !exchange.isEndedByNode {
		exchange.all.sender.Enqueue(exchange.nodeID, CancelCommand(exchange.jobID))
	}
}

// CancelCommand tells a node to stop a job it is working on (cut-a §5.3): Close sends it.
func CancelCommand(jobID int64) nodeproto.Command {
	return nodeproto.Command{Type: nodeproto.CommandCancel, JobID: jobID}
}
