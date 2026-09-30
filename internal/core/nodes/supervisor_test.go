// SPDX-License-Identifier: GPL-3.0-or-later

package nodes

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
)

// fakeProcess is a node child the test ends by hand.
type fakeProcess struct {
	done      chan struct{}
	once      sync.Once
	wasKilled bool
}

func (process *fakeProcess) Done() <-chan struct{} { return process.done }
func (process *fakeProcess) Err() error            { return errors.New("exit status 1") }
func (process *fakeProcess) Kill() error {
	process.once.Do(func() { process.wasKilled = true; close(process.done) })
	return nil
}
func (process *fakeProcess) exit() { process.once.Do(func() { close(process.done) }) }

// fakeLiveness says the node was heard at a time the test sets.
type fakeLiveness struct {
	mutex sync.Mutex
	heard time.Time
}

func (liveness *fakeLiveness) LastHeard(string) (time.Time, bool) {
	liveness.mutex.Lock()
	defer liveness.mutex.Unlock()
	return liveness.heard, !liveness.heard.IsZero()
}

func (liveness *fakeLiveness) set(heard time.Time) {
	liveness.mutex.Lock()
	defer liveness.mutex.Unlock()
	liveness.heard = heard
}

// fakeSettings holds every setting at its starting value, read from the registry, so the test
// cannot drift from what a new server starts with.
type fakeSettings struct{}

func (fakeSettings) Int(_ context.Context, key settings.Key) (int64, error) {
	return startingValue[int64](key)
}

func (fakeSettings) Duration(_ context.Context, key settings.Key) (time.Duration, error) {
	count, err := startingValue[int64](key)
	definition, _ := settings.Lookup(key)
	return definition.LengthOf(count), err
}

func (fakeSettings) Bool(_ context.Context, key settings.Key) (bool, error) {
	return startingValue[bool](key)
}

// startingValue is key's starting value in the registry.
func startingValue[T any](key settings.Key) (T, error) {
	var value T
	definition, found := settings.Lookup(key)
	if !found {
		return value, fmt.Errorf("%s is not a setting", key)
	}
	err := json.Unmarshal(definition.StartJSON, &value)
	return value, err
}

// syncLog is a log the supervisor writes while the test reads it.
type syncLog struct {
	mutex  sync.Mutex
	buffer bytes.Buffer
}

func (log *syncLog) Write(data []byte) (int, error) {
	log.mutex.Lock()
	defer log.mutex.Unlock()
	return log.buffer.Write(data)
}

func (log *syncLog) String() string {
	log.mutex.Lock()
	defer log.mutex.Unlock()
	return log.buffer.String()
}

type supervised struct {
	clock               *clocktest.Fake
	liveness            *fakeLiveness
	tick                chan time.Time
	log                 *syncLog
	mutex               sync.Mutex
	started             []*fakeProcess
	shouldFailNextStart bool
	stop                context.CancelFunc
	finished            chan error
}

func supervise(t *testing.T) *supervised {
	t.Helper()
	harness := &supervised{
		clock:    clocktest.NewFake(time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)),
		liveness: &fakeLiveness{},
		tick:     make(chan time.Time),
		log:      &syncLog{},
		finished: make(chan error, 1),
	}
	start := func(context.Context) (Process, error) {
		harness.mutex.Lock()
		defer harness.mutex.Unlock()
		if harness.shouldFailNextStart {
			harness.shouldFailNextStart = false
			return nil, errors.New("fork: resource temporarily unavailable")
		}
		process := &fakeProcess{done: make(chan struct{})}
		harness.started = append(harness.started, process)
		return process, nil
	}
	logger := slog.New(slog.NewTextHandler(harness.log, nil))
	supervisor := NewSupervisor(start, harness.liveness, "local", harness.clock, fakeSettings{}, logger)
	ctx, cancel := context.WithCancel(t.Context())
	harness.stop = cancel
	go func() { harness.finished <- supervisor.Run(ctx, harness.tick) }()
	t.Cleanup(func() {
		cancel()
		<-harness.finished
	})
	return harness
}

// advanceAndJudge advances the clock and ticks twice: the second tick is received only once the first has
// been acted on, so on return the supervisor has judged the new time.
func (harness *supervised) advanceAndJudge(step time.Duration) {
	harness.clock.Advance(step)
	harness.tick <- harness.clock.Now()
	harness.tick <- harness.clock.Now()
}

func (harness *supervised) startedProcesses() []*fakeProcess {
	harness.mutex.Lock()
	defer harness.mutex.Unlock()
	return append([]*fakeProcess(nil), harness.started...)
}

func TestSupervisor_RestartsANodeThatStaysSilent(t *testing.T) {
	t.Parallel()
	harness := supervise(t)
	harness.advanceAndJudge(0)
	if len(harness.startedProcesses()) != 1 {
		t.Fatalf("%d starts, want the node started at once", len(harness.startedProcesses()))
	}
	harness.liveness.set(harness.clock.Now().Add(10 * time.Second))
	harness.advanceAndJudge(40 * time.Second)
	if !strings.Contains(harness.log.String(), "the node is silent") || harness.startedProcesses()[0].wasKilled {
		t.Errorf("30 s after it was last heard: log %q; want it shown silent, not killed", harness.log.String())
	}
	harness.advanceAndJudge(90 * time.Second)
	if !harness.startedProcesses()[0].wasKilled {
		t.Fatal("120 s after it was last heard the node was not killed")
	}
	harness.advanceAndJudge(5 * time.Second)
	if len(harness.startedProcesses()) != 2 {
		t.Errorf("%d starts, want the node started again", len(harness.startedProcesses()))
	}
}

func TestSupervisor_StartsAnExitedNodeAgainAfterTheDelay(t *testing.T) {
	t.Parallel()
	harness := supervise(t)
	harness.advanceAndJudge(0)
	harness.startedProcesses()[0].exit()
	harness.advanceAndJudge(time.Second)
	if len(harness.startedProcesses()) != 1 {
		t.Fatalf("%d starts 1 s after the node exited, want no restart before 5 s", len(harness.startedProcesses()))
	}
	harness.advanceAndJudge(5 * time.Second)
	if len(harness.startedProcesses()) != 2 || !strings.Contains(harness.log.String(), "the node exited") {
		t.Errorf("%d starts, log %q; want it started again, and the exit reported",
			len(harness.startedProcesses()), harness.log.String())
	}
}

func TestSupervisor_ACouldNotStartIsTriedAgainAndStoppingKillsTheNode(t *testing.T) {
	t.Parallel()
	harness := supervise(t)
	harness.mutex.Lock()
	harness.shouldFailNextStart = true
	harness.mutex.Unlock()
	harness.advanceAndJudge(0)
	harness.advanceAndJudge(5 * time.Second)
	if len(harness.startedProcesses()) != 1 || !strings.Contains(harness.log.String(), "could not start the node") {
		t.Fatalf("%d starts, log %q; want the failure reported and the start retried",
			len(harness.startedProcesses()), harness.log.String())
	}
	harness.stop()
	if err := <-harness.finished; err != nil {
		t.Errorf("Run returned %v", err)
	}
	harness.finished <- nil // for the cleanup
	if !harness.startedProcesses()[0].wasKilled {
		t.Error("stopping the supervisor left the node running")
	}
}
