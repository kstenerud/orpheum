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

// goneOnce is a registry with casts never answered and guest receivers whose cast is over, which its
// first look ends, and the guest receivers live now.
type goneOnce struct {
	unanswered []live.Cast
	guests     []live.Guest
	liveGuest  int64
	// idleable are the casts to guest receivers by code, and endedIdle the ones End ended, their
	// senders told.
	idleable []live.Cast
	// endedSilent are the ids of the casts ended silent, the hub having ended them.
	endedSilent []int64
	endedIdle   []int64
}

func (gone *goneOnce) GuestCastsEndingIdle() []live.Cast {
	return gone.idleable
}

func (gone *goneOnce) End(_, _, castID int64) (live.Cast, error) {
	gone.endedIdle = append(gone.endedIdle, castID)
	return live.Cast{ID: castID}, nil
}

func (gone *goneOnce) EndSilent(_, castID int64) (live.Cast, error) {
	gone.endedSilent = append(gone.endedSilent, castID)
	return live.Cast{ID: castID}, nil
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

func (gone *goneOnce) EndUnanswered() []live.Cast {
	ended := gone.unanswered
	gone.unanswered = nil
	return ended
}

// endings keeps each cast's session ended, and why, and has the casts in silent end silent, once.
type endings struct {
	ended  chan string
	silent chan together.EndedCast
	// idleFor is how long nothing has played in each cast, by id.
	idleFor map[int64]time.Duration
	// guestWaits takes the waits each look for a guest's casts' ends was given, where not nil.
	guestWaits chan [2]time.Duration
}

func (ender endings) EndGuestCasts(unansweredAfter, idleAfter time.Duration) []together.EndedCast {
	if ender.guestWaits != nil {
		ender.guestWaits <- [2]time.Duration{unansweredAfter, idleAfter}
	}
	return []together.EndedCast{}
}

func (ender endings) CastIdleFor(sessionID, _ int64) (time.Duration, error) {
	return ender.idleFor[sessionID], nil
}

func (ender endings) EndSilentCasts() []together.EndedCast {
	ended := []together.EndedCast{}
	for {
		select {
		case each := <-ender.silent:
			ended = append(ended, each)
		default:
			return ended
		}
	}
}

func (ender endings) EndCast(sessionID, _ int64, reason string) error {
	ender.ended <- reason + " " + strconv.FormatInt(sessionID, 10)
	return nil
}

// Each tick tells the senders of the casts the hub ended, their castee silent, each by the registry's
// EndSilent (D555 §3).
func TestEndGoneCasts_TellsTheSenderOfEachCastTheHubEndedSilent(t *testing.T) {
	t.Parallel()
	registry := &goneOnce{}
	ender := endings{ended: make(chan string, 1), silent: make(chan together.EndedCast, 1)}
	ender.silent <- together.EndedCast{CastID: 7, UserID: 1}
	tick := make(chan time.Time)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go endGoneCasts(ctx, registry, ender, noRemovals(), tick, slog.New(slog.DiscardHandler))
	tick <- time.Time{}
	tick <- time.Time{} // the first tick's work is done once the second is taken
	if len(registry.endedSilent) != 1 || registry.endedSilent[0] != 7 {
		t.Errorf("the registry ended casts %v; want the silent cast 7, its sender told", registry.endedSilent)
	}
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

// Each tick ends the casts to a guest receiver a device code made that have had nothing playing for
// live.GuestIdleAfter, their sessions told idle (D469 §3), and leaves one idle for less.
func TestEndGoneCasts_EndsACastToAGuestByCodeIdleForGuestIdleAfter(t *testing.T) {
	t.Parallel()
	registry := &goneOnce{idleable: []live.Cast{{ID: 9, UserID: 1, ReceiverDeviceID: 11}, {ID: 10, UserID: 1}}}
	ender := endings{ended: make(chan string, 2), idleFor: map[int64]time.Duration{
		9: live.GuestIdleAfter, 10: live.GuestIdleAfter - time.Second,
	}}
	tick := make(chan time.Time)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go endGoneCasts(ctx, registry, ender, noRemovals(), tick, slog.New(slog.DiscardHandler))
	tick <- time.Time{}
	if ended := testutil.Receive(t, ender.ended, "the idle cast's end"); ended != together.CastIdle+" 9" {
		t.Errorf("the session was ended as %q; want cast 9 as %s", ended, together.CastIdle)
	}
	tick <- time.Time{} // the first tick's work is done once the second is taken
	select {
	case ended := <-ender.ended:
		if ended == together.CastIdle+" 10" {
			t.Error("a cast idle for less than GuestIdleAfter was ended")
		}
	default:
	}
}

// Each tick ends a guest's casts by link by the waits every cast has, live.UnansweredAfter for a
// castee that never said hello and live.GuestIdleAfter for one with nothing playing, on the hub's one
// end path (D555 §2, §5).
func TestEndGoneCasts_EndsAGuestsCastsByLinkByTheWaitsOfEveryCast(t *testing.T) {
	t.Parallel()
	ender := endings{ended: make(chan string, 1), guestWaits: make(chan [2]time.Duration, 2)}
	tick := make(chan time.Time)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go endGoneCasts(ctx, &goneOnce{}, ender, noRemovals(), tick, slog.New(slog.DiscardHandler))
	tick <- time.Time{}
	waits := testutil.Receive(t, ender.guestWaits, "the look for a guest's casts' ends")
	if waits != [2]time.Duration{live.UnansweredAfter, live.GuestIdleAfter} {
		t.Errorf("a guest's casts were ended by the waits %v; want %v and %v", waits, live.UnansweredAfter,
			live.GuestIdleAfter)
	}
}
