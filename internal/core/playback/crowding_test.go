// SPDX-License-Identifier: AGPL-3.0-or-later

package playback

import (
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
)

// While the house's upload is crowded the films' share is lowered by a fifth at most every
// crowdedLowerEvery, never under a tenth of the setting's; once uncrowdedFor has passed without a
// sign of it, it grows back a tenth every crowdedRaiseEvery until it is the setting's again.
func TestCrowding_LowersTheShareWhileCrowdedAndGrowsItBackOnceItStops(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 10, 12, 16, 0, 0, 0, time.UTC)
	crowded := &crowding{}
	lookAfter := func(after time.Duration, isCrowded bool) (int64, bool) {
		return crowded.look(10_000, isCrowded, start.Add(after), starting)
	}
	if share, isChanged := lookAfter(0, false); share != 10_000 || isChanged {
		t.Fatalf("uncrowded: %d, changed %t; want the setting's 10000, unchanged", share, isChanged)
	}
	if share, isChanged := lookAfter(0, true); share != 8000 || !isChanged {
		t.Fatalf("crowded: %d; want lowered by a fifth, 8000", share)
	}
	if share, _ := lookAfter(5*time.Second, true); share != 8000 {
		t.Errorf("crowded again 5 s later: %d; want 8000, not lowered before crowdedLowerEvery", share)
	}
	for step := 1; step <= 20; step++ {
		lookAfter(time.Duration(step)*crowdedLowerEvery, true)
	}
	floorAt := 20 * crowdedLowerEvery
	if share, _ := lookAfter(floorAt, true); share != 1000 {
		t.Errorf("long crowded: %d; want the floor, a tenth of the setting's, 1000", share)
	}
	if share, _ := lookAfter(floorAt+uncrowdedFor-time.Second, false); share != 1000 {
		t.Errorf("before uncrowdedFor has passed: %d; want still 1000", share)
	}
	if share, isChanged := lookAfter(floorAt+uncrowdedFor, false); share != 1100 || !isChanged {
		t.Errorf("after uncrowdedFor: %d; want grown a tenth, 1100", share)
	}
	if share, _ := lookAfter(floorAt+uncrowdedFor+time.Second, false); share != 1100 {
		t.Errorf("a second later: %d; want 1100, not grown before crowdedRaiseEvery", share)
	}
	for step := 1; step <= 40; step++ {
		lookAfter(floorAt+uncrowdedFor+time.Duration(step)*crowdedRaiseEvery, false)
	}
	if crowded.shareKbps != 0 {
		t.Errorf("grown back for long: %d kept; want the setting's whole share again", crowded.shareKbps)
	}
}

// The house's upload is crowded when two players at once report links too slow for what they need
// while the films need no more than the share; one slow player is its own link, and slow players
// with the films over the share are the films' own doing.
func TestIsCrowded_IsTwoSlowPlayersWhileTheFilmsAreWithinTheShare(t *testing.T) {
	t.Parallel()
	clock := clocktest.NewFake(time.Date(2026, 10, 12, 16, 0, 0, 0, time.UTC))
	now := clock.Now()
	playing := func(needKbps, linkKbps int64) *tokenPlayback {
		return &tokenPlayback{
			picturesNeed: streamNeed{kbps: needKbps, at: now}, link: linkReport{kbps: linkKbps, at: now},
		}
	}
	slowTwo := uploadService(clock, 0, map[string]*tokenPlayback{
		"tv": playing(1296, 1500), "phone": playing(796, 900), "tablet": playing(796, 5000),
	})
	if !slowTwo.isCrowded(10_000, now, starting) {
		t.Error("two slow players within the share: not crowded; want crowded")
	}
	if slowTwo.isCrowded(2000, now, starting) {
		t.Error("two slow players with the films over the share: crowded; want the films' own doing")
	}
	slowOne := uploadService(clock, 0, map[string]*tokenPlayback{
		"tv": playing(1296, 1500), "phone": playing(796, 5000),
	})
	if slowOne.isCrowded(10_000, now, starting) {
		t.Error("one slow player: crowded; want its own link")
	}
	clock.Advance(linkReportedWithin + time.Second)
	for _, playback := range slowTwo.tokens.playbackByToken {
		playback.picturesNeed.at = clock.Now() // still playing, its reports old
	}
	if slowTwo.isCrowded(10_000, clock.Now(), starting) {
		t.Error("slow reports older than linkReportedWithin: crowded; want them not counted")
	}
}

// While the house's upload is crowded a start takes what is left of the lowered share.
func TestStartAtMostKbps_TakesFromTheShareTheCrowdingLeaves(t *testing.T) {
	t.Parallel()
	clock := clocktest.NewFake(time.Date(2026, 10, 12, 16, 0, 0, 0, time.UTC))
	service := uploadService(clock, 4, map[string]*tokenPlayback{}) // a share of 3400
	service.crowdedShareKbps.Store(1200)
	if got, err := service.startAtMostKbps(t.Context(), Holder{UserID: 2}, 91); err != nil || got != 1200 {
		t.Errorf("a start, the house crowded, may take %d (%v); want the lowered share, 1200", got, err)
	}
}
