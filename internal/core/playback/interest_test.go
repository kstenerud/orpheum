// SPDX-License-Identifier: AGPL-3.0-or-later

package playback

import (
	"context"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/chunks"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
)

// stoppingChunks records each film whose unwanted runs it was asked to stop (Chunks.StopUnwanted),
// and each it was told no player waits for (Chunks.ReleaseWanted).
type stoppingChunks struct {
	cutAfterChunks
	stopped, released []int64
}

func (fake *stoppingChunks) ReleaseWanted(_ context.Context, fileID int64) error {
	fake.released = append(fake.released, fileID)
	return nil
}

func (fake *stoppingChunks) StopUnwanted(_ context.Context, fileID int64) error {
	fake.stopped = append(fake.stopped, fileID)
	return nil
}

// keepingSettings answer D309 §8's times as the registry starts them, and every other length nothing.
type keepingSettings struct{ uploadSettings }

func (keepingSettings) Duration(_ context.Context, key settings.Key) (time.Duration, error) {
	lengths := map[settings.Key]time.Duration{}
	lengths[settings.ChunkKeptLikelySeconds] = 5 * time.Minute
	lengths[settings.ChunkKeptLeftSeconds] = 10 * time.Minute
	lengths[settings.ChunkKeptMaybeSeconds] = 20 * time.Minute
	return lengths[key], nil
}

// A playback wants the picture and sound its player last asked for, from the chunk it asked for; an
// init moves nothing; a paused player's position is its own word, and it wants only what is kept
// ahead; a playing player silent for interestLapsesAfter wants nothing (D309 §1, §3, §7).
func TestInterestsIn_AreWhatEachPlayerLastAskedFor(t *testing.T) {
	t.Parallel()
	clock := clocktest.NewFake(time.Date(2026, 10, 17, 20, 0, 0, 0, time.UTC))
	playing := &tokenPlayback{fileID: 1, lastUsedAt: clock.Now()}
	service := uploadService(clock, 0, map[string]*tokenPlayback{"mac": playing, "other": {fileID: 2}})
	playing.wantFetched(false, "q2000k540p", "1/chunks-q2000k540p/g", 30_000, true)
	playing.wantFetched(true, "sound0-96k", "1/chunks-sound0-96k/g", 0, false) // the sound's init
	playing.wantFetched(false, "q1200k480p", "1/chunks-q1200k480p/g", 40_000, true)
	want := []chunks.Interest{
		{StreamName: "q1200k480p", PositionMs: 40_000}, {StreamName: "sound0-96k", PositionMs: 40_000},
	}
	if got := service.InterestsIn(1); !slices.Equal(got, want) {
		t.Fatalf("the interests = %+v; want the 480p it moved to and its sound, from 40 s", got)
	}
	clock.Advance(interestLapsesAfter + time.Second)
	if got := service.InterestsIn(1); len(got) != 0 {
		t.Errorf("playing and silent past interestLapsesAfter: %+v; want none", got)
	}
	playing.lastUsedAt = clock.Now() // a report, a chunk asked for, a word
	if got := service.InterestsIn(1); len(got) != 2 {
		t.Errorf("heard from again: %+v; want both wanted again", got)
	}
	playing.interest.disposition = Paused
	playing.wantFetched(false, "q1200k480p", "1/chunks-q1200k480p/g", 90_000, true) // filling its buffer
	if got := service.InterestsIn(1); len(got) != 2 || got[0].PositionMs != 40_000 || !got[0].IsStopped {
		t.Errorf("paused, the interests = %+v; want both stopped where it paused, 40 s", got)
	}
}

// A player's word replaces what it wanted, a stream it leaves empty kept, and the film's unwanted
// runs stop at once; a stall's offer is wanted beside it (D247 §2). Leaving its page, it wants what it
// keeps ahead, its offer no more; gone to another film, nothing (D309 §1, §2, §7).
func TestSayInterest_ReplacesWhatThePlayerWantedAndLeavingSaysWhy(t *testing.T) {
	t.Parallel()
	clock := clocktest.NewFake(time.Date(2026, 10, 17, 20, 0, 0, 0, time.UTC))
	fake := &stoppingChunks{}
	holder := Holder{UserID: 1, SessionKey: "s", Reach: auth.EveryLibrary()}
	playing := &tokenPlayback{userID: 1, sessionKey: "s", fileID: 1, lastUsedAt: clock.Now()}
	service := uploadService(clock, 0, map[string]*tokenPlayback{"mac": playing})
	service.chunks = fake
	playing.wantFetched(true, "sound0-96k", "1/chunks-sound0-96k/g", 0, true)
	said := Interest{PictureStream: "q400k360p", PositionMs: 600_000}
	if err := service.SayInterest(t.Context(), "mac", holder, said); err != nil {
		t.Fatal(err)
	}
	playing.offer = &chunks.Offer{StreamName: "q700k432p", SoundStreamName: "sound0-96k", FromMs: 590_000}
	want := []chunks.Interest{
		{StreamName: "q400k360p", PositionMs: 600_000},
		{StreamName: "sound0-96k", PositionMs: 600_000},
		{StreamName: "q700k432p", PositionMs: 590_000},
		{StreamName: "sound0-96k", PositionMs: 590_000},
	}
	if got := service.InterestsIn(1); !slices.Equal(got, want) {
		t.Errorf("the interests = %+v; want %+v", got, want)
	}
	if err := service.Leave(t.Context(), "mac", holder, Left); err != nil {
		t.Fatal(err)
	}
	want = []chunks.Interest{
		{StreamName: "q400k360p", PositionMs: 600_000, IsStopped: true},
		{StreamName: "sound0-96k", PositionMs: 600_000, IsStopped: true},
	}
	if got := service.InterestsIn(1); !slices.Equal(got, want) {
		t.Errorf("after leaving its page: %+v; want what it keeps ahead, %+v", got, want)
	}
	if len(fake.released) != 0 {
		t.Errorf("leaving its page let go of film %v's work; want it kept until another film is chosen", fake.released)
	}
	if err := service.Leave(t.Context(), "mac", holder, Replaced); err != nil {
		t.Fatal(err)
	}
	if got := service.InterestsIn(1); len(got) != 0 || !slices.Equal(fake.released, []int64{1}) {
		t.Errorf("gone to another film: %+v, released %v; want none, film 1's work let go of", got, fake.released)
	}
	if !slices.Equal(fake.stopped, []int64{1, 1, 1}) {
		t.Errorf("the films whose unwanted runs were stopped = %v; want film 1 at the word and at each leave",
			fake.stopped)
	}
}

// A player that starts another film has gone to it from every other film it played, paused or not:
// their runs stop at once; another player's are left alone (D309 §7).
func TestReplaceOthers_APlayerStartingAnotherFilmLeavesTheOthersForGood(t *testing.T) {
	t.Parallel()
	clock := clocktest.NewFake(time.Date(2026, 10, 17, 20, 0, 0, 0, time.UTC))
	paused := &tokenPlayback{deviceID: 4, fileID: 1, interest: playerInterest{disposition: Paused}}
	again := &tokenPlayback{deviceID: 4, fileID: 2}
	elsewhere := &tokenPlayback{deviceID: 5, fileID: 1}
	service := uploadService(clock, 0, map[string]*tokenPlayback{"a": paused, "b": again, "c": elsewhere})
	if replaced := service.tokens.replaceOthers(playerOf(4, ""), 2); !slices.Equal(replaced, []int64{1}) {
		t.Errorf("the films left for good = %v; want film 1", replaced)
	}
	if paused.interest.disposition != Replaced || again.interest.disposition != Playing ||
		elsewhere.interest.disposition != Playing {
		t.Errorf("the dispositions = %q, %q, %q; want the device's other film replaced, alone",
			paused.interest.disposition, again.interest.disposition, elsewhere.interest.disposition)
	}
}

// Each store is kept at the highest tier any playback gives it, by where its player stands and how
// long since it was last heard (D309 §8): in use while playing; likely, paused or silent, for 5
// minutes; maybe to 20, and for 10 once it left its page; unlikely, unnamed, once it went to another
// film and after.
func TestStoreTiers_AreTheHighestAnyPlayerGivesEachStore(t *testing.T) {
	t.Parallel()
	clock := clocktest.NewFake(time.Date(2026, 10, 17, 20, 0, 0, 0, time.UTC))
	now := clock.Now()
	heardAgo := func(dir string, disposition Disposition, ago time.Duration) *tokenPlayback {
		return &tokenPlayback{
			lastUsedAt: now.Add(-ago), interest: playerInterest{pictureDir: dir, disposition: disposition},
		}
	}
	service := uploadService(clock, 0, map[string]*tokenPlayback{
		"playing":       heardAgo("playing", Playing, time.Minute),
		"silent":        heardAgo("silent", Playing, 3*time.Minute),
		"paused":        heardAgo("paused", Paused, 4*time.Minute),
		"paused longer": heardAgo("paused longer", Paused, 6*time.Minute),
		"paused gone":   heardAgo("paused gone", Paused, 21*time.Minute),
		"left":          heardAgo("left", Left, 9*time.Minute),
		"left gone":     heardAgo("left gone", Left, 11*time.Minute),
		"replaced":      heardAgo("replaced", Replaced, 0),
		// The same store, played by a player paused a moment and by one that left: the higher keeps it.
		"shared paused": heardAgo("shared", Paused, time.Minute),
		"shared left":   heardAgo("shared", Left, time.Minute),
	})
	service.settings = keepingSettings{}
	tiers, err := service.StoreTiers(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]nodeproto.KeepTier{
		"playing": nodeproto.KeepInUse, "silent": nodeproto.KeepLikely, "paused": nodeproto.KeepLikely,
		"paused longer": nodeproto.KeepMaybe, "left": nodeproto.KeepMaybe, "shared": nodeproto.KeepLikely,
	}
	if !maps.Equal(tiers, want) {
		t.Errorf("the tiers = %v; want %v", tiers, want)
	}
}
