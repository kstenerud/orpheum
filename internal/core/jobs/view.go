// SPDX-License-Identifier: GPL-3.0-or-later

package jobs

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store/cachegen"
)

// ErrCancelled is the cause a running job's context ends with when a person cancels it: its
// handler stops what it started, a node's work included.
var ErrCancelled = errors.New("the job was cancelled")

// ErrNotOpen is a cancel of a job that is no longer queued or running, or was never there.
var ErrNotOpen = errors.New("no such job queued or running")

// View is one job as the job view shows it (cut-a §5.3).
type View struct {
	ID      int64
	Kind    Kind
	Subject int64
	State   string
	// Progress is how far it is, 0 to 1, or nil when not known.
	Progress   *float64
	Reason     string
	LastError  string
	Attempts   int64
	CreatedAt  time.Time
	FinishedAt *time.Time
}

// List is the newest jobs, newest first, at most limit of them.
func (queue *Queue) List(ctx context.Context, limit int64) ([]View, error) {
	rows, err := queue.cache.Read().ListJobs(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	views := make([]View, 0, len(rows))
	for _, row := range rows {
		view := View{
			ID: row.ID, Kind: Kind(row.Kind), Subject: ptr.ValueOrZero(row.Subject), State: row.State,
			Progress: row.Progress, Reason: ptr.ValueOrZero(row.Reason), LastError: ptr.ValueOrZero(row.LastError),
			Attempts: row.Attempts, CreatedAt: time.UnixMilli(row.CreatedAt),
		}
		if row.FinishedAt != nil {
			view.FinishedAt = new(time.UnixMilli(*row.FinishedAt))
		}
		views = append(views, view)
	}
	return views, nil
}

// Cancel cancels a queued or running job: a queued one never runs, and a running one's handler
// is told through its context, whose cause is ErrCancelled, so it can stop the work it started.
// Whatever the handler answers after, the job stays cancelled.
func (queue *Queue) Cancel(ctx context.Context, id int64) error {
	var cancelled int64
	err := queue.cache.Write(ctx, func(queries *cachegen.Queries) error {
		var cancelErr error
		cancelled, cancelErr = queries.CancelJob(ctx, cachegen.CancelJobParams{
			FinishedAt: new(queue.clock.Now().UnixMilli()), ID: id,
		})
		return cancelErr
	})
	if err != nil {
		return fmt.Errorf("cancel job %d: %w", id, err)
	}
	if cancelled == 0 {
		return ErrNotOpen
	}
	queue.mutex.Lock()
	stop, isRunning := queue.running[id]
	queue.mutex.Unlock()
	if isRunning {
		stop(ErrCancelled)
	}
	return nil
}

// SetProgress records how far a running job is, from 0 to 1.
func (queue *Queue) SetProgress(ctx context.Context, id int64, fraction float64) error {
	err := queue.cache.Write(ctx, func(queries *cachegen.Queries) error {
		return queries.SetJobProgress(ctx, cachegen.SetJobProgressParams{Progress: &fraction, ID: id})
	})
	if err != nil {
		return fmt.Errorf("record job %d's progress: %w", id, err)
	}
	return nil
}

// track keeps a running job's cancel until its handler returns.
func (queue *Queue) track(id int64, stop context.CancelCauseFunc) {
	queue.mutex.Lock()
	defer queue.mutex.Unlock()
	queue.running[id] = stop
}

func (queue *Queue) untrack(id int64) {
	queue.mutex.Lock()
	defer queue.mutex.Unlock()
	delete(queue.running, id)
}
