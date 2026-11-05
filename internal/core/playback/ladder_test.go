// SPDX-License-Identifier: AGPL-3.0-or-later

package playback

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/chunks"
	"github.com/kstenerud/orpheum/internal/core/chunks/qualities"
	"github.com/kstenerud/orpheum/internal/core/decide"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// cutAfterChunks is a chunks service whose film has no grid on its keyframes for its first noGridFor
// asks, then starts on its ladder; a playback on time starts at once, as a film cut on time needs no
// keyframes. It counts the asks on keyframes, and offers what offered says.
type cutAfterChunks struct {
	noGridFor, asked int
	offered          chunks.Offered
}

func (*cutAfterChunks) Stalled(context.Context, chunks.Playback, int64, int64) (chunks.Offer, error) {
	return chunks.Offer{}, nil
}

func (cut *cutAfterChunks) StartOnLadder(
	_ context.Context, playback chunks.Playback, _, _ int64,
) (chunks.Offer, error) {
	if playback.IsOnTime {
		return chunks.Offer{Quality: qualities.Quality{Kbps: 2400, Height: 1080}, IsOnTime: true}, nil
	}
	cut.asked++
	if cut.asked <= cut.noGridFor {
		return chunks.Offer{}, fmt.Errorf("file 1: %w", chunks.ErrNoGrid)
	}
	return chunks.Offer{Quality: qualities.Quality{Kbps: 2400, Height: 1080}}, nil
}

func (*cutAfterChunks) IsReadyAt(context.Context, chunks.Playback, chunks.Offer, int64) (bool, error) {
	return false, nil
}

func (cut *cutAfterChunks) Offered(context.Context, chunks.Playback) (chunks.Offered, error) {
	return cut.offered, nil
}

func (*cutAfterChunks) ReleaseWanted(context.Context, int64) error { return nil }

func (*cutAfterChunks) ForgetFile(int64) {}

func (*cutAfterChunks) StopUnwanted(context.Context, int64) error { return nil }

func (*cutAfterChunks) InterestsBy(chunks.Interests) {}

// waitSettings are settings whose play_ladder_wait_ms is waitMs, server_upload_mbps unanswered.
type waitSettings struct{ waitMs int64 }

func (wait waitSettings) Int(_ context.Context, key settings.Key) (int64, error) {
	if key == settings.ServerUploadMbps {
		return 0, settings.ErrUnanswered
	}
	if key != settings.PlayLadderWaitMs {
		return 0, fmt.Errorf("no setting %s here", key)
	}
	return wait.waitMs, nil
}

func (waitSettings) Duration(context.Context, settings.Key) (time.Duration, error) { return 0, nil }

// A playback of a film not cut yet waits for its keyframes, read from its file's index, and starts
// on its ladder on them once they are kept within play_ladder_wait_ms; past it, a film its device
// cannot play as it is starts on its ladder cut on time, which needs none, and keeps that grid, and
// one it can plays as it is (D260 §1, D344 §2).
func TestStartOnLadder_WaitsForAFilmsKeyframesUpToTheLadderWaitThenStartsOnTime(t *testing.T) {
	t.Parallel()
	pastTheWait := int(2000*time.Millisecond/ladderLookEvery) + 1
	for _, wait := range []struct {
		waitMs     int64
		noGridFor  int
		delivery   decide.Delivery
		isOnLadder bool
		isOnTime   bool
	}{
		{waitMs: 2000, noGridFor: 3, delivery: decide.HLS, isOnLadder: true, isOnTime: false},
		{waitMs: 2000, noGridFor: pastTheWait, delivery: decide.HLS, isOnLadder: true, isOnTime: true},
		{waitMs: 2000, noGridFor: pastTheWait, delivery: decide.Direct, isOnLadder: false},
		// Set to nothing, it does not wait at all.
		{waitMs: 0, noGridFor: 1, delivery: decide.HLS, isOnLadder: true, isOnTime: true},
	} {
		clock := clocktest.NewFake(time.Date(2026, 10, 7, 20, 0, 0, 0, time.UTC))
		cut := &cutAfterChunks{noGridFor: wait.noGridFor}
		service := &Service{
			clock: clock, chunks: cut, speeds: newLinkSpeeds(storetest.OpenCache(t)),
			settings: waitSettings{waitMs: wait.waitMs},
			tokens:   newTokens(),
		}
		started := clock.Now()
		playback := tokenPlayback{fileID: 1, decision: decide.Decision{Delivery: wait.delivery}}
		offer, why, err := service.startOnLadder(t.Context(), Holder{}, Request{}, playback, 0)
		isAsWanted := (offer != nil) == wait.isOnLadder && (why == "noGridYet") != wait.isOnLadder &&
			(offer == nil || offer.IsOnTime == wait.isOnTime)
		if err != nil || !isAsWanted {
			t.Errorf("waiting %d ms for a %s film, with no grid for %d looks: %+v, %q, %v; want on its ladder %t, "+
				"on time %t", wait.waitMs, wait.delivery, wait.noGridFor, offer, why, err, wait.isOnLadder,
				wait.isOnTime)
		}
		service.keepStart(&playback, offer)
		if offer != nil && chunksPlaybackOf(playback).IsOnTime != wait.isOnTime {
			t.Errorf("waiting %d ms, the playback keeps on time %t; want %t", wait.waitMs, playback.isOnTime,
				wait.isOnTime)
		}
		if waited := clock.Now().Sub(started); waited > time.Duration(wait.waitMs)*time.Millisecond {
			t.Errorf("waiting %d ms, it waited %v", wait.waitMs, waited)
		}
	}
}

// A device that starts a film on its ladder has gone to it from every other film it played: their
// runs no one else wants stop at once (D309 §7).
func TestStartOnLadder_LeavesThePlayersOtherFilmsForGood(t *testing.T) {
	t.Parallel()
	clock := clocktest.NewFake(time.Date(2026, 10, 17, 20, 0, 0, 0, time.UTC))
	fake := &stoppingChunks{}
	before := &tokenPlayback{deviceID: 4, fileID: 2, interest: playerInterest{disposition: Paused}}
	service := &Service{
		clock: clock, chunks: fake, speeds: newLinkSpeeds(storetest.OpenCache(t)),
		settings: waitSettings{}, tokens: newTokens(),
	}
	service.tokens.playbackByToken["before"] = before
	_, _, err := service.startOnLadder(t.Context(), Holder{}, Request{DeviceID: 4}, tokenPlayback{fileID: 1}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if before.interest.disposition != Replaced || !slices.Equal(fake.stopped, []int64{2}) {
		t.Errorf("the device's other film: %q, stopped %v; want it replaced and film 2's runs stopped",
			before.interest.disposition, fake.stopped)
	}
}

// A person who chose the original file plays it, not its film's ladder, though the film is cut
// (web-client.md: playback is never a black box).
func TestStartOnLadder_APersonWhoChoseTheOriginalPlaysIt(t *testing.T) {
	t.Parallel()
	clock := clocktest.NewFake(time.Date(2026, 10, 24, 20, 0, 0, 0, time.UTC))
	cut := &cutAfterChunks{}
	service := &Service{
		clock: clock, chunks: cut, speeds: newLinkSpeeds(storetest.OpenCache(t)),
		settings: waitSettings{}, tokens: newTokens(),
	}
	offer, why, err := service.startOnLadder(t.Context(), Holder{}, Request{PreferredDelivery: decide.Direct},
		tokenPlayback{fileID: 1}, 0)
	if err != nil || offer != nil || why != "originalChosen" || cut.asked != 0 {
		t.Errorf("the original chosen: %+v, %q, %v, the ladder asked %d times; want no offer, originalChosen",
			offer, why, err, cut.asked)
	}
}
