// SPDX-License-Identifier: AGPL-3.0-or-later

package identify

import (
	"context"
	"fmt"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/preferences"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// What should have been fetched and was not is filled in by a sweep, passively (D335 §1): it looks
// for a part an item should have and does not, and queues the item's fetch, which fetches what is
// missing as its first fetch would have, through the queue's back-off and its giving up (D231); a
// series whose fetch gave up has a failure of its own and waits for a person. It fills only what is
// empty: nothing stored is fetched again (that is a refresh, a person's deliberate act, D335 §2).
// Each kind of missing part is one query here; the first are a series' orderings' (episode-orderings.md
// §3.4).

// missingFetchesPerSweep is how many items one sweep queues the fetch of what they are missing: a
// little at a time, never a flood at startup (D335 §3), as the sweep runs hourly and at start.
const missingFetchesPerSweep = 20

// QueueMissing queues the fetch of what matched series are missing of their orderings, at most
// missingFetchesPerSweep a sweep: first each episode group a person chose that is not fetched, at a
// person's priority; then each list of groups a series fetched before its record carried one lacks,
// behind the queue's other work. It says how many it queued.
func (refresher *Refresher) QueueMissing(ctx context.Context) (int, error) {
	owing, err := refresher.database.Read().ListSeriesOwingOrderingParts(ctx,
		sqlcgen.ListSeriesOwingOrderingPartsParams{
			Axis: preferences.AxisEpisodeOrdering, Provider: new(FetchProvider), MaxRows: missingFetchesPerSweep,
		})
	if err != nil {
		return 0, fmt.Errorf("list the series missing a part of their orderings: %w", err)
	}
	queued := 0
	for _, series := range owing {
		job := missingOrderingsJob(series.ID, series.IsChosen != 0)
		added, err := refresher.enqueueDerived(ctx, job)
		if err != nil {
			return queued, err
		}
		queued += added
	}
	return queued, nil
}

// QueueChosenOrderings queues, at the person's priority, the fetch of each episode group of a series
// a person chose that is not fetched yet: a first fetch on their act (D170; episode-orderings.md
// §3.4), which the series' page says is under way. It says whether it queued one.
func (refresher *Refresher) QueueChosenOrderings(ctx context.Context, seriesID int64) (bool, error) {
	chosen, err := refresher.database.Read().ListChosenGroupsToFetch(ctx, sqlcgen.ListChosenGroupsToFetchParams{
		Axis: preferences.AxisEpisodeOrdering, CollectionID: seriesID, Provider: new(FetchProvider),
	})
	if err != nil {
		return false, fmt.Errorf("list the episode groups chosen in series %d: %w", seriesID, err)
	}
	if len(chosen) == 0 {
		return false, nil
	}
	queued, err := refresher.queue.Enqueue(ctx, missingOrderingsJob(seriesID, true))
	if err != nil {
		return false, fmt.Errorf("queue series %d's chosen orderings: %w", seriesID, err)
	}
	return queued, nil
}

// FetchProvider is the provider a fetch_metadata job calls in Cut A, whose parts a sweep looks for.
const FetchProvider = "tmdb"

// missingOrderingsJob is the fetch_metadata job that fetches what a matched series' orderings are
// owed (a match of a matched series, fetchOwedOrderings): at a person's priority for a group one
// chose, else behind the queue's other work.
func missingOrderingsJob(seriesID int64, isChosen bool) jobs.Request {
	job := jobs.Request{
		Kind: jobs.KindFetchMetadata, Target: jobs.Target{ItemID: seriesID}, Destination: FetchDestination,
		Reason: "the list of its episode orderings, fetched before it was asked for", Priority: rollingPriority,
	}
	if isChosen {
		job.Reason, job.Priority = "an episode ordering a person chose", jobs.PriorityPersonsRequest
	}
	return job
}
