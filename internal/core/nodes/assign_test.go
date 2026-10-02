// SPDX-License-Identifier: GPL-3.0-or-later

package nodes_test

import (
	"context"
	"crypto/rand"
	"errors"
	"log/slog"
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
		Sender: bench.sender, Clock: bench.now, Random: rand.Reader, Logger: slog.New(slog.DiscardHandler),
		Cache: bench.cache, Settings: settingstest.Seeded(t, storetest.OpenMigrated(t)), MaxSilence: time.Minute,
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
