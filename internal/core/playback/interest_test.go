// SPDX-License-Identifier: AGPL-3.0-or-later

package playback

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/chunks"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
)

// stoppingChunks records each film whose unwanted runs it was asked to stop (Chunks.StopUnwanted).
type stoppingChunks struct {
	cutAfterChunks
	stopped []int64
}

func (fake *stoppingChunks) StopUnwanted(_ context.Context, fileID int64) error {
	fake.stopped = append(fake.stopped, fileID)
	return nil
}

// A playback wants the picture and sound its player last asked for, from the chunk it asked for; an
// init moves nothing, a paused player's position is its own word, and a player silent for
// interestLapsesAfter, no request, report or word, wants nothing (D309 §1, §3, §4).
func TestInterestsIn_AreWhatEachPlayerLastAskedForUntilItFallsSilent(t *testing.T) {
	t.Parallel()
	clock := clocktest.NewFake(time.Date(2026, 10, 17, 20, 0, 0, 0, time.UTC))
	playing := &tokenPlayback{fileID: 1, lastUsedAt: clock.Now()}
	service := uploadService(clock, 0, map[string]*tokenPlayback{"mac": playing, "other": {fileID: 2}})
	playing.wantFetched(false, "q2000k540p", 30_000, true)
	playing.wantFetched(true, "sound0-96k", 0, false) // the sound's init
	playing.wantFetched(false, "q1200k480p", 40_000, true)
	want := []chunks.Interest{
		{StreamName: "q1200k480p", PositionMs: 40_000}, {StreamName: "sound0-96k", PositionMs: 40_000},
	}
	if got := service.InterestsIn(1); !slices.Equal(got, want) {
		t.Fatalf("the interests = %+v; want the 480p it moved to and its sound, from 40 s", got)
	}
	playing.interest.isPaused = true
	playing.wantFetched(false, "q1200k480p", 90_000, true) // filling its buffer, paused
	if got := service.InterestsIn(1); got[0].PositionMs != 40_000 || !got[0].IsPaused {
		t.Errorf("paused, the interest = %+v; want it paused where it said, 40 s", got[0])
	}
	clock.Advance(interestLapsesAfter)
	playing.lastUsedAt = clock.Now() // a report, a chunk asked for, a word
	clock.Advance(interestLapsesAfter)
	if got := service.InterestsIn(1); len(got) != 2 {
		t.Errorf("heard from again: %+v; want both still wanted", got)
	}
	clock.Advance(time.Second)
	if got := service.InterestsIn(1); len(got) != 0 {
		t.Errorf("silent past interestLapsesAfter: %+v; want none", got)
	}
}

// A player's word replaces what it wanted, a stream it leaves empty kept, and the film's unwanted
// runs stop at once; a stall's offer is wanted beside it, from where it is made (D247 §2); leaving
// wants nothing (D309 §1, §2).
func TestSayInterest_ReplacesWhatThePlayerWantedAndStopsWhatNoOneWants(t *testing.T) {
	t.Parallel()
	clock := clocktest.NewFake(time.Date(2026, 10, 17, 20, 0, 0, 0, time.UTC))
	fake := &stoppingChunks{}
	holder := Holder{UserID: 1, SessionKey: "s", Reach: auth.EveryLibrary()}
	playing := &tokenPlayback{userID: 1, sessionKey: "s", fileID: 1, lastUsedAt: clock.Now()}
	service := uploadService(clock, 0, map[string]*tokenPlayback{"mac": playing})
	service.chunks = fake
	playing.wantFetched(true, "sound0-96k", 0, true)
	said := Interest{PictureStream: "q400k360p", PositionMs: 600_000, IsPaused: true}
	if err := service.SayInterest(t.Context(), "mac", holder, said); err != nil {
		t.Fatal(err)
	}
	playing.offer = &chunks.Offer{StreamName: "q700k432p", SoundStreamName: "sound0-96k", FromMs: 590_000}
	want := []chunks.Interest{
		{StreamName: "q400k360p", PositionMs: 600_000, IsPaused: true},
		{StreamName: "sound0-96k", PositionMs: 600_000, IsPaused: true},
		{StreamName: "q700k432p", PositionMs: 590_000},
		{StreamName: "sound0-96k", PositionMs: 590_000},
	}
	if got := service.InterestsIn(1); !slices.Equal(got, want) {
		t.Errorf("the interests = %+v; want %+v", got, want)
	}
	if err := service.Leave(t.Context(), "mac", holder); err != nil {
		t.Fatal(err)
	}
	if got := service.InterestsIn(1); len(got) != 0 {
		t.Errorf("after leaving: %+v; want none", got)
	}
	if !slices.Equal(fake.stopped, []int64{1, 1}) {
		t.Errorf("the films whose unwanted runs were stopped = %v; want film 1, at the word and the leave",
			fake.stopped)
	}
}
