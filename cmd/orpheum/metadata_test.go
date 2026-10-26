// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"crypto/rand"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/platform/jobqueue/jobqueuetest"
	"github.com/kstenerud/orpheum/internal/store/cachegen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// unreadableSettings is a database that cannot be read.
type unreadableSettings struct{}

func (unreadableSettings) Duration(context.Context, settings.Key) (time.Duration, error) {
	return 0, errors.New("database is locked")
}

// A loop's wait is its setting, read afresh; when the setting cannot be read, the loop goes on at
// the registry's starting value, not a copy of it.
func TestLengthOrStarting_IsTheSettingOrElseItsStartingValue(t *testing.T) {
	t.Parallel()
	config := settingstest.Seeded(t, storetest.OpenMigrated(t))
	settingstest.Set(t, config, settings.ReadThroughSweepEveryHours, "3")
	logger := slog.New(slog.DiscardHandler)
	if got := lengthOrStarting(t.Context(), config, settings.ReadThroughSweepEveryHours, logger); got != 3*time.Hour {
		t.Errorf("read_through_sweep_every_hours at 3: a wait of %v, want 3h", got)
	}
	for _, check := range []struct {
		key  settings.Key
		want time.Duration
	}{
		{settings.MetadataRefreshCheckEveryHours, 24 * time.Hour},
		{settings.ArtworkServedAtFlushEverySeconds, time.Hour},
	} {
		if got := lengthOrStarting(t.Context(), unreadableSettings{}, check.key, logger); got != check.want {
			t.Errorf("%s unreadable: a wait of %v, want its starting value, %v", check.key, got, check.want)
		}
	}
}

// The jobs no one waits for are looked for at most half the shortest play priority an admin may set
// apart, so a job keeps play priority at most half as long again as its setting says (D234 §3).
func TestDemoteUnwantedEvery_IsAtMostHalfTheShortestPlayPriorityKept(t *testing.T) {
	t.Parallel()
	kept, isDefined := settings.Lookup(settings.PlayPriorityKeptSeconds)
	if !isDefined {
		t.Fatal("play_priority_kept_seconds is not a setting")
	}
	if shortest := time.Duration(kept.Minimum) * kept.Unit; demoteUnwantedEvery > shortest/2 {
		t.Errorf("demoteUnwantedEvery is %v, against play_priority_kept_seconds' shortest, %v; want at most "+
			"half of it", demoteUnwantedEvery, shortest)
	}
}

// The job view hears from the exchanges where the node holding a job's work last said it is: here
// its status, which lists the attempt paused (D240 §6).
func TestReportedProgressOf_IsWhereTheNodeHoldingAJobsWorkLastSaidItIs(t *testing.T) {
	t.Parallel()
	cache := storetest.OpenCache(t)
	err := cache.Write(t.Context(), func(queries *cachegen.Queries) error {
		return queries.InsertNodeAttempt(t.Context(), cachegen.InsertNodeAttemptParams{
			AttemptToken: "attempt-7", JobID: 7, JobKind: string(jobs.KindReadThrough), NodeID: 1,
			WorkJSON: `{"type":"read_through"}`, SentAt: 1,
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	exchanges := nodes.NewExchanges(nodes.ExchangeParts{
		Clock: clocktest.NewFake(time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)), Random: rand.Reader,
		Logger: slog.New(slog.DiscardHandler), Cache: cache, MaxSilence: time.Minute,
		Settings: settingstest.Seeded(t, storetest.OpenMigrated(t)),
	})
	if err := exchanges.Load(t.Context()); err != nil {
		t.Fatal(err)
	}
	words := reportedProgressOf(exchanges)
	if word, isSaid := words(7); isSaid {
		t.Errorf("before its node spoke of job 7 its word is %q, want none", word)
	}
	exchanges.Reconcile(t.Context(), 1,
		[]nodeproto.HeldAttempt{{JobID: 7, AttemptToken: "attempt-7", State: nodeproto.HeldPaused}}, nil)
	if word, isSaid := words(7); !isSaid || word != "paused" {
		t.Errorf("once its node's status listed job 7 paused, its word is %q (%v), want paused", word, isSaid)
	}
}

// toldSender is a node that records what it is told, and has no work sent to it.
type toldSender struct{ told chan nodeproto.Command }

func (sender toldSender) Enqueue(_ int64, command nodeproto.Command) { sender.told <- command }

func (toldSender) Send(context.Context, int64, nodeproto.Command) error { return nil }

func (toldSender) ParkUnlessConnected(context.Context, int64) error { return nil }

func (toldSender) Withdraw(int64, string) bool { return false }

func (toldSender) StartOf(int64) string { return "" }

func (toldSender) Unanswered(_ context.Context, _ int64, err error) error { return err }

func (toldSender) Unpark(context.Context, jobs.Destination) {}

func (toldSender) Resources(context.Context, int64) (nodes.Resources, bool, error) {
	return nodes.Resources{}, false, nil
}

// A play's job no one waits for any more falls to background priority at the next look, and the
// node holding one given to it is told (D234 §3, S87).
func TestDemoteUnwanted_APlaysJobNoOneWaitsForFallsAndItsNodeIsTold(t *testing.T) {
	t.Parallel()
	cache := storetest.OpenCache(t)
	now := clocktest.NewFake(time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC))
	config := settingstest.Seeded(t, storetest.OpenMigrated(t))
	logger := slog.New(slog.DiscardHandler)
	queue := jobs.NewQueue(jobs.QueueParts{
		Cache: cache, Clock: now, Settings: config, Random: jobqueuetest.Unjittered(), Logger: logger,
	})
	if _, err := queue.Enqueue(t.Context(), jobs.Request{
		Kind: jobs.KindReadThrough, Target: jobs.Target{FileID: 4}, Priority: jobs.PriorityPlayNow, Reason: "test",
	}); err != nil {
		t.Fatal(err)
	}
	runner := queue.NewRunner(jobs.KindReadThrough, func(context.Context, jobs.Job) error {
		return jobs.Assign(now.Now().Add(time.Hour))
	})
	if err := runner.RunDue(t.Context()); err != nil {
		t.Fatal(err)
	}
	listed, err := cache.Read().ListJobs(t.Context(), 1)
	if err != nil || len(listed) != 1 {
		t.Fatalf("the jobs are %+v (%v), want the one measurement", listed, err)
	}
	job := listed[0].ID
	err = cache.Write(t.Context(), func(queries *cachegen.Queries) error {
		return queries.InsertNodeAttempt(t.Context(), cachegen.InsertNodeAttemptParams{
			AttemptToken: "attempt", JobID: job, JobKind: string(jobs.KindReadThrough), NodeID: 1,
			WorkJSON: `{"type":"read_through"}`, SentAt: 1,
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	told := toldSender{told: make(chan nodeproto.Command, 1)}
	exchanges := nodes.NewExchanges(nodes.ExchangeParts{
		Sender: told, Rooms: told, Clock: now, Random: rand.Reader, Logger: logger, Cache: cache, Settings: config,
		MaxSilence: time.Hour,
	})
	if err = exchanges.Load(t.Context()); err != nil {
		t.Fatal(err)
	}
	now.Advance(11 * time.Minute)
	tick := make(chan time.Time, 1)
	tick <- now.Now()
	ctx, stop := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		demoteUnwanted(ctx, queue, exchanges, tick, logger)
	}()
	command := testutil.Receive(t, told.told, "the node told")
	stop()
	testutil.Receive(t, done, "the loop's end")
	if command.Type != nodeproto.CommandReprioritize || command.JobID != job ||
		command.Priority != jobs.PriorityShownInterest {
		t.Errorf("the node was told %+v, want job %d at the shown-interest priority", command, job)
	}
	// Someone wanted it, so it stays a rung above the background (D324).
	again, err := cache.Read().ListJobs(t.Context(), 1)
	if err != nil || len(again) != 1 || again[0].Priority != jobs.PriorityShownInterest {
		t.Errorf("the job is %+v (%v), want it at the shown-interest priority", again, err)
	}
}
