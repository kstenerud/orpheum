// SPDX-License-Identifier: AGPL-3.0-or-later

package auth

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
)

// No more than hashesAtOnce password hashes run at once, and one that finds no turn before its wait
// ends is refused as too many: each takes argon2id's memory, and a few hundred sign-ins at once used
// it all (the second audit's K3, 2026-10-13).
func TestHashSlots_RunAtMostSoManyAtOnceAndRefuseOneThatWaitsTooLong(t *testing.T) {
	t.Parallel()
	slots := newHashSlots()
	release := make(chan struct{})
	var running, most atomic.Int64
	var group sync.WaitGroup
	for range hashesAtOnce {
		group.Go(func() {
			_ = slots.run(t.Context(), func() { //nolint:errcheck // the slot is free
				now := running.Add(1)
				for {
					seen := most.Load()
					if now <= seen || most.CompareAndSwap(seen, now) {
						break
					}
				}
				select {
				case <-release:
				case <-t.Context().Done():
				}
				running.Add(-1)
			})
		})
	}
	for running.Load() < hashesAtOnce {
		runtime.Gosched()
	}
	waiting, cancel := context.WithCancel(t.Context())
	cancel()
	ran := false
	if err := slots.run(waiting, func() { ran = true }); !errors.Is(err, ErrRateLimited) || ran {
		t.Errorf("a hash with every slot taken and its wait over: %v, ran %v; want ErrRateLimited, not run", err, ran)
	}
	close(release)
	group.Wait()
	if most.Load() > hashesAtOnce {
		t.Errorf("%d hashes ran at once; want at most %d", most.Load(), hashesAtOnce)
	}
}
