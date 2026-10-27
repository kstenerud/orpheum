// SPDX-License-Identifier: AGPL-3.0-or-later

package playback

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/settings"
)

// The upload controller's tests run at the settings' starting values, by the names the controller's
// constants had before the constants sweep (2026-10-20).
var (
	starting          = startingUploadTuning()
	crowdedLowerEvery = starting.crowdedLowerEvery
	crowdedRaiseEvery = starting.uncrowdedRaiseEvery
	uncrowdedFor      = starting.uncrowdedFor
	roomFor           = starting.raiseRoom
	overAtLowestFor   = starting.sendAwayAfter
	sentAwaySettles   = starting.sentAwaySettles()
)

// The playbacks' timing in tests: its starting values, by the names its constants had.
var (
	interestLapsesAfter = startingPlaybackTiming().interestLapsesAfter
	cpuCeilingFor       = startingPlaybackTiming().cpuCeilingFor
	speedFreshFor       = startingPlaybackTiming().speedFreshFor
	relaxEvery          = startingPlaybackTiming().steadinessRelaxesEvery
)

// keyedSettings answers each key it has, and fails every other.
type keyedSettings map[settings.Key]int64

func (answers keyedSettings) Int(_ context.Context, key settings.Key) (int64, error) {
	if value, found := answers[key]; found {
		return value, nil
	}
	return 0, errors.New("the database is gone")
}

func (answers keyedSettings) Duration(ctx context.Context, key settings.Key) (time.Duration, error) {
	seconds, err := answers.Int(ctx, key)
	return time.Duration(seconds) * time.Second, err
}

// Each of the controller's values is its setting's, and one that cannot be read its starting value
// (the constants sweep, 2026-10-20).
func TestUploadTuningOf_EachValueIsItsSettingsAndAnUnreadOneItsStart(t *testing.T) {
	t.Parallel()
	answers := keyedSettings{}
	answers[settings.UploadFilmsSharePercent] = 70
	answers[settings.UploadCrowdedSlowUnderPercent] = 120
	answers[settings.UploadCrowdedSlowPlayersAtLeast] = 3
	answers[settings.UploadCrowdedKeepsPercent] = 60
	answers[settings.UploadCrowdedLowerEverySeconds] = 5
	answers[settings.UploadCrowdedFloorPercent] = 20
	answers[settings.UploadUncrowdedForSeconds] = 90
	answers[settings.UploadUncrowdedGrowsPercent] = 25
	answers[settings.UploadUncrowdedRaiseEverySeconds] = 15
	answers[settings.UploadRaiseUnderPercent] = 50
	answers[settings.UploadRaiseRoomSeconds] = 45
	service := &Service{settings: answers}
	got := service.uploadTuningOf(t.Context(), slog.New(slog.DiscardHandler))
	want := uploadTuning{
		filmsSharePercent: 70, slowUnderPercent: 120, slowPlayersAtLeast: 3, crowdedKeepsPercent: 60,
		crowdedLowerEvery: 5 * time.Second, crowdedFloorPercent: 20, uncrowdedFor: 90 * time.Second,
		uncrowdedGrowsPercent: 25, uncrowdedRaiseEvery: 15 * time.Second, raiseUnderPercent: 50,
		raiseRoom: 45 * time.Second, sendAwayAfter: starting.sendAwayAfter,
	}
	if got != want {
		t.Errorf("the tuning read is %+v; want %+v, the send-away wait, unread, at its start", got, want)
	}
	if got.shareKbps(10) != 7000 {
		t.Errorf("at 70%%, the films' share of 10 Mbit/s is %d kbit/s; want 7000", got.shareKbps(10))
	}
}
