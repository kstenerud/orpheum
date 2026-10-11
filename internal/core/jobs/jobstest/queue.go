// SPDX-License-Identifier: AGPL-3.0-or-later

package jobstest

import (
	"context"

	"github.com/kstenerud/orpheum/internal/core/jobs"
)

// Queue is the job queue as a list: every request it took, in order.
type Queue []jobs.Request

// Enqueue lists job and says it was queued, unless a job of its kind for its target is listed
// already, as the real queue merges a repeat request into the open job and adds none.
func (queue *Queue) Enqueue(_ context.Context, job jobs.Request) (bool, error) {
	for _, open := range *queue {
		if open.Kind == job.Kind && open.Target == job.Target {
			return false, nil
		}
	}
	*queue = append(*queue, job)
	return true, nil
}
