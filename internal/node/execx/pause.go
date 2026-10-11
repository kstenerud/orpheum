// SPDX-License-Identifier: AGPL-3.0-or-later

package execx

import (
	"context"
	"sync"
	"syscall"
	"time"
)

// Pausing pauses a watched run and continues it (D240 §6): its process group stopped with SIGSTOP
// and continued with SIGCONT, the tool keeping its files and memory and using neither disk nor
// processor meanwhile. Paused time is neither a stall nor spent deadline. A run paused before its
// tool is started begins paused. The zero value is a run not paused.
type Pausing struct {
	mutex    sync.Mutex
	isPaused bool      // guarded by mutex
	pid      int       // guarded by mutex: the running tool's, 0 when none runs
	dog      *watchdog // guarded by mutex: the running tool's watchdog
}

// pausingKey is the context key a Pausing travels under.
type pausingKey struct{}

// WithPausing is ctx carrying pausing, which pauses and continues every watched run made on it
// (RunWatched): the work that runs one need not know it can be paused.
func WithPausing(ctx context.Context, pausing *Pausing) context.Context {
	return context.WithValue(ctx, pausingKey{}, pausing)
}

// movedKey is the context key a run's movement hook travels under.
type movedKey struct{}

// WithMoved is ctx carrying moved, which every watched run made on it calls each time it moves:
// its position advances, or it reads or writes. The node's work list counts a job's movement by it.
func WithMoved(ctx context.Context, moved func()) context.Context {
	return context.WithValue(ctx, movedKey{}, moved)
}

// Moved calls the movement hook ctx carries, if it carries one: for work that reads in the node's
// own process, which moves as it reads.
func Moved(ctx context.Context) {
	if moved := movedOf(ctx); moved != nil {
		moved()
	}
}

// movedOf is the movement hook ctx carries, or nil.
func movedOf(ctx context.Context) func() {
	if moved, isMoved := ctx.Value(movedKey{}).(func()); isMoved {
		return moved
	}
	return nil
}

// pausingOf is the Pausing ctx carries, or nil.
func pausingOf(ctx context.Context) *Pausing {
	if pausing, isPausing := ctx.Value(pausingKey{}).(*Pausing); isPausing {
		return pausing
	}
	return nil
}

// Pause stops the run's process group until Continue, or keeps it from running once it starts.
func (pausing *Pausing) Pause() {
	pausing.mutex.Lock()
	defer pausing.mutex.Unlock()
	if pausing.isPaused {
		return
	}
	pausing.isPaused = true
	pausing.signal(syscall.SIGSTOP)
	if pausing.dog != nil {
		pausing.dog.pause()
	}
}

// Continue continues a paused run.
func (pausing *Pausing) Continue() {
	pausing.mutex.Lock()
	defer pausing.mutex.Unlock()
	if !pausing.isPaused {
		return
	}
	pausing.isPaused = false
	if pausing.dog != nil {
		pausing.dog.resume()
	}
	pausing.signal(syscall.SIGCONT)
}

// IsPaused is whether the run is paused.
func (pausing *Pausing) IsPaused() bool {
	pausing.mutex.Lock()
	defer pausing.mutex.Unlock()
	return pausing.isPaused
}

// attach binds a started tool to the pausing, stopping it at once when the run is paused.
func (pausing *Pausing) attach(pid int, dog *watchdog) {
	pausing.mutex.Lock()
	defer pausing.mutex.Unlock()
	pausing.pid, pausing.dog = pid, dog
	if pausing.isPaused {
		pausing.signal(syscall.SIGSTOP)
		dog.pause()
	}
}

// detach unbinds a tool that has exited.
func (pausing *Pausing) detach() {
	pausing.mutex.Lock()
	defer pausing.mutex.Unlock()
	pausing.pid, pausing.dog = 0, nil
}

// signal sends sig to the running tool's process group, if one runs. The caller holds the mutex.
func (pausing *Pausing) signal(sig syscall.Signal) {
	if pausing.pid > 0 {
		// The negative pid names the process group: the tool and anything it started.
		_ = syscall.Kill(-pausing.pid, sig) //nolint:errcheck // a group already gone needs no signal
	}
}

// pause stops the watchdog's judging: a paused run neither stalls nor spends its deadline.
func (dog *watchdog) pause() {
	dog.mutex.Lock()
	defer dog.mutex.Unlock()
	dog.pausedAt = dog.clock.Now()
}

// resume moves the run's start and last movement on by the time it was paused, and judges again.
func (dog *watchdog) resume() {
	dog.mutex.Lock()
	defer dog.mutex.Unlock()
	if dog.pausedAt.IsZero() {
		return
	}
	paused := dog.clock.Now().Sub(dog.pausedAt)
	dog.startedAt, dog.lastAdvanceAt = dog.startedAt.Add(paused), dog.lastAdvanceAt.Add(paused)
	dog.pausedAt = time.Time{}
}
