// SPDX-License-Identifier: GPL-3.0-or-later

package jobqueue

import (
	"context"
	"encoding/binary"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/randbytes"
)

// LongestRetryDelay is the longest any retry waits: the doubling back-off stops there, and no
// jitter takes a wait past it. A job that keeps failing retries about once a day until its
// attempt limit; the limit, not an ever-longer wait, is what makes a failure final.
const LongestRetryDelay = 24 * time.Hour

// jitterFraction is how far a retry's wait may move either way, so that jobs which failed
// together do not all retry together (D231 §10).
const jitterFraction = 0.1

// doubledDelay is first doubled doublings times, stopping at LongestRetryDelay. Stopping also
// keeps it from overflowing, however many attempts a limit allows.
func doubledDelay(first time.Duration, doublings int64) time.Duration {
	wait := first
	for range doublings {
		if wait >= LongestRetryDelay/2 {
			return LongestRetryDelay
		}
		wait *= 2
	}
	return wait
}

// jittered is wait moved by up to jitterFraction either way, at random, and never past
// LongestRetryDelay. If the random source fails, the wait is not moved: a retry still happens.
func (queue *Queue[T]) jittered(ctx context.Context, wait time.Duration) time.Duration {
	raw, err := randbytes.Read(queue.random, 8)
	if err != nil {
		queue.logger.WarnContext(ctx, "could not jitter a retry's wait", "err", err)
		return min(wait, LongestRetryDelay)
	}
	// The top 53 bits make a fraction in [0, 1) that a float64 holds exactly.
	const mantissaBits = 53
	fraction := float64(binary.BigEndian.Uint64(raw)>>(64-mantissaBits)) / (1 << mantissaBits)
	offset := time.Duration((2*fraction - 1) * jitterFraction * float64(wait))
	return min(wait+offset, LongestRetryDelay)
}
