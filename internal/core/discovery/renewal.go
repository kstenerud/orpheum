// SPDX-License-Identifier: AGPL-3.0-or-later

package discovery

import (
	"context"
	"sync"
)

// renewal runs one session renewal at a time: a caller that finds one under way waits for its
// outcome rather than opening a session of its own, so the step loop, the signalling loop and the
// certificate job asking at once open one (the protocol's §2.3; C11).
type renewal struct {
	mutex   sync.Mutex
	running *renewing // guarded by mutex; nil when none runs
}

// renewing is one renewal under way, and its outcome once done is closed.
type renewing struct {
	done  chan struct{}
	token string
	err   error
}

// do runs renew, or waits for the one already running and takes its outcome. A waiter whose ctx ends
// stops waiting; the renewal runs on for the others.
func (one *renewal) do(ctx context.Context, renew func(context.Context) (string, error)) (string, error) {
	one.mutex.Lock()
	if running := one.running; running != nil {
		one.mutex.Unlock()
		select {
		case <-running.done:
			return running.token, running.err
		case <-ctx.Done():
			return "", context.Cause(ctx)
		}
	}
	running := &renewing{done: make(chan struct{})}
	one.running = running
	one.mutex.Unlock()
	running.token, running.err = renew(ctx)
	one.mutex.Lock()
	one.running = nil
	one.mutex.Unlock()
	close(running.done)
	return running.token, running.err
}
