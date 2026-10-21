// SPDX-License-Identifier: AGPL-3.0-or-later

package diskwatch_test

import (
	"context"
	"errors"
	"io/fs"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/node/diskwatch"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// blocking is an operation that returns only once the test releases it, as an open of a share
// whose host is gone returns once it is back.
type blocking struct {
	started, release chan struct{}
}

func newBlocking() blocking {
	return blocking{started: make(chan struct{}), release: make(chan struct{})}
}

func (operation blocking) run(ctx context.Context) func() (string, error) {
	return func() (string, error) {
		close(operation.started)
		select {
		case <-operation.release:
		case <-ctx.Done():
		}
		return "opened", nil
	}
}

// runStalling runs a blocking operation on root and steps the fake clock past its window until it
// answers.
func runStalling(
	t *testing.T, gate *diskwatch.Gate, fake *clocktest.Fake, root string, operation blocking,
	released chan<- string,
) error {
	t.Helper()
	answered := make(chan error, 1)
	go func() {
		_, err := diskwatch.Run(t.Context(), gate,
			diskwatch.Operation{Root: root, Path: root + "/film.mkv", Window: time.Minute}, operation.run(t.Context()),
			func(value string) { released <- value })
		answered <- err
	}()
	testutil.Receive(t, operation.started, "the operation's start")
	for {
		select {
		case err := <-answered:
			return err
		case <-t.Context().Done():
			t.Fatal("the stalled operation never answered")
		case <-time.After(5 * time.Millisecond):
			fake.AdvanceToNext()
		}
	}
}

// An operation that returns within its window answers; one that does not is stalled, naming its
// root and path, and what it returns later is released (D233 §10).
func TestGate_AnOperationPastItsWindowIsStalledAndReleasedWhenItReturns(t *testing.T) {
	t.Parallel()
	fake := clocktest.NewFake(time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC))
	gate := diskwatch.NewGate(fake)
	value, err := diskwatch.Run(t.Context(), gate,
		diskwatch.Operation{Root: "/m", Path: "/m/a.mkv", Window: time.Minute},
		func() (string, error) { return "opened", nil }, func(string) {})
	if err != nil || value != "opened" {
		t.Fatalf("a quick operation = %q, %v; want it answered", value, err)
	}
	released := make(chan string, 1)
	operation := newBlocking()
	err = runStalling(t, gate, fake, "/m", operation, released)
	var stalled *diskwatch.StalledError
	if !errors.As(err, &stalled) || !errors.Is(err, diskwatch.ErrStalled) || stalled.Path != "/m/film.mkv" ||
		stalled.IsBehindAnother {
		t.Fatalf("a stuck operation = %v; want it stalled waiting on /m/film.mkv", err)
	}
	if stuck := gate.Stuck(); len(stuck) != 1 || stuck[0].Root != "/m" {
		t.Errorf("the stuck operations = %+v; want the one on /m", stuck)
	}
	close(operation.release)
	if value := testutil.Receive(t, released, "the late answer's release"); value != "opened" {
		t.Errorf("released %q; want what the operation returned", value)
	}
}

// While an operation on a root is stuck, every other on it is stalled at once, running nothing,
// while those on other roots run; once it returns, the root runs operations again (D233 §10).
func TestGate_ARootWithAStuckOperationRunsNothingMoreUntilItReturns(t *testing.T) {
	t.Parallel()
	fake := clocktest.NewFake(time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC))
	gate := diskwatch.NewGate(fake)
	released := make(chan string, 1)
	operation := newBlocking()
	if err := runStalling(t, gate, fake, "/m", operation, released); !errors.Is(err, diskwatch.ErrStalled) {
		t.Fatalf("the first operation = %v; want it stalled", err)
	}
	ran := false
	_, err := diskwatch.Run(t.Context(), gate,
		diskwatch.Operation{Root: "/m", Path: "/m/b.mkv", Window: time.Minute},
		func() (string, error) { ran = true; return "", nil }, func(string) {})
	var stalled *diskwatch.StalledError
	if !errors.As(err, &stalled) || !stalled.IsBehindAnother || ran {
		t.Errorf("an operation behind the stuck one = %v, run %t; want it stalled, not run", err, ran)
	}
	if value, err := diskwatch.Run(t.Context(), gate,
		diskwatch.Operation{Root: "/other", Path: "/other/c.mkv", Window: time.Minute},
		func() (string, error) { return "opened", nil }, func(string) {}); err != nil || value != "opened" {
		t.Errorf("an operation on another root = %q, %v; want it run", value, err)
	}
	if _, isHeld := gate.Hold("/m", "/m"); isHeld {
		t.Error("a hold on the stuck root was taken; want it refused")
	}
	close(operation.release)
	testutil.Receive(t, released, "the stuck operation's release") // released once the root is open
	if value, err := diskwatch.Run(t.Context(), gate,
		diskwatch.Operation{Root: "/m", Path: "/m/b.mkv", Window: time.Minute},
		func() (string, error) { return "opened", nil }, func(string) {}); err != nil || value != "opened" {
		t.Errorf("once the stuck one returned, an operation = %q, %v; want it run", value, err)
	}
}

// The errors a network share mounted to give up returns for a host that is gone are a disk failing a
// read; a file missing or refused is not (D233 §10).
func TestIsDiskFailure_IsWhatAShareThatGivesUpReturns(t *testing.T) {
	t.Parallel()
	for _, check := range []struct {
		err  error
		want bool
	}{
		{&fs.PathError{Op: "open", Path: "/m/a.mkv", Err: syscall.EIO}, true},
		{&fs.PathError{Op: "open", Path: "/m/a.mkv", Err: syscall.ESTALE}, true},
		{&fs.PathError{Op: "open", Path: "/m/a.mkv", Err: syscall.ETIMEDOUT}, true},
		{&fs.PathError{Op: "open", Path: "/m/a.mkv", Err: syscall.EHOSTDOWN}, true},
		{&fs.PathError{Op: "open", Path: "/m/a.mkv", Err: syscall.ENOENT}, false},
		{&fs.PathError{Op: "open", Path: "/m/a.mkv", Err: syscall.EACCES}, false},
		{nil, false},
	} {
		if got := diskwatch.IsDiskFailure(check.err); got != check.want {
			t.Errorf("IsDiskFailure(%v) = %t; want %t", check.err, got, check.want)
		}
	}
}

// With the mounts known, a disk mounted inside a root is a place of its own: one stuck there holds
// up only what is on it, the root's other files and the root itself going on, where a disk dead
// inside a root held up every reading of the root (D328 §1).
func TestGate_ADiskStuckInsideARootHoldsUpOnlyWhatIsOnIt(t *testing.T) {
	t.Parallel()
	gate := diskwatch.NewGate(clocktest.NewFake(time.Date(2026, 10, 21, 23, 0, 0, 0, time.UTC))).
		KeyingByMounts(func(path string) (string, bool) {
			if path == "/media/disk1" || strings.HasPrefix(path, "/media/disk1/") {
				return "/media/disk1", true
			}
			return "/", true // the root is on the system's own disk
		})
	release, isHeld := gate.Hold("/media", "/media/disk1/Wing.mkv")
	if !isHeld {
		t.Fatal("the disk could not be held")
	}
	t.Cleanup(release)
	readOf := func(path string) error {
		_, err := diskwatch.Run(t.Context(), gate, diskwatch.Operation{Root: "/media", Path: path, Window: time.Minute},
			func() (string, error) { return "read", nil }, func(string) {})
		return err
	}
	var stalled *diskwatch.StalledError
	if err := readOf("/media/disk1/Metropolis.mkv"); !errors.As(err, &stalled) || !stalled.IsBehindAnother {
		t.Errorf("a read on the stuck disk: %v; want it not tried, behind the stuck one", err)
	}
	if err := readOf("/media/Metropolis.mkv"); err != nil {
		t.Errorf("a read beside the stuck disk: %v; want it run", err)
	}
	if _, isStuck := gate.IsStuck("/media", "/media"); isStuck {
		t.Error("the root itself reads as stuck; want only the disk inside it")
	}
	if stuck, isStuck := gate.IsStuck("/media", "/media/disk1"); !isStuck || stuck.Path != "/media/disk1/Wing.mkv" {
		t.Errorf("the disk: %+v, %t; want it stuck on Wing.mkv", stuck, isStuck)
	}
}
