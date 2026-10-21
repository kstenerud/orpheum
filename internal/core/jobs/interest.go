// SPDX-License-Identifier: AGPL-3.0-or-later

package jobs

import (
	"context"
	"fmt"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/store"
)

// The priorities a person's interest gives work (D324 §2), between a play's and the background's.
const (
	// PriorityLookedAt is the work of a film or series whose page someone is looking at: the latest
	// look highest, each earlier one a rung below, none below PriorityPersonsRequest's next.
	PriorityLookedAt int64 = 100
	// PriorityShownInterest, "user has shown interest", is the work of anything a person has played,
	// looked at or partly watched, once that interest is past: a rung above the background's, below
	// a person's request (the owner, 2026-10-21, on anything a person showed any interest in: it
	// "goes to the just-above-bg priority if it would otherwise drop to bg priority").
	PriorityShownInterest int64 = 5
)

// Interested is what a person's interest is in: items, for the jobs about them (their metadata
// and artwork), and files, for the jobs about those (read-throughs, subtitles).
type Interested struct {
	ItemIDs []int64
	FileIDs []int64
}

// isEmpty is whether it names nothing.
func (interested Interested) isEmpty() bool {
	return len(interested.ItemIDs) == 0 && len(interested.FileIDs) == 0
}

// Raised is an open job a person's interest raised: one given to a node is the node's to be told.
type Raised struct {
	ID         int64
	Priority   int64
	IsAssigned bool
}

// InterestJobs raises and releases the jobs a person's interest is in (D324 §2): cache.db's job
// table, through internal/store, which alone writes it.
type InterestJobs struct {
	storage *store.JobStorage
	clock   clock.Clock
}

// NewInterestJobs returns the interest's jobs over cache.db.
func NewInterestJobs(cache *store.Cache, clockSource clock.Clock) *InterestJobs {
	return &InterestJobs{storage: cache.Jobs(), clock: clockSource}
}

// Raise gives every open job about what is interested at least priority, wanted until until, or
// for good when until is zero, and returns those whose priority rose. A job already higher, as a
// play's, keeps its priority.
func (interest *InterestJobs) Raise(
	ctx context.Context, interested Interested, priority int64, until time.Time,
) ([]Raised, error) {
	if interested.isEmpty() {
		return nil, nil
	}
	var untilMs int64
	if !until.IsZero() {
		untilMs = until.UnixMilli()
	}
	rows, err := interest.storage.RaiseJobsOfInterest(ctx, interested.ItemIDs, interested.FileIDs, priority, untilMs)
	if err != nil {
		return nil, fmt.Errorf("raise the interest's work: %w", err)
	}
	raised := make([]Raised, 0, len(rows))
	for _, row := range rows {
		raised = append(raised, Raised{ID: row.ID, Priority: row.Priority, IsAssigned: row.State == "assigned"})
	}
	return raised, nil
}

// Release ends now the wanted time of the jobs about what is no longer interested, below a play's
// priority: the next demotion gives them the shown-interest rung (D324 §2).
func (interest *InterestJobs) Release(ctx context.Context, interested Interested) error {
	if interested.isEmpty() {
		return nil
	}
	nowMs := interest.clock.Now().Add(-time.Millisecond).UnixMilli()
	if err := interest.storage.ReleaseJobsOfInterest(ctx, interested.ItemIDs, interested.FileIDs, PriorityPlayNow,
		nowMs); err != nil {
		return fmt.Errorf("release the interest's work: %w", err)
	}
	return nil
}
