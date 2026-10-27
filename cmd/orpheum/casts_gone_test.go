// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"log/slog"
	"strconv"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/live"
	"github.com/kstenerud/orpheum/internal/core/together"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// goneOnce is a registry with one cast whose receiver is gone, which its first look ends.
type goneOnce struct {
	casts []live.Cast
}

func (gone *goneOnce) EndGone() []live.Cast {
	ended := gone.casts
	gone.casts = nil
	return ended
}

// endings keeps each cast's session ended, and why.
type endings struct {
	ended chan string
}

func (ender endings) EndCast(sessionID, _ int64, reason string) error {
	ender.ended <- reason + " " + strconv.FormatInt(sessionID, 10)
	return nil
}

// Each tick ends the casts whose receiver is gone, their sessions told receiverGone, so their senders
// are told (D364 §6).
func TestEndGoneCasts_EndsEachGoneReceiversSessionAtATick(t *testing.T) {
	t.Parallel()
	registry := &goneOnce{casts: []live.Cast{{ID: 7, UserID: 1, ReceiverDeviceID: 11}}}
	ender := endings{ended: make(chan string, 1)}
	tick := make(chan time.Time)
	stopped := make(chan struct{})
	ctx, cancel := context.WithCancel(t.Context())
	go func() {
		endGoneCasts(ctx, registry, ender, tick, slog.New(slog.DiscardHandler))
		close(stopped)
	}()
	tick <- time.Time{}
	if ended := testutil.Receive(t, ender.ended, "the gone cast's end"); ended != together.CastReceiverGone+" 7" {
		t.Errorf("the session was ended as %q; want cast 7 as %s", ended, together.CastReceiverGone)
	}
	cancel()
	testutil.Receive(t, stopped, "the loop's stop as its context ends")
}
