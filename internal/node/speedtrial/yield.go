// SPDX-License-Identifier: AGPL-3.0-or-later

package speedtrial

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/clock"
)

// quietAfterRun is how long the trials wait after the last run of chunks ends before they go on: a
// person who starts one film often starts another, or seeks, soon after.
const quietAfterRun = 30 * time.Second

// errRunStarted is the cause a step is stopped with when a run of chunks starts.
var errRunStarted = errors.New("a run of chunks started")

// yielding keeps the trials out of a run of chunks' way: a step starts only when no run goes and
// none has for quietAfterRun, and a run starting stops the step going at once.
type yielding struct {
	clock clock.Clock

	mutex     sync.Mutex
	running   int
	lastEnded time.Time
	// changed is closed, and made again, whenever a run starts or ends.
	changed chan struct{}
	// stopStep stops the step going, nil when none goes.
	stopStep context.CancelCauseFunc
}

func newYielding(clockSource clock.Clock) *yielding {
	return &yielding{clock: clockSource, changed: make(chan struct{})}
}

// runStarts says a run of chunks has started: the step going is stopped at once, and none starts
// until every run started has ended, by the func it returns, and quietAfterRun has passed.
func (yielding *yielding) runStarts() (ends func()) {
	yielding.mutex.Lock()
	yielding.running++
	if yielding.stopStep != nil {
		yielding.stopStep(errRunStarted)
	}
	yielding.signalLocked()
	yielding.mutex.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			yielding.mutex.Lock()
			defer yielding.mutex.Unlock()
			yielding.running--
			yielding.lastEnded = yielding.clock.Now()
			yielding.signalLocked()
		})
	}
}

func (yielding *yielding) signalLocked() {
	close(yielding.changed)
	yielding.changed = make(chan struct{})
}

// awaitClear waits until a step may go, and returns its context, stopped with errRunStarted when a run
// starts, and the func that ends the step. It fails only when ctx ends.
func (yielding *yielding) awaitClear(ctx context.Context) (context.Context, func(), error) {
	for {
		yielding.mutex.Lock()
		wait := time.Duration(0)
		if !yielding.lastEnded.IsZero() {
			wait = quietAfterRun - yielding.clock.Now().Sub(yielding.lastEnded)
		}
		if yielding.running == 0 && wait <= 0 {
			stepCtx, stop := context.WithCancelCause(ctx)
			yielding.stopStep = stop
			yielding.mutex.Unlock()
			return stepCtx, func() {
				yielding.mutex.Lock()
				yielding.stopStep = nil
				yielding.mutex.Unlock()
				stop(nil)
			}, nil
		}
		changed, running := yielding.changed, yielding.running
		yielding.mutex.Unlock()
		if err := yielding.await(ctx, changed, running == 0, wait); err != nil {
			return nil, nil, err
		}
	}
}

// await waits for changed, or, when isQuiet, for wait to pass, or for ctx to end.
func (yielding *yielding) await(ctx context.Context, changed <-chan struct{}, isQuiet bool, wait time.Duration) error {
	var quiet <-chan time.Time
	if isQuiet {
		timer := yielding.clock.NewTimer(wait)
		defer timer.Stop()
		quiet = timer.C()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-changed:
	case <-quiet:
	}
	return nil
}

// isStoppedByRun is whether stepCtx was stopped because a run of chunks started.
func isStoppedByRun(stepCtx context.Context) bool {
	return errors.Is(context.Cause(stepCtx), errRunStarted)
}
