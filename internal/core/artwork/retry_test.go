// SPDX-License-Identifier: GPL-3.0-or-later

package artwork

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/jobs/jobstest"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/platform/outbound"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// failingFetcher is a source whose image server fails with err, counting the calls.
type failingFetcher struct {
	err       error
	callCount int
}

func (*failingFetcher) RecheckAfter() time.Duration { return 12 * time.Hour }

func (source *failingFetcher) Fetch(context.Context, string) (Unsanitised, error) {
	source.callCount++
	return Unsanitised{}, source.err
}

func TestService_AMissingImageIsRecheckedOnceAfterTheCDNForgetsThenGivenUp(t *testing.T) {
	t.Parallel()
	bench := newBench(t, fakeFetcher{})
	queue := &jobstest.Queue{}
	bench.service.parts.Queue = queue
	gone := owedRef{Kind: identify.ArtPoster, SourceRef: "/gone.jpg"}
	bench.handle(t, gone)
	if len(*queue) != 1 || (*queue)[0].Kind != jobs.KindFetchMetadata || (*queue)[0].ItemID != bench.itemID ||
		!(*queue)[0].RunnableAt.Equal(bench.service.parts.Clock.Now().Add(12*time.Hour)) {
		t.Fatalf("queued %+v; want the item's record refreshed once, twelve hours on", *queue)
	}
	var args struct {
		IsRefresh         bool `json:"isRefresh"`
		ShouldRetryImages bool `json:"shouldRetryImages"`
	}
	if err := json.Unmarshal([]byte((*queue)[0].ArgsJSON), &args); err != nil || !args.IsRefresh ||
		!args.ShouldRetryImages {
		t.Errorf("the recheck's args %s (%v)", (*queue)[0].ArgsJSON, err)
	}
	// The recheck's refresh asks for the images again: it removes the rows it left.
	err := bench.database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
		return queries.RetryItemArtwork(t.Context(), sqlcgen.RetryItemArtworkParams{
			RemovedAt: new(int64(1)), ItemID: bench.itemID,
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	*queue = nil
	bench.handle(t, gone)
	if len(*queue) != 0 {
		t.Errorf("queued %+v after the second miss; want it given up", *queue)
	}
	if rows := bench.rows(t); len(rows) != 1 || rows[0].SourceSha256 != nil {
		t.Errorf("rows %+v; want the miss held, so no pass asks for it again", rows)
	}
}

func TestService_AFailingHostIsPausedAfterFiveFailuresAndProbedAfterEachStep(t *testing.T) {
	t.Parallel()
	bench := newBench(t, fakeFetcher{})
	down := &failingFetcher{err: errors.New("503")}
	bench.service.parts.Fetchers["tmdb"] = down
	clock, isFake := bench.service.parts.Clock.(*clocktest.Fake)
	if !isFake {
		t.Fatal("the bench's clock is not a fake")
	}
	job := bench.job(owedRef{Kind: identify.ArtPoster, SourceRef: "/p.jpg"})
	for range breakAfterFailures {
		err := bench.service.Handle(t.Context(), job)
		var wait *jobs.RetryAtError
		if err == nil || errors.As(err, &wait) {
			t.Fatalf("a failure costs its job an attempt: %v", err)
		}
	}
	err := bench.service.Handle(t.Context(), job)
	wait, isWait := errors.AsType[*jobs.RetryAtError](err)
	if !isWait || down.callCount != breakAfterFailures || !wait.At.Equal(clock.Now().Add(RetrySchedule()[0])) {
		t.Fatalf("after %d failures: %v, %d calls; want a pause of %v with no call", breakAfterFailures, err,
			down.callCount, RetrySchedule()[0])
	}
	clock.Advance(RetrySchedule()[0])
	if err := bench.service.Handle(t.Context(), job); err == nil || down.callCount != breakAfterFailures+1 {
		t.Fatalf("the probe: %v, %d calls", err, down.callCount)
	}
	wait, isWait = errors.AsType[*jobs.RetryAtError](bench.service.Handle(t.Context(), job))
	if !isWait || !wait.At.Equal(clock.Now().Add(RetrySchedule()[1])) {
		t.Errorf("after a failed probe: %v; want the next step, %v", wait, RetrySchedule()[1])
	}
}

func TestService_ARateLimitWaitsWithoutCostingAnAttempt(t *testing.T) {
	t.Parallel()
	bench := newBench(t, fakeFetcher{})
	bench.service.parts.Fetchers["tmdb"] = &failingFetcher{
		err: &outbound.RateLimitedError{Provider: "TMDB", RetryAfter: time.Minute},
	}
	err := bench.service.Handle(t.Context(), bench.job(owedRef{Kind: identify.ArtPoster, SourceRef: "/p.jpg"}))
	if wait, isWait := errors.AsType[*jobs.RetryAtError](err); !isWait ||
		!wait.At.Equal(bench.service.parts.Clock.Now().Add(time.Minute)) {
		t.Errorf("a 429: %v; want a wait of a minute", err)
	}
}
