// SPDX-License-Identifier: AGPL-3.0-or-later

package jobs

import (
	"testing"
	"time"
)

// Interest raises an item's jobs and its files' (D324 §2), never lowering one a play holds higher;
// past its wanted time, or released, a raised job falls to the shown-interest rung, not the
// background, and one raised for good stays.
func TestInterestJobs_RaiseTheWorkOfInterestAndLetItFallToTheShownInterestRung(t *testing.T) {
	t.Parallel()
	fixed := newFixture(t)
	ctx := t.Context()
	fixed.enqueue(t, 1, PriorityBackground)
	fixed.enqueue(t, 2, PriorityPlayNow)
	fixed.enqueue(t, 3, PriorityBackground)
	file := int64(40)
	if _, err := fixed.queue.Enqueue(ctx, Request{
		Kind: KindReadThrough, Target: Target{FileID: file}, Priority: PriorityBackground, Reason: "test",
	}); err != nil {
		t.Fatal(err)
	}
	interest := NewInterestJobs(fixed.cache, fixed.clock)
	until := fixed.clock.Now().Add(time.Minute)
	raised, err := interest.Raise(ctx, Interested{ItemIDs: []int64{1, 2}, FileIDs: []int64{file}}, PriorityLookedAt,
		until)
	if err != nil || len(raised) != 2 {
		t.Fatalf("Raise = %+v, %v; want item 1's job and the file's raised, item 2's play kept", raised, err)
	}
	if got := fixed.job(t, 1).Priority; got != PriorityLookedAt {
		t.Errorf("item 1's job is at %d; want the looked-at priority", got)
	}
	if got := fixed.job(t, 2).Priority; got != PriorityPlayNow {
		t.Errorf("item 2's play is at %d; interest must not lower it", got)
	}
	if releaseErr := interest.Release(ctx, Interested{FileIDs: []int64{file}}); releaseErr != nil {
		t.Fatal(releaseErr)
	}
	if _, raiseErr := interest.Raise(ctx, Interested{ItemIDs: []int64{3}}, PriorityShownInterest,
		time.Time{}); raiseErr != nil {
		t.Fatal(raiseErr)
	}
	demoted, err := fixed.queue.DemoteUnwanted(ctx)
	if err != nil || len(demoted) != 1 {
		t.Fatalf("demoted %+v, %v; want the released read-through alone, item 1 still wanted", demoted, err)
	}
	fixed.clock.Advance(2 * time.Minute)
	if _, err := fixed.queue.DemoteUnwanted(ctx); err != nil {
		t.Fatal(err)
	}
	if got := fixed.job(t, 1).Priority; got != PriorityShownInterest {
		t.Errorf("item 1's job, its interest past, is at %d; want the shown-interest rung", got)
	}
	if got := fixed.job(t, 3).Priority; got != PriorityShownInterest {
		t.Errorf("item 3's job, raised for good, is at %d; want the shown-interest rung", got)
	}
}
