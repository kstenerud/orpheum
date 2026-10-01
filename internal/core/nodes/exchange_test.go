// SPDX-License-Identifier: GPL-3.0-or-later

package nodes_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// recordingSender is a node that keeps what it was sent, and whose Unanswered says the node went
// away when isGone.
type recordingSender struct {
	sent   chan nodeproto.Command
	isGone bool
}

var errParked = errors.New("parked on the node")

func (sender *recordingSender) Send(_ context.Context, _ int64, command nodeproto.Command) error {
	sender.sent <- command
	return nil
}

func (sender *recordingSender) Enqueue(_ int64, command nodeproto.Command) { sender.sent <- command }

func (sender *recordingSender) Unanswered(_ context.Context, _ int64, err error) error {
	if sender.isGone {
		return errors.Join(errParked, err)
	}
	return err
}

// exchangeBench is exchanges over a recording node, on a fake clock, logging to a buffer.
type exchangeBench struct {
	exchanges *nodes.Exchanges
	sender    *recordingSender
	now       *clocktest.Fake
	logged    *bytes.Buffer
}

func newExchanges(t *testing.T) exchangeBench {
	t.Helper()
	bench := exchangeBench{
		sender: &recordingSender{sent: make(chan nodeproto.Command, 4)},
		now:    clocktest.NewFake(time.Date(2026, time.October, 1, 12, 0, 0, 0, time.UTC)),
		logged: &bytes.Buffer{},
	}
	bench.exchanges = nodes.NewExchanges(bench.sender, bench.now, slog.New(slog.NewJSONHandler(bench.logged, nil)))
	return bench
}

// askAndWait asks for job 7's prepare, and waits for its next result on another goroutine, whose
// error it returns on the channel.
func (bench exchangeBench) askAndWait(ctx context.Context, t *testing.T) (*nodes.Exchange, <-chan error) {
	t.Helper()
	exchange, err := bench.exchanges.Ask(ctx, 1, 7, prepareCommand, "prepare 7")
	if err != nil {
		t.Fatal(err)
	}
	testutil.Receive(t, bench.sender.sent, "the prepare command")
	waited := make(chan error, 1)
	go func() {
		_, nextErr := exchange.Next(ctx, time.Minute)
		waited <- nextErr
	}()
	return exchange, waited
}

// wantToldToStop checks that closing the exchange tells the node to stop job 7.
func (bench exchangeBench) wantToldToStop(t *testing.T, exchange *nodes.Exchange) {
	t.Helper()
	exchange.Close()
	stop := testutil.Receive(t, bench.sender.sent, "the cancel")
	if stop.Type != nodeproto.CommandCancel || stop.JobID != 7 {
		t.Errorf("the node was sent %+v, want it told to stop job 7", stop)
	}
}

var prepareCommand = nodeproto.Command{Type: nodeproto.CommandPrepare, Prepare: &nodeproto.Prepare{}}

// A result reaches the job that asked, by its id, and once the node said how the work ended,
// closing the exchange sends it nothing more.
func TestExchange_AResultReachesTheJobThatAskedAndAFinishedOneIsNotCancelled(t *testing.T) {
	t.Parallel()
	bench := newExchanges(t)
	exchanges, sender := bench.exchanges, bench.sender
	exchange, err := exchanges.Ask(t.Context(), 1, 7, prepareCommand, "prepare 7")
	if err != nil {
		t.Fatal(err)
	}
	if sent := testutil.Receive(t, sender.sent, "the prepare command"); sent.Type != nodeproto.CommandPrepare ||
		sent.JobID != 7 {
		t.Fatalf("the node was sent %+v, want the prepare, named by its job's id", sent)
	}
	exchanges.Deliver(t.Context(), 1, nodeproto.Result{JobID: 8, Prepare: &nodeproto.PrepareReport{}})
	done := nodeproto.Result{JobID: 7, Prepare: &nodeproto.PrepareReport{State: nodeproto.PrepareDone}}
	exchanges.Deliver(t.Context(), 1, done)
	got, err := exchange.Next(t.Context(), time.Minute)
	if err != nil || got.Prepare == nil || got.Prepare.State != nodeproto.PrepareDone {
		t.Fatalf("Next = %+v, %v; want job 7's report, not job 8's", got, err)
	}
	exchange.MarkEndedByNode()
	exchange.Close()
	select {
	case sent := <-sender.sent:
		t.Errorf("closing a finished exchange sent %+v, want nothing", sent)
	default:
	}
}

// A silence that runs out tells the node to stop the work. It is ErrNodeSilent, naming the work,
// and parks the job only when the node went away.
func TestExchange_ASilenceTellsTheNodeToStopAndParksOnlyWhenItWentAway(t *testing.T) {
	t.Parallel()
	for _, isGone := range []bool{false, true} {
		bench := newExchanges(t)
		bench.sender.isGone = isGone
		exchange, waited := bench.askAndWait(t.Context(), t)
		// The wait may not have begun yet; the clock moves on until it has run out.
		for len(waited) == 0 {
			bench.now.Advance(time.Minute)
			runtime.Gosched()
		}
		err := testutil.Receive(t, waited, "the wait to end")
		if !errors.Is(err, nodes.ErrNodeSilent) || !strings.Contains(err.Error(), "prepare 7") ||
			errors.Is(err, errParked) != isGone {
			t.Errorf("a silence, the node gone %v: %v; want ErrNodeSilent naming the work, parked only when gone",
				isGone, err)
		}
		bench.wantToldToStop(t, exchange)
	}
}

// A job's context ending, a cancel or a shutdown, tells the node to stop the work.
func TestExchange_AContextEndingTellsTheNodeToStop(t *testing.T) {
	t.Parallel()
	bench := newExchanges(t)
	ctx, cancel := context.WithCancel(t.Context())
	exchange, waited := bench.askAndWait(ctx, t)
	cancel()
	if err := testutil.Receive(t, waited, "the wait to end"); !errors.Is(err, context.Canceled) {
		t.Errorf("a wait whose context ended: %v, want its error", err)
	}
	bench.wantToldToStop(t, exchange)
}

// A result no job waits for is dropped, and the log names the job by its id, as the records do
// (D227).
func TestExchanges_AResultForNoWaitingJobIsLoggedByItsJobID(t *testing.T) {
	t.Parallel()
	bench := newExchanges(t)
	exchanges, logged := bench.exchanges, bench.logged
	exchanges.Deliver(t.Context(), 1, nodeproto.Result{JobID: 42, Prepare: &nodeproto.PrepareReport{}})
	if !strings.Contains(logged.String(), `"jobId":42`) || !strings.Contains(logged.String(), `"nodeId":1`) {
		t.Errorf("the log = %s, want the job named as jobId and the node as nodeId", logged.String())
	}
}
