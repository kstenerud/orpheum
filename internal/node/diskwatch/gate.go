// SPDX-License-Identifier: AGPL-3.0-or-later

package diskwatch

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"syscall"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/clock"
)

// ErrStalled is an operation on a root its disk did not answer, or one not tried because an earlier
// one on the root is still stuck.
var ErrStalled = errors.New("its disk is not answering")

// StalledError is ErrStalled for an operation on Root, waiting on Path, for Waited.
type StalledError struct {
	Root, Path string
	Waited     time.Duration
	// IsBehindAnother is an operation not tried, its root already stuck on another.
	IsBehindAnother bool
}

func (stalled *StalledError) Error() string {
	if stalled.IsBehindAnother {
		return fmt.Sprintf("not tried: an earlier operation on %s is still waiting on its disk: %s", stalled.Root,
			ErrStalled)
	}
	return fmt.Sprintf("%s did not answer in %s: %s", stalled.Path, stalled.Waited, ErrStalled)
}

func (*StalledError) Unwrap() error { return ErrStalled }

// Stuck is an operation left stuck: on what path, since when.
type Stuck struct {
	Root, Path string
	Since      time.Time
}

// Gate runs the operations on the node's roots, one stuck operation at most on each.
type Gate struct {
	clock clock.Clock

	mutex sync.Mutex
	stuck map[string]Stuck // guarded by mutex: by root
}

// NewGate returns a gate with no root stuck.
func NewGate(clockSource clock.Clock) *Gate {
	return &Gate{clock: clockSource, stuck: map[string]Stuck{}}
}

// Operation is what an operation on a root reads, and how long it may take before it is stalled.
type Operation struct {
	Root, Path string
	Window     time.Duration
}

// Run runs operate on the operation's root, waiting on its path, and returns what it does, unless it
// has not returned within its window, when it is a *StalledError and the root is stuck until it
// returns, what it returns then given to release; or the root is already stuck, when it is a
// *StalledError run not at all. A ctx that ends first ends the wait, operate left to return to
// release as a stuck one would be, without making the root stuck.
func Run[Value any](
	ctx context.Context, gate *Gate, operation Operation, operate func() (Value, error), release func(Value),
) (Value, error) {
	root, path, window := operation.Root, operation.Path, operation.Window
	var none Value
	if behind := gate.stuckOn(root); behind != nil {
		return none, &StalledError{Root: root, Path: behind.Path, IsBehindAnother: true}
	}
	type returned struct {
		value Value
		err   error
	}
	done := make(chan returned, 1)
	go func() {
		value, err := operate()
		done <- returned{value: value, err: err}
	}()
	timer := gate.clock.NewTimer(window)
	defer timer.Stop()
	select {
	case got := <-done:
		return got.value, got.err
	case <-ctx.Done():
		go func() {
			if got := <-done; got.err == nil {
				release(got.value)
			}
		}()
		return none, fmt.Errorf("waiting on %s: %w", path, ctx.Err())
	case <-timer.C():
	}
	if !gate.markStuck(Stuck{Root: root, Path: path, Since: gate.clock.Now()}) {
		// Another operation on the root got stuck first: this one is as good as behind it.
		go func() {
			if got := <-done; got.err == nil {
				release(got.value)
			}
		}()
		return none, &StalledError{Root: root, Path: path, Waited: window}
	}
	go func() {
		got := <-done
		gate.unstick(root)
		if got.err == nil {
			release(got.value)
		}
	}()
	return none, &StalledError{Root: root, Path: path, Waited: window}
}

// Hold marks root stuck on path, by an operation the caller watches itself, as a listing's reading
// is, until the returned release is called; false, holding nothing, when the root is already stuck.
func (gate *Gate) Hold(root, path string) (release func(), isHeld bool) {
	if !gate.markStuck(Stuck{Root: root, Path: path, Since: gate.clock.Now()}) {
		return func() {}, false
	}
	return func() { gate.unstick(root) }, true
}

// IsStuck is whether an operation on root is stuck, and on what.
func (gate *Gate) IsStuck(root string) (Stuck, bool) {
	stuck := gate.stuckOn(root)
	if stuck == nil {
		return Stuck{}, false
	}
	return *stuck, true
}

// Stuck are the operations left stuck, the oldest first.
func (gate *Gate) Stuck() []Stuck {
	gate.mutex.Lock()
	defer gate.mutex.Unlock()
	stuck := make([]Stuck, 0, len(gate.stuck))
	for _, each := range gate.stuck {
		stuck = append(stuck, each)
	}
	slices.SortFunc(stuck, func(a, b Stuck) int { return a.Since.Compare(b.Since) })
	return stuck
}

func (gate *Gate) stuckOn(root string) *Stuck {
	gate.mutex.Lock()
	defer gate.mutex.Unlock()
	if stuck, isStuck := gate.stuck[root]; isStuck {
		return &stuck
	}
	return nil
}

// markStuck marks root stuck on an operation, and reports false when it already was.
func (gate *Gate) markStuck(stuck Stuck) bool {
	gate.mutex.Lock()
	defer gate.mutex.Unlock()
	if _, isStuck := gate.stuck[stuck.Root]; isStuck {
		return false
	}
	gate.stuck[stuck.Root] = stuck
	return true
}

func (gate *Gate) unstick(root string) {
	gate.mutex.Lock()
	defer gate.mutex.Unlock()
	delete(gate.stuck, root)
}

// IsDiskFailure is whether err is a read its disk failed as one that does not answer does: an I/O
// error, a stale handle, a timeout or a host down, which a network share mounted to give up on a
// host that is gone returns (D233 §10).
func IsDiskFailure(err error) bool {
	return errors.Is(err, syscall.EIO) || errors.Is(err, syscall.ESTALE) || errors.Is(err, syscall.ETIMEDOUT) ||
		errors.Is(err, syscall.EHOSTDOWN)
}
