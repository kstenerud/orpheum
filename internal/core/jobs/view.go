// SPDX-License-Identifier: AGPL-3.0-or-later

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
	// State is queued, running, done, failed or cancelled: a job given to a node is running.
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
	// OnNode is where the node it was given to last said it is, for a job assigned to one: waiting
	// for a slot or a reader, running, or paused for play-driven work (D240 §6); empty otherwise.
	OnNode string
}

// ReportedProgress says where the node holding a job's work last said it is: nodes.Exchanges.ReportedProgressOf.
type ReportedProgress func(jobID int64) (word string, isSaid bool)

// SetReportedProgress has the job view say where the node holding each assigned job's work is.
func (queue *Queue) SetReportedProgress(words ReportedProgress) {
	queue.reportedProgressMutex.Lock()
	defer queue.reportedProgressMutex.Unlock()
	queue.reportedProgress = words
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
		if row.State == jobqueue.StateAssigned {
			// To a person, work given to a node runs there; OnNode says where it is.
			view.State = string(jobqueue.StateRunning)
			if words := queue.reportedProgressSource(); words != nil {
				if word, isSaid := words(row.ID); isSaid {
					view.OnNode = word
				}
			}
		}
		views = append(views, view)
	}
	return views, nil
}

func (queue *Queue) reportedProgressSource() ReportedProgress {
	queue.reportedProgressMutex.Lock()
	defer queue.reportedProgressMutex.Unlock()
	return queue.reportedProgress
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

// KindHealth is how one kind's jobs are doing: queued, the oldest of them, and failed in the last
// day (D233 §8).
type KindHealth = jobqueue.KindHealth

// healthWindow is how far back the queue's health counts failures: a day, as D233 §8 says.
const healthWindow = 24 * time.Hour

// Health is each kind's queued jobs, the oldest of them, and its jobs failed in the last day, for
// the System page (D233 §8).
func (queue *Queue) Health(ctx context.Context) ([]KindHealth, error) {
	return queue.queue.Health(ctx, queue.clock.Now().Add(-healthWindow))
}

// OnOverdue has the queue tell overdue each assigned job whose time, assigned_job_limit_hours, ran
// out: its node is told to stop the work it still holds (nodes.Exchanges.Overdue).
func (queue *Queue) OnOverdue(overdue func(ctx context.Context, id int64)) {
	queue.queue.OnOverdue(overdue)
}

// OnCancel has the queue tell cancelled each job a cancel ended: an assigned job's node is told to
// stop its work (D234 §6).
func (queue *Queue) OnCancel(cancelled func(ctx context.Context, id int64)) {
	queue.queue.OnCancel(cancelled)
}
