// SPDX-License-Identifier: GPL-3.0-or-later

package nodes_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"log/slog"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/platform/jobqueue"
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
	logger := slog.New(slog.NewJSONHandler(bench.logged, nil))
	bench.exchanges = nodes.NewExchanges(bench.sender, bench.now, rand.Reader, logger)
	return bench
}

// askAndWait asks for job 7's prepare, at play priority, and waits for its next result on another
// goroutine, whose error it returns on the channel. The command names the job and its priority, by
// which the node orders what waits (D234 §3).
func (bench exchangeBench) askAndWait(ctx context.Context, t *testing.T) (*nodes.Exchange, <-chan error) {
	t.Helper()
	job := jobs.Job{ID: 7, Priority: jobs.PriorityPlayNow}
	exchange, err := bench.exchanges.Ask(ctx, 1, job, prepareCommand, "prepare 7")
	if err != nil {
		t.Fatal(err)
	}
	sent := testutil.Receive(t, bench.sender.sent, "the prepare command")
	if sent.JobID != 7 || sent.Priority != jobs.PriorityPlayNow {
		t.Errorf("the node was sent job %d at priority %d, want job 7 at %d", sent.JobID, sent.Priority,
			jobs.PriorityPlayNow)
	}
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
	if stop.Type != nodeproto.CommandCancel || stop.JobID != 7 || stop.AttemptToken == "" {
		t.Errorf("the node was sent %+v, want it told to stop job 7's attempt, by its token", stop)
	}
}

var prepareCommand = nodeproto.Command{
	Work: nodeproto.Work{Type: nodeproto.CommandPrepare, Prepare: &nodeproto.Prepare{}},
}

// deliver hands exchanges node nodeID's result, naming the prepare command unless it names another,
// as a node's results do, and fails the test if it is not taken.
func deliver(t *testing.T, exchanges *nodes.Exchanges, nodeID int64, result nodeproto.Result) {
	t.Helper()
	if result.Work == nil {
		result.Work = &prepareCommand.Work
	}
	if err := exchanges.Deliver(t.Context(), nodeID, result); err != nil {
		t.Errorf("the result was not taken: %v", err)
	}
}

// A result reaches the job that asked by its attempt's token, from the node it asked, never by the
// job's id alone (S83, S84); and once the node said how the work ended, closing the exchange sends
// it nothing more.
func TestExchange_AResultReachesTheJobThatAskedAndAFinishedOneIsNotCancelled(t *testing.T) {
	t.Parallel()
	bench := newExchanges(t)
	exchanges, sender := bench.exchanges, bench.sender
	exchange, err := exchanges.Ask(t.Context(), 1, jobs.Job{ID: 7}, prepareCommand, "prepare 7")
	if err != nil {
		t.Fatal(err)
	}
	sent := testutil.Receive(t, sender.sent, "the prepare command")
	if sent.Type != nodeproto.CommandPrepare || sent.JobID != 7 || len(sent.AttemptToken) != 32 {
		t.Fatalf("the node was sent %+v, want the prepare, named by its job's id and a token of its own", sent)
	}
	report := func(state nodeproto.PrepareState) *nodeproto.PrepareReport {
		return &nodeproto.PrepareReport{State: state}
	}
	// Another job's, an earlier attempt's at job 7, and job 7's attempt's from another node: none is
	// this attempt's.
	deliver(t, exchanges, 1, nodeproto.Result{JobID: 8, Prepare: report(nodeproto.PrepareFailed)})
	deliver(t, exchanges, 1, nodeproto.Result{
		JobID: 7, AttemptToken: "0123456789abcdef0123456789abcdef", Prepare: report(nodeproto.PrepareFailed),
	})
	deliver(t, exchanges, 2, nodeproto.Result{
		JobID: 7, AttemptToken: sent.AttemptToken, Prepare: report(nodeproto.PrepareFailed),
	})
	deliver(t, exchanges, 1, nodeproto.Result{
		JobID: 7, AttemptToken: sent.AttemptToken, Prepare: report(nodeproto.PrepareDone),
	})
	got, err := exchange.Next(t.Context(), time.Minute)
	if err != nil || got.Prepare == nil || got.Prepare.State != nodeproto.PrepareDone {
		t.Fatalf("Next = %+v, %v; want this attempt's report from node 1, and no other", got, err)
	}
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
	deliver(t, exchanges, 1, nodeproto.Result{JobID: 42, Prepare: &nodeproto.PrepareReport{}})
	if !strings.Contains(logged.String(), `"jobId":42`) || !strings.Contains(logged.String(), `"nodeId":1`) {
		t.Errorf("the log = %s, want the job named as jobId and the node as nodeId", logged.String())
	}
}

// A node that refused the command, not knowing it, took no part of it: the job waits on the node,
// which needs updating, and closing the exchange sends it nothing to stop (D233 §9).
func TestExchange_ARefusalParksTheJobUntilTheNodeIsUpdatedAndStopsNothing(t *testing.T) {
	t.Parallel()
	bench := newExchanges(t)
	exchange, err := bench.exchanges.Ask(t.Context(), 1, jobs.Job{ID: 7}, prepareCommand, "prepare 7")
	if err != nil {
		t.Fatal(err)
	}
	sent := testutil.Receive(t, bench.sender.sent, "the prepare command")
	refusal := `this node cannot read the command "prepare"`
	refused := nodeproto.Result{JobID: 7, AttemptToken: sent.AttemptToken, Refusal: refusal}
	deliver(t, bench.exchanges, 1, refused)
	_, err = exchange.Next(t.Context(), time.Minute)
	parking, isParked := jobqueue.ParkingOf(err)
	if !isParked || parking.Destination != jobs.NodeDestination(1) || parking.Reason != jobs.ReasonNeedsUpdate ||
		!strings.Contains(err.Error(), "cannot read the command") {
		t.Errorf("a refusal: %v (%+v), want the job parked on node 1, which needs updating, saying why", err, parking)
	}
	exchange.Close()
	select {
	case sent := <-bench.sender.sent:
		t.Errorf("closing after a refusal sent %+v, want nothing to stop", sent)
	default:
	}
}

// A node that started again has lost the work it held in memory (D238 §3): the job waiting on it
// learns so at once, rather than after its silence runs out, and nothing is sent to stop work the
// node no longer has. A job waiting on another node waits on.
func TestExchange_ANodeThatStartedAgainEndsTheWaitOfItsJobsOnly(t *testing.T) {
	t.Parallel()
	bench := newExchanges(t)
	exchange, waited := bench.askAndWait(t.Context(), t)
	elsewhere, err := bench.exchanges.Ask(t.Context(), 2, jobs.Job{ID: 8}, prepareCommand, "prepare 8")
	if err != nil {
		t.Fatal(err)
	}
	sentElsewhere := testutil.Receive(t, bench.sender.sent, "job 8's prepare command")
	bench.exchanges.Interrupt(t.Context(), 1)
	if ended := testutil.Receive(t, waited, "job 7's wait ending"); !errors.Is(ended, nodes.ErrNodeRestarted) {
		t.Errorf("job 7's wait ended with %v, want ErrNodeRestarted", ended)
	}
	exchange.Close()
	deliver(t, bench.exchanges, 2, nodeproto.Result{
		JobID: 8, AttemptToken: sentElsewhere.AttemptToken,
		Prepare: &nodeproto.PrepareReport{State: nodeproto.PrepareDone},
	})
	result, err := elsewhere.Next(t.Context(), time.Minute)
	if err != nil || result.Prepare == nil {
		t.Errorf("job 8, on node 2, heard %+v, %v, want its result", result, err)
	}
	elsewhere.Close()
	select {
	case sent := <-bench.sender.sent:
		t.Errorf("the nodes were sent %+v, want nothing: node 1 lost job 7 with its restart", sent)
	default:
	}
}

// Progress is the node's word that it still holds the work: each starts the silence again and goes
// to OnProgress, whose error ends the wait; progress for another attempt, or from another node, is
// dropped. How the work ended is still Next's answer.
func TestExchange_ProgressKeepsTheWaitAliveAndOnlyItsOwnReachesTheJob(t *testing.T) {
	t.Parallel()
	bench := newExchanges(t)
	exchange, err := bench.exchanges.Ask(t.Context(), 1, jobs.Job{ID: 7}, prepareCommand, "prepare 7")
	if err != nil {
		t.Fatal(err)
	}
	sent := testutil.Receive(t, bench.sender.sent, "the prepare command")
	heard := make(chan nodeproto.Progress, 4)
	exchange.OnProgress = func(progress nodeproto.Progress) error {
		heard <- progress
		if progress.PreparedThroughMs < 0 {
			return errors.New("a prepare cannot go back")
		}
		return nil
	}
	running := func(nodeID int64, token string, throughMs int64) {
		bench.exchanges.DeliverProgress(t.Context(), nodeID, nodeproto.Progress{
			JobID: 7, AttemptToken: token, State: nodeproto.ProgressRunning, PreparedThroughMs: throughMs,
		})
	}
	ended := make(chan error, 1)
	go func() {
		_, nextErr := exchange.Next(t.Context(), time.Minute)
		ended <- nextErr
	}()
	running(2, sent.AttemptToken, 1000)
	running(1, "0123456789abcdef0123456789abcdef", 2000)
	running(1, sent.AttemptToken, 3000)
	if progress := testutil.Receive(t, heard, "the job's progress"); progress.PreparedThroughMs != 3000 {
		t.Errorf("the job heard %+v first, want its own attempt's from node 1, 3000 ms", progress)
	}
	// Past the first minute, the silence started again at the progress has not run out.
	for range 3 {
		bench.now.Advance(30 * time.Second)
		running(1, sent.AttemptToken, 4000)
		testutil.Receive(t, heard, "the job's progress again")
	}
	select {
	case err := <-ended:
		t.Fatalf("the wait ended %v while the node kept saying where the work is", err)
	default:
	}
	running(1, sent.AttemptToken, -1)
	testutil.Receive(t, heard, "the progress the job refuses")
	if err := testutil.Receive(t, ended, "the wait to end"); err == nil ||
		!strings.Contains(err.Error(), "cannot go back") {
		t.Errorf("the wait ended %v, want OnProgress's error", err)
	}
	bench.wantToldToStop(t, exchange)
}

// A result is recorded by its kind's recorder as it arrives, before any job hears it and whether or
// not a job still waits (S81): the node's acknowledgement waits on it, and a recorder's error leaves
// the result unacknowledged, for the node to send again, with no job told. A refusal is recorded by
// nothing, and a result naming no command is malformed.
func TestExchanges_RecordEveryResultBeforeAnyJobHearsIt(t *testing.T) {
	t.Parallel()
	bench := newExchanges(t)
	var recorded []int64
	var refusal error
	bench.exchanges.Record(nodeproto.CommandPrepare, func(_ context.Context, _ int64, result nodeproto.Result) error {
		if refusal != nil {
			return refusal
		}
		recorded = append(recorded, result.JobID)
		return nil
	})
	done := &nodeproto.PrepareReport{State: nodeproto.PrepareDone}
	// No job waits for job 41's: it is recorded all the same.
	deliver(t, bench.exchanges, 1, nodeproto.Result{JobID: 41, AttemptToken: "gone", Prepare: done})
	exchange, err := bench.exchanges.Ask(t.Context(), 1, jobs.Job{ID: 7}, prepareCommand, "prepare 7")
	if err != nil {
		t.Fatal(err)
	}
	defer exchange.Close()
	token := testutil.Receive(t, bench.sender.sent, "the prepare command").AttemptToken
	waited := make(chan error, 1)
	go func() {
		_, nextErr := exchange.Next(t.Context(), time.Minute)
		waited <- nextErr
	}()
	refusal = errors.New("cache.db is busy")
	err = bench.exchanges.Deliver(t.Context(), 1, nodeproto.Result{
		JobID: 7, AttemptToken: token, Work: &prepareCommand.Work, Prepare: done,
	})
	if err == nil || !strings.Contains(err.Error(), "busy") {
		t.Errorf("a result its recorder could not record was taken: %v", err)
	}
	select {
	case ended := <-waited:
		t.Fatalf("job 7 heard a result that was not recorded: %v", ended)
	default:
	}
	refusal = nil
	deliver(t, bench.exchanges, 1, nodeproto.Result{JobID: 7, AttemptToken: token, Prepare: done})
	if ended := testutil.Receive(t, waited, "job 7's result"); ended != nil {
		t.Errorf("job 7's wait ended %v, want its result", ended)
	}
	deliver(t, bench.exchanges, 1, nodeproto.Result{JobID: 43, AttemptToken: "x", Refusal: "unknown command"})
	if !slices.Equal(recorded, []int64{41, 7}) {
		t.Errorf("recorded jobs %v, want 41's and 7's once recorded, and no refusal", recorded)
	}
	malformed := bench.exchanges.Deliver(t.Context(), 1, nodeproto.Result{JobID: 44, Prepare: done})
	if !errors.Is(malformed, nodes.ErrMalformedResult) {
		t.Errorf("a result naming no command: %v, want ErrMalformedResult", malformed)
	}
}
