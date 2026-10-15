// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// countingFlusher counts flushes, and whether each one's context was still live.
type countingFlusher struct {
	mutex      sync.Mutex
	flushes    int
	liveAtStop bool
	flushed    chan struct{}
}

func (flusher *countingFlusher) FlushSessionUses(ctx context.Context) error {
	flusher.mutex.Lock()
	defer flusher.mutex.Unlock()
	flusher.flushes++
	flusher.liveAtStop = ctx.Err() == nil
	select {
	case flusher.flushed <- struct{}{}:
	default:
	}
	return nil
}

// fixedLengths answers every duration setting with one length.
type fixedLengths time.Duration

func (length fixedLengths) Duration(context.Context, settings.Key) (time.Duration, error) {
	return time.Duration(length), nil
}

// The sessions' uses are written on the setting's interval, and once more as the server stops,
// with a context that is still live, so the last uses reach the database before it closes.
func TestWriteSessionUses_FlushesOnItsIntervalAndAtStop(t *testing.T) {
	t.Parallel()
	flusher := &countingFlusher{flushed: make(chan struct{}, 1)}
	ctx, cancel := context.WithCancel(t.Context())
	stopped := make(chan struct{})
	go func() {
		writeSessionUses(ctx, flusher, fixedLengths(time.Millisecond), slog.New(slog.DiscardHandler))
		close(stopped)
	}()
	testutil.Receive(t, flusher.flushed, "a flush on the interval")
	cancel()
	testutil.Receive(t, stopped, "the loop to stop")
	flusher.mutex.Lock()
	defer flusher.mutex.Unlock()
	if flusher.flushes < 2 || !flusher.liveAtStop {
		t.Errorf("%d flushes, the last with a live context: %t; want one on the interval and one at stop, live",
			flusher.flushes, flusher.liveAtStop)
	}
}
