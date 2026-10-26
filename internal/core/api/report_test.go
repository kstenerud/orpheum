// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// report sends a play report on a playback, by the media token its decision named.
func (harness testHarness) report(t *testing.T, sessionToken string, decided decision, body map[string]any) response {
	t.Helper()
	if decided.MediaToken == "" || !strings.Contains(decided.URL, decided.MediaToken) {
		t.Fatalf("the decision names media token %q, and its URL is %q", decided.MediaToken, decided.URL)
	}
	return harness.do(t, http.MethodPost, "/playback/"+decided.MediaToken+"/report", sessionToken, body)
}

// reportBody is a report of the position positionMs, read sampledBeforeMs before it was sent, by a
// player that has played: its first report says it started, as a player's does once it plays.
func reportBody(seq, positionMs, sampledBeforeMs int64) map[string]any {
	const sentMonoMs = 1_000_000
	events := []any{}
	if seq == 1 {
		events = append(events, map[string]any{
			"kind": "started", "positionMs": positionMs, "happenedMonoMs": sentMonoMs - sampledBeforeMs,
		})
	}
	return map[string]any{
		"reportSeq": seq, "sentMonoMs": sentMonoMs, "positionMs": positionMs,
		"positionReadMonoMs": sentMonoMs - sampledBeforeMs,
		"events":             events, "spans": []any{},
	}
}

func TestPlayState_TwoDevicesReportingOutOfOrderNeverRewindThePosition(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	tablet := harness.createAdminAndSignIn(t)
	television := harness.signIn(t, "owner", "owner-password")
	harness.shelveFilms(t)
	onTablet, onTelevision := harness.play(t, tablet, chrome(1)), harness.play(t, television, chrome(1))

	if sent := harness.report(t, tablet, onTablet, reportBody(1, 600_000, 0)); sent.status != http.StatusOK {
		t.Fatalf("the tablet's report: %d %s", sent.status, sent.body)
	}
	harness.clock.Advance(time.Minute)
	// The television read 5:00 sixty-five seconds ago, before the tablet read 10:00, and only
	// now gets it through: a report about an earlier moment of watching, arriving later.
	harness.report(t, television, onTelevision, reportBody(1, 300_000, 65_000))
	if resumed := harness.play(t, tablet, chrome(1)); resumed.ResumePositionMs != 600_000 {
		t.Errorf("after an older report arrived late the position is %d, want 600000: never rewound",
			resumed.ResumePositionMs)
	}
	harness.report(t, television, onTelevision, reportBody(2, 120_000, 0))
	if moved := harness.play(t, tablet, chrome(1)); moved.ResumePositionMs != 120_000 {
		t.Errorf("after a later moment of watching the position is %d, want 120000: the latest, not the furthest",
			moved.ResumePositionMs)
	}
	harness.report(t, television, onTelevision, reportBody(2, 900_000, 0))
	if repeated := harness.play(t, tablet, chrome(1)); repeated.ResumePositionMs != 120_000 {
		t.Errorf("a report sent again moved the position to %d", repeated.ResumePositionMs)
	}
}

func TestPlayState_ChangesAndSpansAreLoggedAtTheServersTimeOfTheWatching(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	decided := harness.play(t, user, chrome(1))
	body := reportBody(1, 90_000, 0)
	body["events"] = []any{
		map[string]any{"kind": "started", "positionMs": 0, "happenedMonoMs": 910_000},
		map[string]any{"kind": "paused", "positionMs": 90_000, "happenedMonoMs": 1_000_000},
	}
	body["spans"] = []any{map[string]any{
		"fromPositionMs": 0, "toPositionMs": 90_000, "startedMonoMs": 910_000, "endedMonoMs": 1_000_000,
	}}
	if sent := harness.report(t, user, decided, body); sent.status != http.StatusOK {
		t.Fatalf("the report: %d %s", sent.status, sent.body)
	}
	now := harness.clock.Now().UnixMilli()
	key := sqlcgen.ListPlaySpansOfFileParams{UserID: 1, FileID: 1}
	spans, err := harness.database.Read().ListPlaySpansOfFile(t.Context(), key)
	if err != nil {
		t.Fatal(err)
	}
	if len(spans) != 1 || spans[0].StartedAt != now-90_000 || spans[0].EndedAt != now ||
		spans[0].ToPositionMs != 90_000 || spans[0].DeviceID == nil {
		t.Errorf("spans = %+v, want 0:00 to 1:30 watched in the ninety seconds before now, on the device", spans)
	}
	events, err := harness.database.Read().ListPlayEventsOfFile(t.Context(), sqlcgen.ListPlayEventsOfFileParams(key))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].Kind != "started" || events[0].HappenedAt != now-90_000 ||
		events[1].Kind != "paused" {
		t.Errorf("events = %+v, want started then paused, at the server's time of each", events)
	}
	later := reportBody(2, 95_000, 0)
	later["events"] = []any{map[string]any{"kind": "seeked", "positionMs": 0, "happenedMonoMs": 1_000_001}}
	if bad := harness.report(t, user, decided, later); bad.status != http.StatusBadRequest {
		t.Errorf("a change after the report was sent: %d, want refused", bad.status)
	}
}

func TestPlayState_APositionResumesOnlyBetweenTheThresholdsOfAKnownLength(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	storetest.ExecRaw(t, harness.databasePath,
		`UPDATE file_probe SET probe_json = json_set(probe_json, '$.format.durationMs', 1000000),`+
			` duration_ms = 1000000 WHERE file_id = 1`)
	decided := harness.play(t, user, chrome(1))
	for seq, check := range []struct{ positionMs, want int64 }{
		{30_000, 0},        // 3%: before in_progress_after_percent, it plays from the start
		{500_000, 500_000}, // halfway
		{950_000, 0},       // 95%: finished, it plays from the start
	} {
		harness.report(t, user, decided, reportBody(int64(seq+1), check.positionMs, 0))
		harness.clock.Advance(time.Second)
		if got := harness.play(t, user, chrome(1)).ResumePositionMs; got != check.want {
			t.Errorf("left at %d ms of 1000000: resumes at %d, want %d", check.positionMs, got, check.want)
		}
	}
	// A file whose length is unknown resumes where it was left: nothing becomes "finished".
	unknown := harness.play(t, user, chrome(2))
	harness.report(t, user, unknown, reportBody(1, 950_000, 0))
	if got := harness.play(t, user, chrome(2)).ResumePositionMs; got != 950_000 {
		t.Errorf("a file of unknown length resumes at %d, want 950000", got)
	}
}

// A playback that has not played moves no one's position: its player, its video not yet loaded,
// reports 0, and a host whose session's film never loaded found their place in Conan gone (the
// owner, 2026-10-08). Once it says it started, it moves it.
func TestPlayState_APlaybackThatHasNotPlayedMovesNoPosition(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	watched := harness.play(t, user, chrome(2))
	harness.report(t, user, watched, reportBody(1, 600_000, 0))
	harness.clock.Advance(time.Second)
	stuck := harness.play(t, user, chrome(2))
	notStarted := reportBody(2, 0, 0) // its first report, but of a player that has not played
	notStarted["reportSeq"] = 1
	if sent := harness.report(t, user, stuck, notStarted); sent.status != http.StatusOK {
		t.Fatalf("the stuck player's report: %d %s", sent.status, sent.body)
	}
	harness.clock.Advance(time.Second)
	if got := harness.play(t, user, chrome(2)).ResumePositionMs; got != 600_000 {
		t.Errorf("after a player that never played reported 0:00 the position is %d, want 600000", got)
	}
}

func TestPlayState_ARemovedPersonsWatchingGoesWithThemAndOthersStays(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.do(t, http.MethodPost, "/users", admin,
		map[string]string{"name": "partner", "password": "partner-password", "role": "admin"})
	partner := harness.signIn(t, "partner", "partner-password")
	harness.shelveFilms(t)
	body := reportBody(1, 90_000, 0)
	body["spans"] = []any{map[string]any{
		"fromPositionMs": 0, "toPositionMs": 90_000, "startedMonoMs": 910_000, "endedMonoMs": 1_000_000,
	}}
	for _, token := range []string{admin, partner} {
		if sent := harness.report(t, token, harness.play(t, token, chrome(1)), body); sent.status != http.StatusOK {
			t.Fatalf("a report: %d %s", sent.status, sent.body)
		}
	}
	if removed := harness.do(t, http.MethodDelete, "/users/2", admin, nil); removed.status != http.StatusNoContent {
		t.Fatalf("DELETE /users/2 = %d %s", removed.status, removed.body)
	}
	for userID, want := range map[int64]int{1: 1, 2: 0} {
		key := sqlcgen.ListPlaySpansOfFileParams{UserID: userID, FileID: 1}
		spans, err := harness.database.Read().ListPlaySpansOfFile(t.Context(), key)
		if err != nil {
			t.Fatal(err)
		}
		eventsKey := sqlcgen.ListPlayEventsOfFileParams(key)
		events, err := harness.database.Read().ListPlayEventsOfFile(t.Context(), eventsKey)
		if err != nil {
			t.Fatal(err)
		}
		if len(spans) != want || len(events) != want {
			t.Errorf("user %d after removing user 2: %d spans and %d events, want %d of each (D173 §4.6)",
				userID, len(spans), len(events), want)
		}
	}
	if resumed := harness.play(t, admin, chrome(1)); resumed.ResumePositionMs != 90_000 {
		t.Errorf("the admin's place after removing someone else: %d, want 90000", resumed.ResumePositionMs)
	}
}

// A playback the server does not have, as after it restarted, is said to be gone, not removed, so
// the player can say the server restarted and offer to carry on.
func TestPlayState_AReportOnAPlaybackTheServerDoesNotHaveSaysItIsGone(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	gone := harness.do(t, http.MethodPost, "/playback/never-issued-token/report", user, reportBody(1, 60_000, 0))
	wantError(t, gone, http.StatusNotFound, api.CodePlaybackGone)
}
