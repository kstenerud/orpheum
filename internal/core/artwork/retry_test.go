// SPDX-License-Identifier: GPL-3.0-or-later

package artwork

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/platform/jobqueue"
	"github.com/kstenerud/orpheum/internal/platform/outbound"
)

// failingFetcher is a source whose image server fails with err, counting the calls.
type failingFetcher struct {
	err       error
	callCount int
}

func (*failingFetcher) RecheckAfter() time.Duration { return 12 * time.Hour }

func (*failingFetcher) Destination() jobs.Destination { return jobs.DestinationTMDBImages }

func (source *failingFetcher) Fetch(context.Context, string) (Unsanitised, error) {
	source.callCount++
	return Unsanitised{}, source.err
}

func TestService_AMissingImageIsRecheckedOnceAfterTheCDNForgetsThenGivenUp(t *testing.T) {
	t.Parallel()
	bench := newBench(t, fakeFetcher{})
	gone := Ref{Kind: library.ArtPoster, SourceRef: "/gone.jpg"}
	bench.handle(t, gone)
	dueRechecks := func() []int64 {
		t.Helper()
		due, err := bench.service.DueRechecks(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		return due
	}
	if due := dueRechecks(); len(due) != 0 {
		t.Errorf("due rechecks %v at once; want none while the CDN still answers from its cached 404", due)
	}
	bench.clock.Advance(12 * time.Hour)
	if due := dueRechecks(); !slices.Equal(due, []int64{bench.itemID}) {
		t.Fatalf("due rechecks %v twelve hours on; want the item's record refreshed once (D213 section 1)", due)
	}
	// The recheck's refresh asks for the images again: it removes the rows it left.
	if err := bench.service.RetryImages(t.Context(), bench.itemID); err != nil {
		t.Fatal(err)
	}
	bench.handle(t, gone)
	bench.clock.Advance(24 * time.Hour)
	if due := dueRechecks(); len(due) != 0 {
		t.Errorf("due rechecks %v after the second miss; want it given up", due)
	}
	rows := bench.rows(t)
	if len(rows) != 1 || rows[0].SourceSha256 != nil || rows[0].RefusedAt == nil {
		t.Errorf("rows %+v; want the miss held with when, so no pass asks for it again", rows)
	}
}

func TestService_AnImageServerItsProviderParksPassesTheParkingOnAndRecordsNothing(t *testing.T) {
	t.Parallel()
	bench := newBench(t, fakeFetcher{})
	job := bench.job(Ref{Kind: library.ArtPoster, SourceRef: "/p.jpg"})
	parking := jobs.Parking{Destination: jobs.DestinationTMDBImages, Reason: "unreachable"}
	bench.service.parts.Fetchers["tmdb"] = &failingFetcher{
		err: jobs.Park(parking, fmt.Errorf("%w: status 503", outbound.ErrUnavailable)),
	}
	err := bench.service.Handle(t.Context(), job)
	if parked, isParked := jobqueue.ParkingOf(err); !isParked || parked != parking {
		t.Errorf("an image server its provider parked: %v; want the parking passed to the queue", err)
	}
	if rows := bench.rows(t); len(rows) != 0 {
		t.Errorf("rows %+v; a host down says nothing of the image, which must not be held as refused", rows)
	}
	bench.service.parts.Fetchers["tmdb"] = &failingFetcher{err: errors.New("status 500")}
	err = bench.service.Handle(t.Context(), job)
	if _, isParked := jobqueue.ParkingOf(err); err == nil || isParked || len(bench.rows(t)) != 0 {
		t.Errorf("a plain 500: %v; want the job's own failure, which costs an attempt (D231 §5)", err)
	}
}
