// SPDX-License-Identifier: GPL-3.0-or-later

package nodes_test

import (
	"context"
	"crypto/rand"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/jobqueue"
	"github.com/kstenerud/orpheum/internal/store/storetest"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// answers is an assignee's runner that hands the test each answer a job is resolved with.
type answers chan error

func (resolved answers) ResolveBy(_ context.Context, id int64, decide func(jobs.Job) error) (bool, error) {
	resolved <- decide(jobs.Job{ID: id, Kind: jobs.KindPrepare})
	return true, nil
}

// assignedPrepare is an assignee of prepares whose answers name what decided them, and whose
// progress refuses a prepare said to go back.
func assignedPrepare(resolved answers, cancelled chan<- nodes.Attempt) nodes.Assignee {
	return nodes.Assignee{
		Runner: resolved,
		Answer: func(_ context.Context, _ jobs.Job, _ nodes.Attempt, result nodeproto.Result) error {
			if result.Prepare.State == nodeproto.PrepareDone {
				return nil
			}
			return errors.New("the node said " + string(result.Prepare.State))
		},
		Unanswered: func(_ context.Context, _ jobs.Job, _ nodes.Attempt, err error) error { return err },
		Progress: func(_ context.Context, _ nodes.Attempt, progress nodeproto.Progress) error {
			if progress.PreparedThroughMs < 0 {
				return errors.New("a prepare cannot go back")
			}
			return nil
		},
		Cancelled: func(_ context.Context, attempt nodes.Attempt) error {
			cancelled <- attempt
			return nil
		},
	}
}

// assign gives job 7's prepare to node 1, and returns the command the node was sent.
func (bench exchangeBench) assign(t *testing.T) nodeproto.Command {
	t.Helper()
	err := bench.exchanges.Assign(t.Context(), 1, jobs.Job{ID: 7, Kind: jobs.KindPrepare}, prepareCommand)
	if !jobqueue.IsAssigned(err) {
		t.Fatalf("Assign answered %v, want the job assigned", err)
	}
	if len(bench.sender.sent) != 0 {
		t.Fatal("the node was sent the work before its job was recorded assigned")
	}
	jobqueue.Hand(t.Context(), err) // as the runner does, once it recorded the job assigned
	return testutil.Receive(t, bench.sender.sent, "the prepare command")
}

// A job given to a node is assigned, its attempt kept in cache.db before the command is sent; the
// node's word on how it ended resolves it, with its kind's answer, and the attempt is forgotten
// (D234 §7).
func TestAssign_AJobGivenToANodeIsResolvedByItsWordAndForgotten(t *testing.T) {
	t.Parallel()
	bench := newExchanges(t)
	resolved := make(answers, 1)
	bench.exchanges.Handle(jobs.KindPrepare, assignedPrepare(resolved, nil))
	sent := bench.assign(t)
	if kept, err := bench.cache.Read().ListNodeAttempts(t.Context()); err != nil || len(kept) != 1 ||
		kept[0].AttemptToken != sent.AttemptToken || kept[0].JobID != 7 || kept[0].NodeID != 1 {
		t.Fatalf("cache.db keeps %+v, %v; want job 7's attempt on node 1", kept, err)
	}
	failed := nodeproto.Result{JobID: 7, AttemptToken: sent.AttemptToken, Prepare: &nodeproto.PrepareReport{
		State: nodeproto.PrepareFailed,
	}}
	deliver(t, bench.exchanges, 1, failed)
	answer := testutil.Receive(t, resolved, "job 7's answer")
	if answer == nil || answer.Error() != "the node said failed" {
		t.Errorf("job 7 was resolved with %v, want its kind's answer to the node's word", answer)
	}
	if kept, err := bench.cache.Read().ListNodeAttempts(t.Context()); err != nil || len(kept) != 0 {
		t.Errorf("cache.db keeps %+v, %v once job 7 was resolved; want nothing", kept, err)
	}
}

// The attempts given to nodes outlive a core restart: loaded again, the node's word resolves its job
// as it would have before (S82).
func TestLoad_AnAttemptKeptThroughARestartIsResolvedByItsNodesWord(t *testing.T) {
	t.Parallel()
	bench := newExchanges(t)
	sent := bench.assign(t)
	restarted := nodes.NewExchanges(nodes.ExchangeParts{
		Sender: bench.sender, Rooms: bench.sender, Clock: bench.now, Random: rand.Reader,
		Logger: slog.New(slog.DiscardHandler), Cache: bench.cache, MaxSilence: time.Minute,
		Settings: settingstest.Seeded(t, storetest.OpenMigrated(t)),
	})
	if err := restarted.Load(t.Context()); err != nil {
		t.Fatal(err)
	}
	resolved := make(answers, 1)
	restarted.Handle(jobs.KindPrepare, assignedPrepare(resolved, nil))
	deliver(t, restarted, 1, nodeproto.Result{
		JobID: 7, AttemptToken: sent.AttemptToken,
		Prepare: &nodeproto.PrepareReport{State: nodeproto.PrepareDone},
	})
	if answer := testutil.Receive(t, resolved, "job 7's answer"); answer != nil {
		t.Errorf("job 7, after a restart, was resolved with %v, want done", answer)
	}
}

// A node's word on an attempt whose kind no runner resolves yet is not taken: it is sent again.
func TestDeliver_AResultBeforeItsKindIsResolvedIsNotTaken(t *testing.T) {
	t.Parallel()
	bench := newExchanges(t)
	sent := bench.assign(t)
	err := bench.exchanges.Deliver(t.Context(), 1, nodeproto.Result{
		JobID: 7, AttemptToken: sent.AttemptToken,
		Work: &prepareCommand.Work, Prepare: &nodeproto.PrepareReport{State: nodeproto.PrepareDone},
	})
	if !errors.Is(err, nodes.ErrNotResolvedYet) {
		t.Errorf("a result before its kind resolves: %v, want ErrNotResolvedYet", err)
	}
}

// An assigned job its node says nothing of for the silence limit is unanswered, and the node told
// to stop; its progress keeps it heard of; progress its kind refuses ends it, the node told to stop.
func TestWatch_ASilentAttemptIsUnansweredAndProgressKeepsItHeard(t *testing.T) {
	t.Parallel()
	bench := newExchanges(t)
	resolved := make(answers, 1)
	bench.exchanges.Handle(jobs.KindPrepare, assignedPrepare(resolved, nil))
	sent := bench.assign(t)
	tick := make(chan time.Time)
	go bench.exchanges.Watch(t.Context(), tick)
	running := func(throughMs int64) {
		bench.exchanges.DeliverProgress(t.Context(), 1, nodeproto.Progress{
			JobID: 7, AttemptToken: sent.AttemptToken, State: nodeproto.ProgressRunning, PreparedThroughMs: throughMs,
		})
	}
	for range 3 {
		bench.now.Advance(40 * time.Second)
		running(1000)
		tick <- time.Time{}
	}
	select {
	case answer := <-resolved:
		t.Fatalf("job 7 was resolved with %v while its node said where it is", answer)
	default:
	}
	bench.now.Advance(2 * time.Minute)
	tick <- time.Time{}
	if answer := testutil.Receive(t, resolved, "job 7's answer"); !errors.Is(answer, nodes.ErrNodeSilent) {
		t.Errorf("a silent attempt was resolved with %v, want ErrNodeSilent", answer)
	}
	if stop := testutil.Receive(t, bench.sender.sent, "the cancel"); stop.Type != nodeproto.CommandCancel ||
		stop.AttemptToken != sent.AttemptToken {
		t.Errorf("the node was sent %+v, want told to stop job 7's attempt", stop)
	}
	again := bench.assign(t)
	sent = again
	running(-1)
	if stop := testutil.Receive(t, bench.sender.sent, "the cancel"); stop.Type != nodeproto.CommandCancel {
		t.Errorf("the node was sent %+v, want told to stop the attempt its progress ended", stop)
	}
	refused := testutil.Receive(t, resolved, "the answer")
	if refused == nil || refused.Error() != "a prepare cannot go back" {
		t.Errorf("progress its kind refused resolved the job with %v, want the refusal", refused)
	}
}

// A job cancelled while its node holds its work has the node told to stop it, and its kind told
// (S91); a node that started again resolves its jobs as interrupted (D238 §3).
func TestCancelledAndInterrupt_EndTheAttemptsOfTheirJobAndNode(t *testing.T) {
	t.Parallel()
	bench := newExchanges(t)
	resolved, cancelled := make(answers, 1), make(chan nodes.Attempt, 1)
	bench.exchanges.Handle(jobs.KindPrepare, assignedPrepare(resolved, cancelled))
	sent := bench.assign(t)
	bench.exchanges.Cancelled(t.Context(), 7)
	if stop := testutil.Receive(t, bench.sender.sent, "the cancel"); stop.Type != nodeproto.CommandCancel ||
		stop.AttemptToken != sent.AttemptToken {
		t.Errorf("a cancel sent the node %+v, want told to stop job 7's attempt", stop)
	}
	if attempt := testutil.Receive(t, cancelled, "the kind told of the cancel"); attempt.JobID != 7 {
		t.Errorf("the kind was told of job %d's cancel, want job 7's", attempt.JobID)
	}
	bench.assign(t)
	bench.exchanges.Interrupt(t.Context(), 1)
	if answer := testutil.Receive(t, resolved, "job 7's answer"); !errors.Is(answer, nodes.ErrNodeRestarted) {
		t.Errorf("a node that started again resolved job 7 with %v, want ErrNodeRestarted", answer)
	}
}

// The core squares what it gave a node with what the node's status says it holds (D234 §6): an
// attempt held is heard of, its job adopted; one not held, whose command the node took, is lost, a
// broken invariant counted, its job resolved as the tool's failure; one still on its way is not
// judged; work held that no job was given is a broken invariant too, and the node told to drop it;
// work whose result is on its way is left to it.
func TestSquare_SquaresWhatANodeWasGivenWithWhatItHolds(t *testing.T) {
	t.Parallel()
	bench := newExchanges(t)
	resolved := make(answers, 2)
	bench.exchanges.Handle(jobs.KindPrepare, assignedPrepare(resolved, nil))
	sent := bench.assign(t)
	running := []nodeproto.HeldAttempt{{JobID: 7, AttemptToken: sent.AttemptToken, State: nodeproto.HeldRunning}}
	tick := make(chan time.Time)
	go bench.exchanges.Watch(t.Context(), tick)
	for range 3 {
		bench.now.Advance(40 * time.Second)
		bench.exchanges.Square(t.Context(), 1, running, nil)
		tick <- time.Time{}
	}
	select {
	case answer := <-resolved:
		t.Fatalf("job 7, held by its node, was resolved with %v; want it adopted, heard of by each status", answer)
	default:
	}
	bench.exchanges.Square(t.Context(), 1, nil, map[string]bool{sent.AttemptToken: true})
	select {
	case answer := <-resolved:
		t.Fatalf("job 7, its command on its way, was resolved with %v; want it not judged", answer)
	default:
	}
	if count := bench.exchanges.BrokenInvariants(1); count != 0 {
		t.Fatalf("%d broken invariants before any, want none", count)
	}
	bench.exchanges.Square(t.Context(), 1, nil, nil)
	if answer := testutil.Receive(t, resolved, "job 7's answer"); !errors.Is(answer, nodes.ErrWorkLost) {
		t.Errorf("job 7, its node not holding it, was resolved with %v, want ErrWorkLost", answer)
	}
	stray := []nodeproto.HeldAttempt{
		{JobID: 9, AttemptToken: "stray", State: nodeproto.HeldRunning},
		{JobID: 10, AttemptToken: "ended", State: nodeproto.HeldEnded},
	}
	bench.exchanges.Square(t.Context(), 1, stray, nil)
	if stop := testutil.Receive(t, bench.sender.sent, "the cancel"); stop.Type != nodeproto.CommandCancel ||
		stop.AttemptToken != "stray" {
		t.Errorf("the node was sent %+v, want told to drop the work no job was given", stop)
	}
	bench.wantNothingSent(t, "work whose result is on its way is left to the node")
	if count := bench.exchanges.BrokenInvariants(1); count != 2 {
		t.Errorf("%d broken invariants counted, want 2: the lost job and the stray work", count)
	}
}

// Where the node holding a job's work last said it is, by its progress or its status, is kept for
// the job view: waiting, running, or paused for play-driven work (D240 §6).
func TestNodeWordOf_IsWhereTheNodeLastSaidAJobsWorkIs(t *testing.T) {
	t.Parallel()
	bench := newExchanges(t)
	bench.exchanges.Handle(jobs.KindPrepare, assignedPrepare(make(answers, 2), nil))
	sent := bench.assign(t)
	if word, isSaid := bench.exchanges.NodeWordOf(7); isSaid {
		t.Errorf("before the node spoke of job 7 its word is %q, want none", word)
	}
	bench.exchanges.DeliverProgress(t.Context(), 1, nodeproto.Progress{
		JobID: 7, AttemptToken: sent.AttemptToken, State: nodeproto.ProgressWaiting,
	})
	if word, _ := bench.exchanges.NodeWordOf(7); word != nodeproto.ProgressWaiting {
		t.Errorf("after its progress said waiting, job 7's word is %q", word)
	}
	bench.exchanges.Square(t.Context(), 1,
		[]nodeproto.HeldAttempt{{JobID: 7, AttemptToken: sent.AttemptToken, State: nodeproto.HeldPaused}}, nil)
	if word, _ := bench.exchanges.NodeWordOf(7); word != nodeproto.ProgressPaused {
		t.Errorf("after its status said paused, job 7's word is %q", word)
	}
}

// The core holds work only while its node says it moves: running or waiting work the node says has
// not moved for the core's window is stopped and answered as not moving, the tool's failure, which a
// kind's UnansweredOutcome takes as a stall; under the window it is held (D233 §5).
func TestSquare_WorkItsNodeSaysHasNotMovedForTheWindowIsStopped(t *testing.T) {
	t.Parallel()
	bench := newExchanges(t)
	resolved := make(answers, 2)
	bench.exchanges.Handle(jobs.KindPrepare, assignedPrepare(resolved, nil))
	sent := bench.assign(t)
	held := func(sinceMoved time.Duration) []nodeproto.HeldAttempt {
		return []nodeproto.HeldAttempt{{
			JobID: 7, AttemptToken: sent.AttemptToken, State: nodeproto.HeldWaiting,
			SinceMovedMs: sinceMoved.Milliseconds(),
		}}
	}
	bench.exchanges.Square(t.Context(), 1, held(29*time.Minute), nil)
	select {
	case answer := <-resolved:
		t.Fatalf("job 7, not moving for 29 minutes, was resolved with %v; want it held", answer)
	default:
	}
	bench.exchanges.Square(t.Context(), 1, held(30*time.Minute), nil)
	answer := testutil.Receive(t, resolved, "job 7's answer")
	if !errors.Is(answer, nodes.ErrNotMoving) {
		t.Errorf("job 7, not moving for 30 minutes, was resolved with %v, want ErrNotMoving", answer)
	}
	if stop := testutil.Receive(t, bench.sender.sent, "the cancel"); stop.Type != nodeproto.CommandCancel ||
		stop.AttemptToken != sent.AttemptToken {
		t.Errorf("the node was sent %+v, want told to stop job 7's work", stop)
	}
	stalled := nodes.UnansweredOutcome(jobs.Job{ID: 7}, answer, nil)
	if !strings.HasPrefix(stalled.Error(), "stalled: ") {
		t.Errorf("a kind's outcome of work not moving = %v; want a stall, tried again at no cost", stalled)
	}
	twice := nodes.UnansweredOutcome(jobs.Job{ID: 7, PreviousAttemptToolFailed: true}, answer, nil)
	if !strings.Contains(twice.Error(), jobs.StalledTwice) {
		t.Errorf("a second in a row = %v; want the job's own failure", twice)
	}
}
