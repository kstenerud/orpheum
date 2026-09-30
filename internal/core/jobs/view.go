// SPDX-License-Identifier: GPL-3.0-or-later

package jobs

import (
	"context"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/jobqueue"
)

// ErrCancelled is the cause a running job's context ends with when a person cancels it: its
// handler stops what it started, a node's work included.
var ErrCancelled = jobqueue.ErrCancelled

// ErrNotOpen is a cancel of a job that is no longer queued or running, or was never there.
var ErrNotOpen = jobqueue.ErrNotOpen

// View is one job as the job view shows it (cut-a §5.3).
type View struct {
	ID   int64
	Kind Kind
	Target
	State string
	// ProgressFraction is how far it is, 0 to 1, or nil when not known.
	ProgressFraction *float64
	Reason           string
	LastFailure      string
	AttemptCount     int64
	CreatedAt        time.Time
	FinishedAt       *time.Time
	// Destination is what it needs that can be down, when known (D231 §5).
	Destination Destination
	// WaitingFor is its destination's state while it is queued on a parked one: what the job view
	// shows as "waiting for TMDB — unreachable since 14:02, next check 14:10" (D231 §12).
	WaitingFor *DestinationState
}

// List is the newest jobs, newest first, at most limit of them.
func (queue *Queue) List(ctx context.Context, limit int64) ([]View, error) {
	rows, err := queue.queue.List(ctx, limit)
	if err != nil {
		return nil, err
	}
	views := make([]View, 0, len(rows))
	for _, row := range rows {
		view := View{
			ID: row.ID, Kind: row.Kind, Target: row.Target, State: string(row.State),
			ProgressFraction: row.ProgressFraction, Reason: row.Reason, LastFailure: row.LastFailure,
			AttemptCount: row.AttemptCount, CreatedAt: row.CreatedAt, FinishedAt: row.FinishedAt,
			Destination: row.Destination,
		}
		if state, isParked := queue.queue.DestinationState(row.Destination); isParked &&
			row.State == jobqueue.StateQueued {
			view.WaitingFor = &state
		}
		views = append(views, view)
	}
	return views, nil
}

// Cancel cancels a queued or running job: a queued one never runs, and a running one's handler
// is told through its context, whose cause is ErrCancelled, so it can stop the work it started.
// Whatever the handler answers after, the job stays cancelled.
func (queue *Queue) Cancel(ctx context.Context, id int64) error {
	return queue.queue.Cancel(ctx, id)
}

// SetProgress records how far a running job is, from 0 to 1.
func (queue *Queue) SetProgress(ctx context.Context, id int64, fraction float64) error {
	return queue.queue.SetProgress(ctx, id, fraction)
}
