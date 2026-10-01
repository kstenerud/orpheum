// SPDX-License-Identifier: GPL-3.0-or-later

package nodes

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

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
	waiting map[int64]chan nodeproto.Result // by job id; guarded by mutex
}

// NewExchanges returns the exchanges with nodes reached through sender, waiting on clockSource.
func NewExchanges(sender Sender, clockSource clock.Clock, logger *slog.Logger) *Exchanges {
	return &Exchanges{sender: sender, clock: clockSource, logger: logger, waiting: map[int64]chan nodeproto.Result{}}
}

// Ask sends a node one job's command and waits for its results, which Next takes. The work is
// named by about in errors and the log ("prepare 3/remux/ab12"). The caller closes the exchange it returns;
// a command that could not be sent returns Send's error and nothing to close.
func (all *Exchanges) Ask(
	ctx context.Context, nodeID, jobID int64, command nodeproto.Command, about string,
) (*Exchange, error) {
	// A job's results are few (a prepare's progress, then its end), and each is taken as it comes;
	// a buffer of a few lets the node's post return without waiting on the job's loop.
	results := make(chan nodeproto.Result, 8)
	all.mutex.Lock()
	all.waiting[jobID] = results
	all.mutex.Unlock()
	if err := all.sender.Send(ctx, nodeID, command); err != nil {
		all.forget(jobID)
		return nil, err
	}
	return &Exchange{all: all, nodeID: nodeID, jobID: jobID, about: about, results: results}, nil
}

// Deliver hands a node's result to the job waiting for it. One nothing waits for, or one that finds
// the job's results unread, is dropped: progress is only ever the latest, and the end is sent once,
// to a job that is reading.
func (all *Exchanges) Deliver(ctx context.Context, nodeID int64, result nodeproto.Result) {
	all.mutex.Lock()
	results, found := all.waiting[result.JobID]
	all.mutex.Unlock()
	if !found {
		all.logger.InfoContext(ctx, "a node's result came for no waiting job", "nodeId", nodeID, "jobId", result.JobID)
		return
	}
	select {
	case results <- result:
	default:
		all.logger.DebugContext(ctx, "a node's result was dropped: its job is behind", "jobId", result.JobID)
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
	results       chan nodeproto.Result
	silenceTimer  clock.Timer // made at the first Next
	isEndedByNode bool
}

// Next waits for the job's next result, for at most silence on the core's clock. A silence that
// runs out is ErrNodeSilent, parked on the node when it went away (Sender.Unanswered); a context
// that ends is its error, which the caller reads to tell a cancel from a shutdown.
func (exchange *Exchange) Next(ctx context.Context, silence time.Duration) (nodeproto.Result, error) {
	if exchange.silenceTimer == nil {
		exchange.silenceTimer = exchange.all.clock.NewTimer(silence)
	} else {
		exchange.silenceTimer.Stop()
		exchange.silenceTimer.Reset(silence)
	}
	select {
	case result := <-exchange.results:
		return result, nil
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
	return nodeproto.Command{Type: nodeproto.CommandCancel, Cancel: &nodeproto.Cancel{JobID: jobID}}
}
