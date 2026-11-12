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

// goneOnce is a registry with casts whose receiver is gone, casts never answered and guest receivers
// whose cast is over, which its first look ends, and the guest receivers live now.
type goneOnce struct {
	casts      []live.Cast
	unanswered []live.Cast
	guests     []live.Guest
	liveGuest  int64
}

func (gone *goneOnce) EndGuests() []live.Guest {
	ended := gone.guests
	gone.guests = nil
	return ended
}

func (gone *goneOnce) IsGuest(_, deviceID int64) bool {
	return deviceID == gone.liveGuest
}

// removals keeps each guest receiver removed, and the devices the start's removal kept.
type removals struct {
	removed chan int64
	kept    chan []int64
}

func (remover removals) RemoveGuestReceiver(_ context.Context, _, deviceID int64) error {
	remover.removed <- deviceID
	return nil
}

func (remover removals) RemoveEveryGuestReceiver(_ context.Context, isLive func(userID, deviceID int64) bool) error {
	kept := []int64{}
	for _, deviceID := range []int64{21, 22} {
		if isLive(1, deviceID) {
			kept = append(kept, deviceID)
		}
	}
	remover.kept <- kept
	return nil
}

// noRemovals is a remover the test does not look at.
func noRemovals() removals {
	return removals{removed: make(chan int64, 8), kept: make(chan []int64, 1)}
}

func (gone *goneOnce) EndGone() []live.Cast {
	ended := gone.casts
	gone.casts = nil
	return ended
}

func (gone *goneOnce) EndUnanswered() []live.Cast {
	ended := gone.unanswered
	gone.unanswered = nil
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
		endGoneCasts(ctx, registry, ender, noRemovals(), tick, slog.New(slog.DiscardHandler))
		close(stopped)
	}()
	tick <- time.Time{}
	if ended := testutil.Receive(t, ender.ended, "the gone cast's end"); ended != together.CastReceiverGone+" 7" {
		t.Errorf("the session was ended as %q; want cast 7 as %s", ended, together.CastReceiverGone)
	}
	cancel()
	testutil.Receive(t, stopped, "the loop's stop as its context ends")
}

// Each tick ends the casts never answered too, their sessions told unanswered, so a sender asking a
// receiver that was never handed the question is told (D358 §4, principles §28).
func TestEndGoneCasts_EndsEachUnansweredCastsSessionAtATick(t *testing.T) {
	t.Parallel()
	registry := &goneOnce{unanswered: []live.Cast{{ID: 8, UserID: 1, ReceiverDeviceID: 11}}}
	ender := endings{ended: make(chan string, 1)}
	tick := make(chan time.Time)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go endGoneCasts(ctx, registry, ender, noRemovals(), tick, slog.New(slog.DiscardHandler))
	tick <- time.Time{}
	if ended := testutil.Receive(t, ender.ended, "the unanswered cast's end"); ended != together.CastUnanswered+" 8" {
		t.Errorf("the session was ended as %q; want cast 8 as %s", ended, together.CastUnanswered)
	}
}

// As it starts, the loop removes the guest receivers a stopped core left, but not one live now; each
// tick then removes each guest receiver whose cast is over (D465 §1).
func TestEndGoneCasts_RemovesTheGuestReceiversLeftAtStartAndEachWhoseCastIsOver(t *testing.T) {
	t.Parallel()
	registry := &goneOnce{guests: []live.Guest{{UserID: 1, DeviceID: 31}}, liveGuest: 22}
	remover := noRemovals()
	tick := make(chan time.Time)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go endGoneCasts(ctx, registry, endings{ended: make(chan string, 1)}, remover, tick,
		slog.New(slog.DiscardHandler))
	if kept := testutil.Receive(t, remover.kept, "the start's removal"); len(kept) != 1 || kept[0] != 22 {
		t.Errorf("the start's removal kept %v; want the live guest receiver 22 alone", kept)
	}
	tick <- time.Time{}
	if removed := testutil.Receive(t, remover.removed, "a guest receiver's removal"); removed != 31 {
		t.Errorf("removed guest receiver %d; want 31, whose cast is over", removed)
	}
}
