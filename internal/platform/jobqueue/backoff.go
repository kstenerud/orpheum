// SPDX-License-Identifier: AGPL-3.0-or-later

package jobqueue

import (
	"context"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/backoff"
)

// FallbackLongestRetryDelay is the longest any retry waits when the policy cannot say. The policy's
// LongestRetryDelay is where the doubling back-off stops, and no jitter takes a wait past it: a job
// that keeps failing retries about that often until its attempt limit; the limit, not an
// ever-longer wait, is what makes a failure final.
const FallbackLongestRetryDelay = 24 * time.Hour

// jitterFraction is how far a retry's wait may move either way, so that jobs which failed
// together do not all retry together (D231 §10).
const jitterFraction = 0.1

// longestRetryDelay is the policy's longest wait, or FallbackLongestRetryDelay when it cannot be read
// or the queue has no policy, as one that only probes destinations.
func (queue *Queue[T]) longestRetryDelay(ctx context.Context) time.Duration {
	if queue.policy == nil {
		return FallbackLongestRetryDelay
	}
	longest, err := queue.policy.LongestRetryDelay(ctx)
	if err != nil || longest <= 0 {
		queue.logger.WarnContext(ctx, "could not read the longest retry delay; going on at a day", "err", err)
		return FallbackLongestRetryDelay
	}
	return longest
}

// doubledDelay is first doubled doublings times, stopping at the longest retry delay.
func (queue *Queue[T]) doubledDelay(ctx context.Context, first time.Duration, doublings int64) time.Duration {
	return backoff.Doubled(first, queue.longestRetryDelay(ctx), doublings)
}

// jittered is wait moved by up to jitterFraction either way, at random, and never past the longest
// retry delay. If the random source fails, the wait is not moved: a retry still happens.
func (queue *Queue[T]) jittered(ctx context.Context, wait time.Duration) time.Duration {
	moved, err := backoff.Jittered(queue.random, wait, queue.longestRetryDelay(ctx), jitterFraction)
	if err != nil {
		queue.logger.WarnContext(ctx, "could not jitter a retry's wait", "err", err)
	}
	return moved
}
