// SPDX-License-Identifier: AGPL-3.0-or-later

package nodes_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/core/nodes/nodestest"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/platform/jobqueue"
	"github.com/kstenerud/orpheum/internal/platform/waiting"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/storetest"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// recordingSender is a node that keeps what it was sent, and whose Unanswered says the node went
// away when isGone.
type recordingSender struct {
	sent   chan nodeproto.Command
	isGone bool
	// isQueued is a node that has not taken the work it was sent, which a stop withdraws.
	isQueued bool
	// resources are what the node can run at once; nil is a node whose resources are not known.
	resources *nodes.Resources
	// unparked are the destinations brought back, in order.
	unparked *unparkedDestinations
	// withdrawn are the attempts whose commands were withdrawn, by token, in order.
	withdrawn []string
	// startID is the start its last hello named.
	startID string
	// willSend are the attempts said to be on their way before their commands were queued, by token.
	willSend []string
	// onWithdraw, when set, runs at the next withdrawal, once: what happens while a stop is sent.
	onWithdraw func()
}

func (sender *recordingSender) StartOf(int64) string { return sender.startID }

func (sender *recordingSender) Unpark(ctx context.Context, destination jobs.Destination) {
	sender.unparked.Unpark(ctx, destination)
}

func (sender *recordingSender) Resources(context.Context, int64) (nodes.Resources, bool, error) {
	if sender.resources == nil {
		return nodes.Resources{}, false, nil
	}
	return *sender.resources, true, nil
}

var errParked = errors.New("parked on the node")

func (*recordingSender) ParkUnlessConnected(context.Context, int64) error { return nil }

// Withdraw says the work is still queued when the test made it so: the node has not taken it.
func (sender *recordingSender) Withdraw(_ int64, token string) bool {
	sender.withdrawn = append(sender.withdrawn, token)
	if then := sender.onWithdraw; then != nil {
		sender.onWithdraw = nil
		then()
	}
	return sender.isQueued
}

func (sender *recordingSender) Send(_ context.Context, _ int64, command nodeproto.Command) error {
	sender.sent <- command
	return nil
}

func (sender *recordingSender) Enqueue(_ int64, command nodeproto.Command) { sender.sent <- command }

func (sender *recordingSender) WillSend(_ int64, token string) {
	sender.willSend = append(sender.willSend, token)
}

func (sender *recordingSender) Unanswered(_ context.Context, _ int64, err error) error {
	if sender.isGone {
		return errors.Join(errParked, err)
	}
	return err
}

// exchangeBench is exchanges over a recording node, on a fake clock, logging to a buffer.
type exchangeBench struct {
	exchanges *nodes.Exchanges
	cache     *store.Cache
	sender    *recordingSender
	now       *clocktest.Fake
	logged    *bytes.Buffer
	// unparked are the destinations the exchanges brought back, in order.
	unparked *unparkedDestinations
}

// unparkedDestinations records each destination brought back.
type unparkedDestinations struct {
	mutex sync.Mutex
	all   []jobs.Destination
}

func (unparked *unparkedDestinations) Unpark(_ context.Context, destination jobs.Destination) {
	unparked.mutex.Lock()
	defer unparked.mutex.Unlock()
	unparked.all = append(unparked.all, destination)
}

func (unparked *unparkedDestinations) list() []jobs.Destination {
	unparked.mutex.Lock()
	defer unparked.mutex.Unlock()
	return slices.Clone(unparked.all)
}

func newExchanges(t *testing.T) exchangeBench {
	t.Helper()
	bench := exchangeBench{
		sender: &recordingSender{
			sent: make(chan nodeproto.Command, 4), unparked: &unparkedDestinations{}, startID: "first start",
		},
		now:    clocktest.NewFake(time.Date(2026, time.October, 1, 12, 0, 0, 0, time.UTC)),
		logged: &bytes.Buffer{},
	}
	bench.unparked = bench.sender.unparked
	logger := slog.New(slog.NewJSONHandler(bench.logged, nil))
	database := storetest.OpenMigrated(t)
	bench.cache = storetest.OpenCache(t)
	bench.exchanges = nodestest.NewExchanges(t, nodes.ExchangeParts{
		Sender: bench.sender, Rooms: bench.sender, Clock: bench.now, Logger: logger, Cache: bench.cache,
		Settings: settingstest.Seeded(t, database),
	})
	return bench
}

// wantToldToStop checks that the node was told to stop job 7's attempt sent.
func (bench exchangeBench) wantToldToStop(t *testing.T, sent nodeproto.Command) {
	t.Helper()
	stop := testutil.Receive(t, bench.sender.sent, "the cancel")
	if stop.Type != nodeproto.CommandCancel || stop.JobID != 7 || stop.AttemptToken != sent.AttemptToken {
		t.Errorf("the node was sent %+v, want it told to stop job 7's attempt, by its token", stop)
	}
}

// wantNothingSent checks that the node was sent nothing more.
func (bench exchangeBench) wantNothingSent(t *testing.T, why string) {
	t.Helper()
	select {
	case sent := <-bench.sender.sent:
		t.Errorf("the node was sent %+v, want nothing: %s", sent, why)
	default:
	}
}

var runCommand = nodeproto.Command{
	Work: nodeproto.Work{Type: nodeproto.CommandMakeChunks, MakeChunks: &nodeproto.MakeChunks{
		Stream: nodeproto.ChunkStream{Kind: nodeproto.StreamQuality},
	}},
}

// deliver hands exchanges node nodeID's result, naming the run's command unless it names another,
// as a node's results do, and fails the test if it is not taken.
func deliver(t *testing.T, exchanges *nodes.Exchanges, nodeID int64, result nodeproto.Result) {
	t.Helper()
	if result.Work == nil {
		result.Work = &runCommand.Work
	}
	if err := exchanges.Deliver(t.Context(), nodeID, result); err != nil {
		t.Errorf("the result was not taken: %v", err)
	}
}

// A result resolves the job given to a node by its attempt's token, from the node it was given to,
// never by the job's id alone (S83, S84); the command names the job, its token and its priority, by
// which the node orders what waits (D234 §3); and a job its node said how ended has nothing sent to
// stop it.
func TestExchange_AResultReachesTheJobThatAskedAndAFinishedOneIsNotCancelled(t *testing.T) {
	t.Parallel()
	bench := newExchanges(t)
	resolved := make(answers, 4)
	bench.exchanges.Handle(jobs.KindMakeChunks, assignedRun(resolved, nil))
	err := bench.exchanges.Assign(t.Context(), 1, jobs.Job{
		ID: 7, Kind: jobs.KindMakeChunks,
		Priority: jobs.PriorityPlayNow,
	}, runCommand)
	jobqueue.HandOver(t.Context(), err)
	sent := testutil.Receive(t, bench.sender.sent, "the run's command")
	if sent.Type != nodeproto.CommandMakeChunks || sent.JobID != 7 || len(sent.AttemptToken) != 32 ||
		sent.Priority != jobs.PriorityPlayNow {
		t.Fatalf("the node was sent %+v, want the run, named by its job's id and a token of its own, at play "+
			"priority", sent)
	}
	report := func(state nodeproto.RunState) *nodeproto.MakeChunksReport {
		return &nodeproto.MakeChunksReport{State: state}
	}
	// Another job's, an earlier attempt's at job 7, and job 7's attempt's from another node: none is
	// this attempt's.
	deliver(t, bench.exchanges, 1, nodeproto.Result{JobID: 8, MakeChunks: report(nodeproto.RunFailed)})
	deliver(t, bench.exchanges, 1, nodeproto.Result{
		JobID: 7, AttemptToken: "0123456789abcdef0123456789abcdef", MakeChunks: report(nodeproto.RunFailed),
	})
	deliver(t, bench.exchanges, 2, nodeproto.Result{
		JobID: 7, AttemptToken: sent.AttemptToken, MakeChunks: report(nodeproto.RunFailed),
	})
	deliver(t, bench.exchanges, 1, nodeproto.Result{
		JobID: 7, AttemptToken: sent.AttemptToken, MakeChunks: report(nodeproto.RunDone),
	})
	if answer := testutil.Receive(t, resolved, "job 7's answer"); answer != nil {
		t.Fatalf("job 7 was resolved with %v; want this attempt's report from node 1, and no other", answer)
	}
	if kept, err := bench.cache.Read().ListNodeAttempts(t.Context()); err != nil || len(kept) != 0 {
		t.Errorf("cache.db keeps %+v, %v once job 7 was resolved; want its attempt forgotten", kept, err)
	}
	bench.wantNothingSent(t, "a finished job has nothing to stop")
}

// A silence that runs out tells the node to stop the work. It is ErrNodeSilent, naming the work,
// and parks the job only when the node went away.
func TestExchange_ASilenceTellsTheNodeToStopAndParksOnlyWhenItWentAway(t *testing.T) {
	t.Parallel()
	for _, isGone := range []bool{false, true} {
		bench := newExchanges(t)
		bench.sender.isGone = isGone
		resolved := make(answers, 1)
		bench.exchanges.Handle(jobs.KindMakeChunks, assignedRun(resolved, nil))
		sent := bench.assign(t)
		tick := make(chan time.Time)
		go bench.exchanges.Watch(t.Context(), tick)
		bench.now.Advance(2 * time.Minute)
		tick <- time.Time{}
		err := testutil.Receive(t, resolved, "job 7's answer")
		if !errors.Is(err, nodes.ErrNodeSilent) || !strings.Contains(err.Error(), "make_chunks job 7") ||
			errors.Is(err, errParked) != isGone {
			t.Errorf("a silence, the node gone %v: %v; want ErrNodeSilent naming the work, parked only when gone",
				isGone, err)
		}
		bench.wantToldToStop(t, sent)
	}
}

// A cancel of a job its node holds tells the node to stop the work (S91).
func TestExchange_ACancelTellsTheNodeToStop(t *testing.T) {
	t.Parallel()
	bench := newExchanges(t)
	cancelled := make(chan nodes.Attempt, 1)
	bench.exchanges.Handle(jobs.KindMakeChunks, assignedRun(make(answers, 1), cancelled))
	sent := bench.assign(t)
	bench.exchanges.Cancelled(t.Context(), 7)
	bench.wantToldToStop(t, sent)
	if attempt := testutil.Receive(t, cancelled, "the kind told of the cancel"); attempt.Token != sent.AttemptToken {
		t.Errorf("the kind was told of attempt %s's cancel, want %s", attempt.Token, sent.AttemptToken)
	}
}

// A play's GET that cancels a run a node holds (a seek past it, the dialogue boost's turn ending an
// own-sound run) forgets the node's attempt, a write a request that only reads may make, with why:
// the server logs it at debug, not as a warning (staging, 2026-10-28), and storetest's cache fails
// the test of any write its list does not allow (D303, DL10).
func TestExchange_ACancelUnderARequestThatOnlyReadsForgetsTheAttemptAsItMay(t *testing.T) {
	t.Parallel()
	bench := newExchanges(t)
	cancelled := make(chan nodes.Attempt, 1)
	bench.exchanges.Handle(jobs.KindMakeChunks, assignedRun(make(answers, 1), cancelled))
	sent := bench.assign(t)
	bench.exchanges.Cancelled(waiting.ForAReader(t.Context()), 7)
	bench.wantToldToStop(t, sent)
	testutil.Receive(t, cancelled, "the kind told of the cancel")
}

// A job whose time ran out (assigned_job_limit_hours) tells its node to stop the work, and its
// attempt is forgotten: the queue ended the job, and nothing of its kind is told, as for a cancel
// (G4, D244 §6).
func TestExchange_AJobWhoseTimeRanOutTellsTheNodeToStop(t *testing.T) {
	t.Parallel()
	bench := newExchanges(t)
	cancelled := make(chan nodes.Attempt, 1)
	bench.exchanges.Handle(jobs.KindMakeChunks, assignedRun(make(answers, 1), cancelled))
	sent := bench.assign(t)
	bench.exchanges.Overdue(t.Context(), 7)
	bench.wantToldToStop(t, sent)
	select {
	case attempt := <-cancelled:
		t.Errorf("the kind was told attempt %s was cancelled; the queue already ended its job", attempt.Token)
	default:
	}
	bench.exchanges.Cancelled(t.Context(), 7)
	bench.wantNothingSent(t, "the attempt is forgotten once its time ran out")
}

// A result for no attempt given to the node resolves nothing, and the log names the job by its id,
// as the records do (D227).
func TestExchanges_AResultForNoWaitingJobIsLoggedByItsJobID(t *testing.T) {
	t.Parallel()
	bench := newExchanges(t)
	exchanges, logged := bench.exchanges, bench.logged
	deliver(t, exchanges, 1, nodeproto.Result{JobID: 42, MakeChunks: &nodeproto.MakeChunksReport{}})
	if !strings.Contains(logged.String(), `"jobId":42`) || !strings.Contains(logged.String(), `"nodeId":1`) {
		t.Errorf("the log = %s, want the job named as jobId and the node as nodeId", logged.String())
	}
}

// A node that refused the command, not knowing it, took no part of it: the job waits on the node,
// which needs updating, and nothing is sent to stop it (D233 §9).
func TestExchange_ARefusalParksTheJobUntilTheNodeIsUpdatedAndStopsNothing(t *testing.T) {
	t.Parallel()
	bench := newExchanges(t)
	resolved := make(answers, 1)
	bench.exchanges.Handle(jobs.KindMakeChunks, assignedRun(resolved, nil))
	sent := bench.assign(t)
	refusal := `this node cannot read the command "make_chunks"`
	deliver(t, bench.exchanges, 1, nodeproto.Result{JobID: 7, AttemptToken: sent.AttemptToken, Refusal: refusal})
	err := testutil.Receive(t, resolved, "job 7's answer")
	parking, isParked := jobqueue.ParkingOf(err)
	if !isParked || parking.Destination != jobs.NodeDestination(1) || parking.Reason != jobs.ReasonNeedsUpdate ||
		!strings.Contains(err.Error(), "cannot read the command") {
		t.Errorf("a refusal: %v (%+v), want the job parked on node 1, which needs updating, saying why", err, parking)
	}
	bench.wantNothingSent(t, "a refused command has nothing to stop")
}

// A node that started again has lost the work it held in memory (D238 §3): the job given to it is
// resolved so at once, rather than after its silence runs out, and nothing is sent to stop work the
// node no longer has. A job given to another node waits on.
func TestExchange_ANodeThatStartedAgainEndsTheWaitOfItsJobsOnly(t *testing.T) {
	t.Parallel()
	bench := newExchanges(t)
	resolved := make(answers, 2)
	bench.exchanges.Handle(jobs.KindMakeChunks, assignedRun(resolved, nil))
	bench.assign(t)
	elsewhere := bench.exchanges.Assign(t.Context(), 2, jobs.Job{ID: 8, Kind: jobs.KindMakeChunks}, runCommand)
	jobqueue.HandOver(t.Context(), elsewhere)
	sentElsewhere := testutil.Receive(t, bench.sender.sent, "job 8's run's command")
	bench.exchanges.Interrupt(t.Context(), 1, "started again")
	if ended := testutil.Receive(t, resolved, "job 7's answer"); !errors.Is(ended, nodes.ErrNodeRestarted) {
		t.Errorf("job 7 was resolved with %v, want ErrNodeRestarted", ended)
	}
	deliver(t, bench.exchanges, 2, nodeproto.Result{
		JobID: 8, AttemptToken: sentElsewhere.AttemptToken,
		MakeChunks: &nodeproto.MakeChunksReport{State: nodeproto.RunDone},
	})
	if answer := testutil.Receive(t, resolved, "job 8's answer"); answer != nil {
		t.Errorf("job 8, on node 2, was resolved with %v, want its result", answer)
	}
	bench.wantNothingSent(t, "node 1 lost job 7 with its restart")
}

// Progress is the node's word that it still holds the work: each starts the silence again and goes
// to the kind's Progress, whose error ends the job; progress for another attempt, or from another
// node, is dropped. How the work ended is still the result's.
func TestExchange_ProgressKeepsTheWaitAliveAndOnlyItsOwnReachesTheJob(t *testing.T) {
	t.Parallel()
	bench := newExchanges(t)
	resolved := make(answers, 1)
	heard := make(chan nodeproto.Progress, 8)
	assignee := assignedRun(resolved, nil)
	refuse := assignee.Progress
	assignee.Progress = func(ctx context.Context, attempt nodes.Attempt, progress nodeproto.Progress) error {
		heard <- progress
		return refuse(ctx, attempt, progress)
	}
	bench.exchanges.Handle(jobs.KindMakeChunks, assignee)
	sent := bench.assign(t)
	tick := make(chan time.Time)
	go bench.exchanges.Watch(t.Context(), tick)
	running := func(nodeID int64, token string, through int) {
		bench.exchanges.DeliverProgress(t.Context(), nodeID, nodeproto.Progress{
			JobID: 7, AttemptToken: token, State: nodeproto.ProgressRunning, ChunksMadeThrough: &through,
		})
	}
	running(2, sent.AttemptToken, 10)
	running(1, "0123456789abcdef0123456789abcdef", 20)
	running(1, sent.AttemptToken, 30)
	progress := testutil.Receive(t, heard, "the job's progress")
	if progress.ChunksMadeThrough == nil || *progress.ChunksMadeThrough != 30 {
		t.Errorf("the job heard %+v first, want its own attempt's from node 1, through chunk 30", progress)
	}
	// Past the first minute, the silence started again at the progress has not run out.
	for range 3 {
		bench.now.Advance(30 * time.Second)
		running(1, sent.AttemptToken, 40)
		testutil.Receive(t, heard, "the job's progress again")
		tick <- time.Time{}
	}
	select {
	case err := <-resolved:
		t.Fatalf("the job was resolved with %v while the node kept saying where the work is", err)
	default:
	}
	running(1, sent.AttemptToken, -1)
	testutil.Receive(t, heard, "the progress the job refuses")
	if err := testutil.Receive(t, resolved, "the job's answer"); err == nil ||
		!strings.Contains(err.Error(), "cannot go back") {
		t.Errorf("the job was resolved with %v, want Progress's error", err)
	}
	bench.wantToldToStop(t, sent)
}

// A result is recorded by its kind's recorder as it arrives, before its job is resolved and whether
// or not one still is (S81): the node's acknowledgement waits on it, and a recorder's error leaves
// the result unacknowledged, for the node to send again, with the job not resolved. A refusal is
// recorded by nothing, and a result naming no work is malformed.
func TestExchanges_RecordEveryResultBeforeAnyJobHearsIt(t *testing.T) {
	t.Parallel()
	bench := newExchanges(t)
	resolved := make(answers, 2)
	bench.exchanges.Handle(jobs.KindMakeChunks, assignedRun(resolved, nil))
	var recorded []int64
	var refusal error
	record := func(_ context.Context, _ int64, result nodeproto.Result) error {
		if refusal != nil {
			return refusal
		}
		recorded = append(recorded, result.JobID)
		return nil
	}
	bench.exchanges.Record(nodeproto.CommandMakeChunks, record)
	done := &nodeproto.MakeChunksReport{State: nodeproto.RunDone}
	// No job is given an attempt "gone": job 41's result is recorded all the same.
	deliver(t, bench.exchanges, 1, nodeproto.Result{JobID: 41, AttemptToken: "gone", MakeChunks: done})
	sent := bench.assign(t)
	refusal = errors.New("cache.db is busy")
	err := bench.exchanges.Deliver(t.Context(), 1, nodeproto.Result{
		JobID: 7, AttemptToken: sent.AttemptToken, Work: &runCommand.Work, MakeChunks: done,
	})
	if err == nil || !strings.Contains(err.Error(), "busy") {
		t.Errorf("a result its recorder could not record was taken: %v", err)
	}
	select {
	case answer := <-resolved:
		t.Fatalf("job 7 was resolved by a result that was not recorded: %v", answer)
	default:
	}
	refusal = nil
	deliver(t, bench.exchanges, 1, nodeproto.Result{JobID: 7, AttemptToken: sent.AttemptToken, MakeChunks: done})
	if answer := testutil.Receive(t, resolved, "job 7's answer"); answer != nil {
		t.Errorf("job 7 was resolved with %v, want its result", answer)
	}
	deliver(t, bench.exchanges, 1, nodeproto.Result{JobID: 43, AttemptToken: "x", Refusal: "unknown command"})
	if len(recorded) != 2 || recorded[0] != 41 || recorded[1] != 7 {
		t.Errorf("recorded jobs %v, want 41's and 7's once recorded, and no refusal", recorded)
	}
	malformed := bench.exchanges.Deliver(t.Context(), 1, nodeproto.Result{JobID: 44, MakeChunks: done})
	if !errors.Is(malformed, nodes.ErrMalformedResult) {
		t.Errorf("a result naming no work: %v, want ErrMalformedResult", malformed)
	}
}

// A job cancelled before its node took its command has the command withdrawn, and nothing sent to
// stop work the node never had (S27).
func TestExchange_ACancelBeforeTheNodeTookTheWorkSendsItNothing(t *testing.T) {
	t.Parallel()
	bench := newExchanges(t)
	bench.exchanges.Handle(jobs.KindMakeChunks, assignedRun(make(answers, 1), make(chan nodes.Attempt, 1)))
	bench.assign(t)
	bench.sender.isQueued = true
	bench.exchanges.Cancelled(t.Context(), 7)
	bench.wantNothingSent(t, "the node never took the work, which was withdrawn")
}

// Work a person waits for, to play, is sent play-driven, and pauses the background work on its
// device; background work is not (D240 §6). Each command carries its job's priority, by which the
// node orders what waits (D234 §3).
func TestExchange_PlayPriorityWorkIsSentPlayDrivenAndBackgroundWorkIsNot(t *testing.T) {
	t.Parallel()
	bench := newExchanges(t)
	bench.exchanges.Handle(jobs.KindMakeChunks, assignedRun(make(answers, 4), nil))
	for _, job := range []jobs.Job{
		{ID: 7, Kind: jobs.KindMakeChunks, Priority: jobs.PriorityPlayNow},
		{ID: 8, Kind: jobs.KindMakeChunks, Priority: 0},
	} {
		jobqueue.HandOver(t.Context(), bench.exchanges.Assign(t.Context(), 1, job, runCommand))
		sent := testutil.Receive(t, bench.sender.sent, "the run's command")
		if sent.Priority != job.Priority {
			t.Errorf("job %d at priority %d was sent at priority %d", job.ID, job.Priority, sent.Priority)
		}
		if wantPlayDriven := job.Priority >= jobs.PriorityPlayNow; sent.IsPlayDriven != wantPlayDriven {
			t.Errorf("job %d at priority %d was sent play-driven %v, want %v", job.ID, job.Priority,
				sent.IsPlayDriven, wantPlayDriven)
		}
	}
}
