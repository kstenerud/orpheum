// SPDX-License-Identifier: AGPL-3.0-or-later

package backoff

import (
	"encoding/binary"
	"io"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/randbytes"
)

// Doubled is first doubled doublings times, stopping at most. Stopping also keeps it from
// overflowing, however many doublings are asked.
func Doubled(first, most time.Duration, doublings int64) time.Duration {
	wait := min(first, most)
	for range doublings {
		if wait >= most/2 {
			return most
		}
		wait *= 2
	}
	return wait
}

// Jittered is wait moved by up to fraction of it either way, at random from random, and never past
// most. If random fails, the wait is not moved, and the error says why: a retry still happens.
func Jittered(random io.Reader, wait, most time.Duration, fraction float64) (time.Duration, error) {
	raw, err := randbytes.Read(random, 8)
	if err != nil {
		return min(wait, most), err
	}
	// The top 53 bits make a share in [0, 1) that a float64 holds exactly.
	const mantissaBits = 53
	share := float64(binary.BigEndian.Uint64(raw)>>(64-mantissaBits)) / (1 << mantissaBits)
	offset := time.Duration((2*share - 1) * fraction * float64(wait))
	return min(wait+offset, most), nil
}
