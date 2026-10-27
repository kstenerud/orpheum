// SPDX-License-Identifier: AGPL-3.0-or-later

package playback

import (
	"log/slog"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
)

// How long the service holds what its players tell it is settings, read at each look at the upload:
// a playing player's interest then lapses after the setting's 40 s, not its starting two minutes
// (the constants sweep, 2026-10-20). One that cannot be read stays at its start.
func TestRefreshTiming_IsTheSettingsAndInterestLapsesByIt(t *testing.T) {
	t.Parallel()
	clock := clocktest.NewFake(time.Date(2026, 10, 20, 20, 0, 0, 0, time.UTC))
	playing := &tokenPlayback{fileID: 1, lastUsedAt: clock.Now()}
	service := uploadService(clock, 0, map[string]*tokenPlayback{"mac": playing})
	answers := keyedSettings{}
	answers[settings.ChunkInterestLapsesAfterSeconds] = 40
	answers[settings.ChunkCPUCeilingKeptSeconds] = 15
	answers[settings.LinkSteadinessRelaxesSeconds] = 20
	service.settings = answers
	service.refreshTiming(t.Context(), slog.New(slog.DiscardHandler))
	want := playbackTiming{
		interestLapsesAfter: 40 * time.Second, cpuCeilingFor: 15 * time.Second, speedFreshFor: speedFreshFor,
		steadinessRelaxesEvery: 20 * time.Second,
	}
	if got := service.timingNow(); got != want {
		t.Errorf("the timing read is %+v; want %+v, the link speed's, unread, at its start", got, want)
	}
	playing.wantFetched(askedFile{stream: "q2000k540p", dir: "1/chunks-q2000k540p/g", startMs: 30_000, isChunk: true})
	clock.Advance(41 * time.Second)
	if got := service.InterestsIn(1); len(got) != 0 {
		t.Errorf("playing and silent 41 s, the lapse at 40 s: %+v; want none", got)
	}
}
