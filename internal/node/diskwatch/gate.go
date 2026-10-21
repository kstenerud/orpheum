// SPDX-License-Identifier: AGPL-3.0-or-later

package diskwatch

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
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

// Gate runs the operations on the node's roots, one stuck operation at most on each place: a root,
// or a disk mounted inside one when it knows the mounts (KeyingByMounts), so a disk dead inside a
// root holds up only what is on it (D328 §1).
type Gate struct {
	clock clock.Clock
	// mountPointOf is the mount point holding a path, read from the mount table, which reads nothing
	// on the disk; nil keys every operation by its root.
	mountPointOf func(path string) (string, bool)

	mutex sync.Mutex
	stuck map[string]Stuck // guarded by mutex: by place
}

// NewGate returns a gate with no root stuck.
func NewGate(clockSource clock.Clock) *Gate {
	return &Gate{clock: clockSource, stuck: map[string]Stuck{}}
}

// KeyingByMounts has the gate hold an operation on a disk mounted inside a root apart from the
// root's others: mountPointOf says the mount point holding a path.
func (gate *Gate) KeyingByMounts(mountPointOf func(path string) (string, bool)) *Gate {
	gate.mountPointOf = mountPointOf
	return gate
}

// placeOf is the place an operation on root waiting on path holds when it is stuck: the mount point
// holding path when that is inside root, else root.
func (gate *Gate) placeOf(root, path string) string {
	if gate.mountPointOf == nil {
		return root
	}
	mountPoint, isKnown := gate.mountPointOf(path)
	if !isKnown || mountPoint == root || !isUnder(mountPoint, root) {
		return root
	}
	return mountPoint
}

// isUnder is whether path is under directory, not at it.
func isUnder(path, directory string) bool {
	return strings.HasPrefix(path, strings.TrimSuffix(directory, "/")+"/")
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
	place := gate.placeOf(root, path)
	var none Value
	if behind := gate.stuckOn(place); behind != nil {
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
	if !gate.markStuck(place, Stuck{Root: root, Path: path, Since: gate.clock.Now()}) {
		// Another operation on the place got stuck first: this one is as good as behind it.
		go func() {
			if got := <-done; got.err == nil {
				release(got.value)
			}
		}()
		return none, &StalledError{Root: root, Path: path, Waited: window}
	}
	go func() {
		got := <-done
		gate.unstick(place)
		if got.err == nil {
			release(got.value)
		}
	}()
	return none, &StalledError{Root: root, Path: path, Waited: window}
}

// Hold marks the place of root holding path stuck on path, by an operation the caller watches itself,
// as a listing's reading is, until the returned release is called; false, holding nothing, when the
// place is already stuck.
func (gate *Gate) Hold(root, path string) (release func(), isHeld bool) {
	place := gate.placeOf(root, path)
	if !gate.markStuck(place, Stuck{Root: root, Path: path, Since: gate.clock.Now()}) {
		return func() {}, false
	}
	return func() { gate.unstick(place) }, true
}

// IsStuck is whether an operation on the place of root holding path is stuck, and on what; path is
// the root itself for the root's own place.
func (gate *Gate) IsStuck(root, path string) (Stuck, bool) {
	stuck := gate.stuckOn(gate.placeOf(root, path))
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

func (gate *Gate) stuckOn(place string) *Stuck {
	gate.mutex.Lock()
	defer gate.mutex.Unlock()
	if stuck, isStuck := gate.stuck[place]; isStuck {
		return &stuck
	}
	return nil
}

// markStuck marks place stuck on an operation, and reports false when it already was.
func (gate *Gate) markStuck(place string, stuck Stuck) bool {
	gate.mutex.Lock()
	defer gate.mutex.Unlock()
	if _, isStuck := gate.stuck[place]; isStuck {
		return false
	}
	gate.stuck[place] = stuck
	return true
}

func (gate *Gate) unstick(place string) {
	gate.mutex.Lock()
	defer gate.mutex.Unlock()
	delete(gate.stuck, place)
}

// IsDiskFailure is whether err is a read its disk failed as one that does not answer does: an I/O
// error, a stale handle, a timeout or a host down, which a network share mounted to give up on a
// host that is gone returns (D233 §10).
func IsDiskFailure(err error) bool {
	return errors.Is(err, syscall.EIO) || errors.Is(err, syscall.ESTALE) || errors.Is(err, syscall.ETIMEDOUT) ||
		errors.Is(err, syscall.EHOSTDOWN)
}
