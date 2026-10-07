// SPDX-License-Identifier: GPL-3.0-or-later

package playback

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/chunks"
	"github.com/kstenerud/orpheum/internal/core/chunks/qualities"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
)

// cutAfterChunks is a chunks service whose film has no grid for its first noGridFor asks, then
// starts on its ladder; it counts the asks.
type cutAfterChunks struct {
	noGridFor, asked int
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

func (*cutAfterChunks) ReleaseWanted(context.Context, int64) error { return nil }

// A playback of a film not cut yet waits for its keyframes, read from its file's index, and starts
// on its ladder once they are kept within ladderWaitFor; past it, it plays as its decision says
// (D260 §1).
func TestStartOnLadder_WaitsForAFilmsKeyframesUpToTheLadderWait(t *testing.T) {
	t.Parallel()
	for _, wait := range []struct {
		noGridFor  int
		isOnLadder bool
	}{
		{noGridFor: 3, isOnLadder: true},
		{noGridFor: int(ladderWaitFor/ladderLookEvery) + 1, isOnLadder: false},
	} {
		clock := clocktest.NewFake(time.Date(2026, 10, 7, 20, 0, 0, 0, time.UTC))
		cut := &cutAfterChunks{noGridFor: wait.noGridFor}
		service := &Service{clock: clock, chunks: cut, speeds: newLinkSpeeds()}
		started := clock.Now()
		offer, err := service.startOnLadder(t.Context(), Holder{}, Request{}, tokenPlayback{fileID: 1}, 0)
		if err != nil || (offer != nil) != wait.isOnLadder {
			t.Errorf("with no grid for %d looks: %+v, %v; want on its ladder %t", wait.noGridFor, offer, err,
				wait.isOnLadder)
		}
		if waited := clock.Now().Sub(started); waited > ladderWaitFor {
			t.Errorf("with no grid for %d looks, it waited %v; want at most %v", wait.noGridFor, waited, ladderWaitFor)
		}
	}
}
