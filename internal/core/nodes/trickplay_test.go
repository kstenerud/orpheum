// SPDX-License-Identifier: AGPL-3.0-or-later

package nodes_test

import (
	"context"
	"errors"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/jobqueue"
	"github.com/kstenerud/orpheum/internal/testutil"
)

var trickplayCommand = nodeproto.Command{
	Work: nodeproto.Work{Type: nodeproto.CommandMakeTrickplay, MakeTrickplay: &nodeproto.MakeTrickplay{
		Video: nodeproto.BytesFile{RootPath: "/media", Path: []byte("film.mkv")}, IntervalMs: 10_000,
	}},
}

// A play raises its film's trickplay to a play's place in the queue, but never its CPU or disk tier
// (trickplay.md section 4, question 4): its node is told a priority below a person's, never
// play-driven, as it is assigned and when it is raised after.
func TestTrickplay_ARaisedRunIsNeverSentPlayDrivenNorAtAPersonsPriority(t *testing.T) {
	t.Parallel()
	bench := newExchanges(t)
	bench.exchanges.Handle(jobs.KindTrickplay, assignedRun(make(answers, 4), nil))
	job := jobs.Job{ID: 7, Kind: jobs.KindTrickplay, Priority: jobs.PriorityPlayNow}
	jobqueue.HandOver(t.Context(), bench.exchanges.Assign(t.Context(), 1, job, trickplayCommand))
	sent := testutil.Receive(t, bench.sender.sent, "the trickplay's command")
	if sent.IsPlayDriven || sent.Priority >= jobs.PriorityPersonsRequest {
		t.Errorf("a trickplay at a play's priority was sent play-driven %v at priority %d; want neither",
			sent.IsPlayDriven, sent.Priority)
	}
	bench.exchanges.Reprioritize(7, jobs.PriorityPlayNow)
	raised := testutil.Receive(t, bench.sender.sent, "the trickplay's new priority")
	if raised.Type != nodeproto.CommandReprioritize || raised.IsPlayDriven ||
		raised.Priority >= jobs.PriorityPersonsRequest {
		t.Errorf("a trickplay raised to a play's priority was told %+v; want a priority below a person's", raised)
	}
	// Any other kind keeps its own.
	bench.exchanges.Handle(jobs.KindMakeChunks, assignedRun(make(answers, 4), nil))
	chunks := jobs.Job{ID: 8, Kind: jobs.KindMakeChunks, Priority: jobs.PriorityPlayNow}
	jobqueue.HandOver(t.Context(), bench.exchanges.Assign(t.Context(), 1, chunks, runCommand))
	if sent := testutil.Receive(t, bench.sender.sent, "the run's command"); !sent.IsPlayDriven {
		t.Errorf("a run of chunks at a play's priority was sent %+v, want it play-driven", sent)
	}
}

// A sheet is taken only for a trickplay attempt the core holds, for its node and job: one for a run
// no longer wanted is ErrAttemptNotHeld, which the channel answers so its node stops.
func TestTrickplay_ASheetIsTakenOnlyForAnAttemptTheCoreHolds(t *testing.T) {
	t.Parallel()
	bench := newExchanges(t)
	bench.exchanges.Handle(jobs.KindTrickplay, assignedRun(make(answers, 4), make(chan nodes.Attempt, 1)))
	var taken []nodeproto.TrickplaySheet
	bench.exchanges.TakeSheetsBy(func(_ context.Context, attempt nodes.Attempt, sheet nodeproto.TrickplaySheet) error {
		if attempt.JobID != sheet.JobID {
			t.Errorf("sheet of job %d handed with job %d's attempt", sheet.JobID, attempt.JobID)
		}
		taken = append(taken, sheet)
		return nil
	})
	job := jobs.Job{ID: 7, Kind: jobs.KindTrickplay}
	jobqueue.HandOver(t.Context(), bench.exchanges.Assign(t.Context(), 1, job, trickplayCommand))
	sent := testutil.Receive(t, bench.sender.sent, "the trickplay's command")
	sheet := nodeproto.TrickplaySheet{JobID: 7, AttemptToken: sent.AttemptToken, Index: 2, WebP: []byte("RIFF")}
	if err := bench.exchanges.DeliverTrickplaySheet(t.Context(), 1, sheet); err != nil || len(taken) != 1 {
		t.Fatalf("a held attempt's sheet: %v, taken %d; want it taken", err, len(taken))
	}
	for name, stranger := range map[string]struct {
		nodeID int64
		sheet  nodeproto.TrickplaySheet
	}{
		"another node's":   {2, sheet},
		"another job's":    {1, nodeproto.TrickplaySheet{JobID: 8, AttemptToken: sent.AttemptToken, Index: 2}},
		"an unknown token": {1, nodeproto.TrickplaySheet{JobID: 7, AttemptToken: "gone", Index: 2}},
	} {
		if err := bench.exchanges.DeliverTrickplaySheet(t.Context(), stranger.nodeID, stranger.sheet); !errors.Is(err,
			nodes.ErrAttemptNotHeld) {
			t.Errorf("%s sheet: %v, want ErrAttemptNotHeld", name, err)
		}
	}
	bench.exchanges.Cancelled(t.Context(), 7)
	if err := bench.exchanges.DeliverTrickplaySheet(t.Context(), 1, sheet); !errors.Is(err, nodes.ErrAttemptNotHeld) {
		t.Errorf("a sheet after its run was cancelled: %v, want ErrAttemptNotHeld", err)
	}
	if len(taken) != 1 {
		t.Errorf("%d sheets taken, want only the held attempt's", len(taken))
	}
}
