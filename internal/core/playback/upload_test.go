// SPDX-License-Identifier: AGPL-3.0-or-later

package playback

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// uploadSettings answers server_upload_mbps, or that it is unanswered, and every other setting with
// its starting value.
type uploadSettings struct{ mbps int64 }

func (answers uploadSettings) Int(_ context.Context, key settings.Key) (int64, error) {
	if key != settings.ServerUploadMbps {
		return settings.StartingInt(key), nil
	}
	if answers.mbps == 0 {
		return 0, settings.ErrUnanswered
	}
	return answers.mbps, nil
}

func (uploadSettings) Duration(_ context.Context, key settings.Key) (time.Duration, error) {
	return settings.StartingDuration(key), nil
}

// uploadService is a service whose players are playbacks, by token, at the fake clock's now.
func uploadService(clock *clocktest.Fake, mbps int64, playbacks map[string]*tokenPlayback) *Service {
	service := &Service{tokens: newTokens(), clock: clock, settings: uploadSettings{mbps: mbps}}
	service.tokens.playbackByToken = playbacks
	return service
}

// A playback of chunks needs the rates of the picture and sound it asked for within askedWithin, one
// that asked nothing lately needs nothing, and a playback of a file as it is needs what it was sent
// (D286 §5).
func TestUpload_IsEachPlaybacksRatesOrWhatItWasSent(t *testing.T) {
	t.Parallel()
	clock := clocktest.NewFake(time.Date(2026, 10, 12, 13, 0, 0, 0, time.UTC))
	now := clock.Now()
	service := uploadService(clock, 0, map[string]*tokenPlayback{
		"chunks": {
			picturesNeed: streamNeed{kbps: 3000, at: now.Add(-5 * time.Second)},
			soundsNeed:   streamNeed{kbps: 96, at: now.Add(-5 * time.Second)},
			sentBytes:    5_000_000,
		},
		"paused": {picturesNeed: streamNeed{kbps: 3000, at: now.Add(-askedWithin - time.Second)}},
		"direct": {sentBytes: 10_000_000},
	})
	want := Upload{Playbacks: 2, NeedKbps: 3096 + 4000, SentKbps: 6000}
	if upload := service.TakeUpload(20 * time.Second); upload != want {
		t.Errorf("the upload = %+v; want %+v", upload, want)
	}
	if upload := service.TakeUpload(20 * time.Second); upload != (Upload{Playbacks: 1, NeedKbps: 3096}) {
		t.Errorf("taken again, nothing sent since = %+v; want the chunks' rates alone", upload)
	}
}

// While anyone plays, each tick logs what the players need beside what was sent and the films'
// share of the upload, and whether the need is over it; with no one playing, nothing.
func TestWatchUpload_LogsTheNeedBesideTheFilmsShareWhileAnyonePlays(t *testing.T) {
	t.Parallel()
	clock := clocktest.NewFake(time.Date(2026, 10, 12, 13, 0, 0, 0, time.UTC))
	idle := uploadService(clock, 10, map[string]*tokenPlayback{})
	playing := uploadService(clock, 10, map[string]*tokenPlayback{
		"chunks": {picturesNeed: streamNeed{kbps: 9000, at: clock.Now()}},
	})
	logs := &testutil.Log{}
	logger := slog.New(slog.NewJSONHandler(logs, nil))
	for _, service := range []*Service{idle, playing} {
		ctx, cancel := context.WithCancel(t.Context())
		tick := make(chan time.Time)
		done := make(chan struct{})
		go func() {
			service.WatchUpload(ctx, tick, logger)
			close(done)
		}()
		clock.Advance(10 * time.Second)
		tick <- clock.Now()
		cancel()
		testutil.Receive(t, done, "the watch to stop")
	}
	logs.AwaitLine(t, `"msg":"the server's upload"`, `"playbacks":1`, `"needKbps":9000`, `"sentKbps":0`,
		`"uploadMbps":10`, `"filmsShareKbps":8500`, `"isOverShare":true`)
	if count := strings.Count(logs.String(), `"msg":"the server's upload"`); count != 1 {
		t.Errorf("the upload was logged %d times; want once, while someone played", count)
	}
}

// A start in a Watch Together group of three takes at most a third of what the playbacks of other
// films leave of the share, whichever of the three asks first; alone, what is left; with the setting
// unanswered, no bound (D286 §5).
func TestStartAtMostKbps_IsAnEvenPartForAGroupsPlayers(t *testing.T) {
	t.Parallel()
	clock := clocktest.NewFake(time.Date(2026, 10, 12, 16, 0, 0, 0, time.UTC))
	other := &tokenPlayback{fileID: 7, picturesNeed: streamNeed{kbps: 1000, at: clock.Now()}}
	service := uploadService(clock, 4, map[string]*tokenPlayback{"other": other}) // a share of 3400
	service.GroupPlayersBy(func(hostUserID int64) int {
		if hostUserID == 1 {
			return 3
		}
		return 0
	})
	host := Holder{UserID: 1}
	guest := Holder{HostUserID: 1}
	for name, holder := range map[string]Holder{"the host": host, "a guest": guest} {
		if got, err := service.startAtMostKbps(t.Context(), holder, 91); err != nil || got != 800 {
			t.Errorf("%s's start may take %d (%v); want a third of the 2400 the other film leaves, 800",
				name, got, err)
		}
	}
	if got, err := service.startAtMostKbps(t.Context(), Holder{UserID: 2}, 91); err != nil || got != 2400 {
		t.Errorf("a start alone may take %d (%v); want what is left, 2400", got, err)
	}
	unanswered := uploadService(clock, 0, map[string]*tokenPlayback{})
	if got, err := unanswered.startAtMostKbps(t.Context(), host, 91); err != nil || got != 0 {
		t.Errorf("with the setting unanswered a start may take %d (%v); want 0, no bound", got, err)
	}
}
