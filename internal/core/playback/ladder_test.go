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
)

// cutAfterChunks is a chunks service whose film has no grid for its first noGridFor asks, then
// starts on its ladder; it counts the asks, and offers what offered says.
type cutAfterChunks struct {
	noGridFor, asked int
	offered          chunks.Offered
}

func (*cutAfterChunks) Stalled(context.Context, chunks.Playback, int64, int64) (chunks.Offer, error) {
	return chunks.Offer{}, nil
}

func (cut *cutAfterChunks) StartOnLadder(context.Context, chunks.Playback, int64, int64) (chunks.Offer, error) {
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
// on its ladder once they are kept within play_ladder_wait_ms; past it, it plays as its decision says
// (D260 §1).
func TestStartOnLadder_WaitsForAFilmsKeyframesUpToTheLadderWait(t *testing.T) {
	t.Parallel()
	for _, wait := range []struct {
		waitMs     int64
		noGridFor  int
		isOnLadder bool
	}{
		{waitMs: 2000, noGridFor: 3, isOnLadder: true},
		{waitMs: 2000, noGridFor: int(2000*time.Millisecond/ladderLookEvery) + 1, isOnLadder: false},
		// Set to nothing, it does not wait at all.
		{waitMs: 0, noGridFor: 1, isOnLadder: false},
	} {
		clock := clocktest.NewFake(time.Date(2026, 10, 7, 20, 0, 0, 0, time.UTC))
		cut := &cutAfterChunks{noGridFor: wait.noGridFor}
		service := &Service{
			clock: clock, chunks: cut, speeds: newLinkSpeeds(), settings: waitSettings{waitMs: wait.waitMs},
			tokens: newTokens(),
		}
		started := clock.Now()
		offer, why, err := service.startOnLadder(t.Context(), Holder{}, Request{}, tokenPlayback{fileID: 1}, 0)
		if err != nil || (offer != nil) != wait.isOnLadder || (why == "noGridYet") == wait.isOnLadder {
			t.Errorf("waiting %d ms, with no grid for %d looks: %+v, %v; want on its ladder %t", wait.waitMs,
				wait.noGridFor, offer, err, wait.isOnLadder)
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
		clock: clock, chunks: fake, speeds: newLinkSpeeds(), settings: waitSettings{}, tokens: newTokens(),
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
		clock: clock, chunks: cut, speeds: newLinkSpeeds(), settings: waitSettings{}, tokens: newTokens(),
	}
	offer, why, err := service.startOnLadder(t.Context(), Holder{}, Request{PreferredDelivery: decide.Direct},
		tokenPlayback{fileID: 1}, 0)
	if err != nil || offer != nil || why != "originalChosen" || cut.asked != 0 {
		t.Errorf("the original chosen: %+v, %q, %v, the ladder asked %d times; want no offer, originalChosen",
			offer, why, err, cut.asked)
	}
}
