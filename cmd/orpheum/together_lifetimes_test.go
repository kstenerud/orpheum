// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/together"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// recordingEnder keeps the lifetimes each look was given.
type recordingEnder struct {
	looks chan together.Lifetimes
}

func (ender recordingEnder) EndStale(lifetimes together.Lifetimes) []int64 {
	ender.looks <- lifetimes
	return nil
}

// Each tick ends Watch Together's stale links and sessions by the settings' lifetimes as they read
// then (D346).
func TestEndStaleTogether_EndsByTheSettingsAtEachTick(t *testing.T) {
	t.Parallel()
	ender := recordingEnder{looks: make(chan together.Lifetimes, 1)}
	tick := make(chan time.Time)
	stopped := make(chan struct{})
	ctx, cancel := context.WithCancel(t.Context())
	go func() {
		endStaleTogether(ctx, ender, fixedLengths(90*time.Minute), tick, slog.New(slog.DiscardHandler))
		close(stopped)
	}()
	tick <- time.Time{}
	looked := testutil.Receive(t, ender.looks, "a look at the tick")
	if want := (together.Lifetimes{Link: 90 * time.Minute, Idle: 90 * time.Minute}); looked != want {
		t.Errorf("the look was given %+v; want the settings' %+v", looked, want)
	}
	cancel()
	testutil.Receive(t, stopped, "the loop's stop as its context ends")
}
