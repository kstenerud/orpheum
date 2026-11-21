// SPDX-License-Identifier: AGPL-3.0-or-later

package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/core/unexpected"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/platform/jobqueue"
	"github.com/kstenerud/orpheum/internal/platform/jobqueue/jobqueuetest"
	"github.com/kstenerud/orpheum/internal/platform/outbound"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

func TestIsPassingFailure_KnowsTheCausesThatPassAndNoOther(t *testing.T) {
	t.Parallel()
	tmdb := func(status int) error {
		return fmt.Errorf("fetch item 3: %w", outbound.StatusError("TMDB", outbound.Answer{StatusCode: status}))
	}
	for name, err := range map[string]error{
		"a 500":          tmdb(http.StatusInternalServerError),
		"a 503":          tmdb(http.StatusServiceUnavailable),
		"a 429":          tmdb(http.StatusTooManyRequests),
		"a timeout":      fmt.Errorf("fetch: %w", context.DeadlineExceeded),
		"a silent node":  fmt.Errorf("%w by noon", jobqueue.ErrOverdue),
		"an unreachable": fmt.Errorf("dial: %w", outbound.ErrUnavailable),
	} {
		if !IsPassingFailure(err) {
			t.Errorf("%s (%v) is not known to pass", name, err)
		}
	}
	for name, err := range map[string]error{
		"a 404":          tmdb(http.StatusNotFound),
		"a parse error":  errors.New("ffprobe: Invalid data found when processing input"),
		"a cancellation": context.Canceled,
	} {
		if IsPassingFailure(err) {
			t.Errorf("%s (%v) is known to pass", name, err)
		}
	}
}

func TestSubject_NamesEachTargetAndItemIDsOfReadsTheItemsBack(t *testing.T) {
	t.Parallel()
	subjects := []string{
		Subject(Target{ItemID: 12}), Subject(Target{FileID: 7}), Subject(Target{ChunkStoreID: 9}),
		Subject(Target{GroupID: 3}), Subject(Target{}), Subject(Target{ItemID: 4}),
	}
	want := []string{"item 12", "file 7", "chunk store 9", "season group 3", "the server", "item 4"}
	if !slices.Equal(subjects, want) {
		t.Errorf("subjects = %v, want %v", subjects, want)
	}
	if got := ItemIDsOf(subjects); !slices.Equal(got, []int64{12, 4}) {
		t.Errorf("ItemIDsOf = %v, want [12 4]", got)
	}
}

// failsUnknownBut2 fails item 2's fetch with TMDB's 500, a cause that passes, and any other's for a
// cause not known.
func failsUnknownBut2(_ context.Context, job Job) error {
	if job.ItemID == 2 {
		return fmt.Errorf("fetch: %w", outbound.StatusError("TMDB", outbound.Answer{StatusCode: 500}))
	}
	return errors.New("the provider said what was not understood")
}

// TestRunDue_AJobGivenUpForAnUnknownCauseIsKeptAndNotQueuedAgainThisStart is the job queue's give-up
// (D511 §1-3): a cause not known is kept by the recorder and its work is queued no more this start;
// a TMDB 500, a cause that passes, is never given up.
func TestRunDue_AJobGivenUpForAnUnknownCauseIsKeptAndNotQueuedAgainThisStart(t *testing.T) {
	t.Parallel()
	database := storetest.OpenMigrated(t)
	config := settingstest.Seeded(t, database)
	settingstest.Set(t, config, settings.JobAttemptLimit, "2")
	fakeClock := clocktest.NewFake(time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC))
	logger := slog.New(slog.NewTextHandler(t.Output(), nil))
	recorder, err := unexpected.New(t.Context(), unexpected.Parts{
		Database: database, Settings: config, Clock: fakeClock, Logger: logger, Version: "test", StartID: "aa",
	})
	if err != nil {
		t.Fatal(err)
	}
	queue := NewQueue(QueueParts{
		Cache: storetest.OpenCache(t), Clock: fakeClock, Settings: config, Random: jobqueuetest.Unjittered(),
		Logger: logger, UnknownFailures: recorder,
	})
	fixed := fixture{queue: queue, cache: nil, clock: fakeClock, config: config}
	inserted, err := queue.EnqueueBatch(t.Context(), []Request{
		{Kind: KindFetchMetadata, Target: Target{ItemID: 1}, Reason: "test"},
		{Kind: KindFetchMetadata, Target: Target{ItemID: 2}, Reason: "test"},
	})
	if err != nil || inserted != 2 {
		t.Fatalf("queued %d, %v; want both", inserted, err)
	}
	runner := queue.NewRunner(KindFetchMetadata, failsUnknownBut2)
	for range 4 {
		fixed.runDue(t, runner)
		fakeClock.Advance(25 * time.Hour)
	}
	if !recorder.IsUnavailable(string(KindFetchMetadata), "item 1") {
		t.Error("a job given up for an unknown cause is not held unavailable")
	}
	if recorder.IsUnavailable(string(KindFetchMetadata), "item 2") {
		t.Error("a job failing with TMDB's 500 was given up as unknown")
	}
	report, err := recorder.Report(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Kinds) != 1 || len(report.Kinds[0].Failures) != 1 ||
		report.Kinds[0].Failures[0].Error != "the provider said what was not understood" {
		t.Errorf("report = %+v; want item 1's failure kept, with its error", report)
	}
	isAdded, err := queue.Enqueue(t.Context(), Request{Kind: KindFetchMetadata, Target: Target{ItemID: 1}})
	if err != nil || isAdded {
		t.Errorf("a request for work unavailable this start: added %t, %v; want it not queued", isAdded, err)
	}
	inserted, err = queue.EnqueueBatch(t.Context(), []Request{
		{Kind: KindFetchMetadata, Target: Target{ItemID: 1}}, {Kind: KindFetchMetadata, Target: Target{ItemID: 3}},
	})
	if err != nil || inserted != 1 {
		t.Errorf("a batch with work unavailable this start inserted %d, %v; want only item 3", inserted, err)
	}
}
