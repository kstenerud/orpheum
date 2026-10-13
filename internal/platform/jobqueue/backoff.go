// SPDX-License-Identifier: AGPL-3.0-or-later

package jobqueue

import (
	"context"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/backoff"
)

// LongestRetryDelay is the longest any retry waits: the doubling back-off stops there, and no
// jitter takes a wait past it. A job that keeps failing retries about once a day until its
// attempt limit; the limit, not an ever-longer wait, is what makes a failure final.
const LongestRetryDelay = 24 * time.Hour

// jitterFraction is how far a retry's wait may move either way, so that jobs which failed
// together do not all retry together (D231 §10).
const jitterFraction = 0.1

// doubledDelay is first doubled doublings times, stopping at LongestRetryDelay.
func doubledDelay(first time.Duration, doublings int64) time.Duration {
	return backoff.Doubled(first, LongestRetryDelay, doublings)
}

// jittered is wait moved by up to jitterFraction either way, at random, and never past
// LongestRetryDelay. If the random source fails, the wait is not moved: a retry still happens.
func (queue *Queue[T]) jittered(ctx context.Context, wait time.Duration) time.Duration {
	moved, err := backoff.Jittered(queue.random, wait, LongestRetryDelay, jitterFraction)
	if err != nil {
		queue.logger.WarnContext(ctx, "could not jitter a retry's wait", "err", err)
	}
	return moved
}
