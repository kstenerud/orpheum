// SPDX-License-Identifier: AGPL-3.0-or-later

package jobs

import (
	"testing"

	"github.com/kstenerud/orpheum/internal/platform/jobqueue"
)

func TestCancelRetired_CancelsTheWorkOfWhatWasRetiredByTargetOrDestinationAndForgetsTheDestinations(t *testing.T) {
	t.Parallel()
	fixed := newFixture(t)
	retiredRoot, keptRoot := RootDestination(2, "/media/films"), RootDestination(2, "/media/series")
	requests := []Request{
		{Kind: KindFetchMetadata, Target: Target{ItemID: 1}},
		{Kind: KindReadThrough, Target: Target{FileID: 2}},
		{Kind: KindExtractSubtitle, Target: Target{StreamID: 3}},
		{Kind: KindConvertFetchedSubtitle, Target: Target{FetchedSubtitleID: 4}},
		{Kind: KindMakeChunks, Target: Target{ChunkStoreID: 5}, Destination: retiredRoot},
		{Kind: KindFetchMetadata, Target: Target{ItemID: 6}},
		{Kind: KindReadThrough, Target: Target{FileID: 7}, Destination: keptRoot},
	}
	for _, request := range requests {
		request.Reason = "test"
		if _, err := fixed.queue.Enqueue(t.Context(), request); err != nil {
			t.Fatal(err)
		}
	}
	fixed.queue.Park(t.Context(), Parking{Destination: retiredRoot, Reason: "missing", AwaitsSignal: true})
	cancelled, err := fixed.queue.CancelRetired(t.Context(), RetiredWork{
		ItemIDs: []int64{1}, FileIDs: []int64{2}, StreamIDs: []int64{3}, FetchedSubtitleIDs: []int64{4},
		Destinations: []Destination{retiredRoot},
	})
	if err != nil || cancelled != 5 {
		t.Errorf("CancelRetired = %d, %v; want the five jobs of what was retired", cancelled, err)
	}
	rows, err := fixed.cache.Read().ListJobs(t.Context(), 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		isKept := (row.ItemID != nil && *row.ItemID == 6) || (row.FileID != nil && *row.FileID == 7)
		want := jobqueue.StateCancelled
		if isKept {
			want = jobqueue.StateQueued
		}
		if jobqueue.State(row.State) != want {
			t.Errorf("job %d (%s) is %s, want %s", row.ID, row.Kind, row.State, want)
		}
	}
	if _, isParked := fixed.queue.DestinationState(retiredRoot); isParked {
		t.Error("the retired root is still parked: nothing brings it back, so it is forgotten")
	}
}
