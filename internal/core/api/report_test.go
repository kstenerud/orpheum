// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// report sends a play report on a playback, by the media token its decision named.
func (harness testHarness) report(t *testing.T, session string, decided decision, body map[string]any) response {
	t.Helper()
	if decided.MediaToken == "" || !strings.Contains(decided.URL, decided.MediaToken) {
		t.Fatalf("the decision names media token %q, and its URL is %q", decided.MediaToken, decided.URL)
	}
	return harness.do(t, http.MethodPost, "/playback/"+decided.MediaToken+"/report", session, body)
}

// at is a report of the position positionMs, read sampledBefore milliseconds before it was sent.
func at(seq, positionMs, sampledBefore int64) map[string]any {
	const sentMonoMs = 1_000_000
	return map[string]any{
		"reportSeq": seq, "sentMonoMs": sentMonoMs, "positionMs": positionMs,
		"positionReadMonoMs": sentMonoMs - sampledBefore,
		"events":             []any{}, "spans": []any{},
	}
}

func TestPlayState_TwoDevicesReportingOutOfOrderNeverRewindThePosition(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	tablet := harness.createAdminAndSignIn(t)
	television := harness.signIn(t, "owner", "owner-password")
	harness.shelveFilms(t)
	onTablet, onTelevision := harness.play(t, tablet, chrome(1)), harness.play(t, television, chrome(1))

	if sent := harness.report(t, tablet, onTablet, at(1, 600_000, 0)); sent.status != http.StatusNoContent {
		t.Fatalf("the tablet's report: %d %s", sent.status, sent.body)
	}
	harness.clock.Advance(time.Minute)
	// The television read 5:00 sixty-five seconds ago, before the tablet read 10:00, and only
	// now gets it through: a report about an earlier moment of watching, arriving later.
	harness.report(t, television, onTelevision, at(1, 300_000, 65_000))
	if resumed := harness.play(t, tablet, chrome(1)); resumed.ResumePositionMs != 600_000 {
		t.Errorf("after an older report arrived late the position is %d, want 600000: never rewound",
			resumed.ResumePositionMs)
	}
	harness.report(t, television, onTelevision, at(2, 120_000, 0))
	if moved := harness.play(t, tablet, chrome(1)); moved.ResumePositionMs != 120_000 {
		t.Errorf("after a later moment of watching the position is %d, want 120000: the latest, not the furthest",
			moved.ResumePositionMs)
	}
	harness.report(t, television, onTelevision, at(2, 900_000, 0))
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
	body := at(1, 90_000, 0)
	body["events"] = []any{
		map[string]any{"kind": "started", "positionMs": 0, "atMonoMs": 910_000},
		map[string]any{"kind": "paused", "positionMs": 90_000, "atMonoMs": 1_000_000},
	}
	body["spans"] = []any{map[string]any{
		"fromPositionMs": 0, "toPositionMs": 90_000, "startedMonoMs": 910_000, "endedMonoMs": 1_000_000,
	}}
	if sent := harness.report(t, user, decided, body); sent.status != http.StatusNoContent {
		t.Fatalf("the report: %d %s", sent.status, sent.body)
	}
	now := harness.clock.Now().UnixMilli()
	key := sqlcgen.ListPlaySpansOfFileParams{UserID: 1, FileID: 1}
	spans, err := harness.library.Read().ListPlaySpansOfFile(t.Context(), key)
	if err != nil {
		t.Fatal(err)
	}
	if len(spans) != 1 || spans[0].StartedAt != now-90_000 || spans[0].EndedAt != now ||
		spans[0].ToPositionMs != 90_000 || spans[0].DeviceID == nil {
		t.Errorf("spans = %+v, want 0:00 to 1:30 watched in the ninety seconds before now, on the device", spans)
	}
	events, err := harness.library.Read().ListPlayEventsOfFile(t.Context(), sqlcgen.ListPlayEventsOfFileParams(key))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].Kind != "started" || events[0].HappenedAt != now-90_000 ||
		events[1].Kind != "paused" {
		t.Errorf("events = %+v, want started then paused, at the server's time of each", events)
	}
	later := at(2, 95_000, 0)
	later["events"] = []any{map[string]any{"kind": "seeked", "positionMs": 0, "atMonoMs": 1_000_001}}
	if bad := harness.report(t, user, decided, later); bad.status != http.StatusBadRequest {
		t.Errorf("a change after the report was sent: %d, want refused", bad.status)
	}
}

func TestPlayState_APositionResumesOnlyBetweenTheThresholdsOfAKnownLength(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	storetest.ExecRaw(t, harness.database,
		`UPDATE file SET probe_json = json_set(probe_json, '$.format.durationMs', 1000000) WHERE id = 1`)
	decided := harness.play(t, user, chrome(1))
	for seq, check := range []struct{ positionMs, want int64 }{
		{30_000, 0},        // 3%: before resume_min_percent, it plays from the start
		{500_000, 500_000}, // halfway
		{950_000, 0},       // 95%: finished, it plays from the start
	} {
		harness.report(t, user, decided, at(int64(seq+1), check.positionMs, 0))
		harness.clock.Advance(time.Second)
		if got := harness.play(t, user, chrome(1)).ResumePositionMs; got != check.want {
			t.Errorf("left at %d ms of 1000000: resumes at %d, want %d", check.positionMs, got, check.want)
		}
	}
	// A file whose length is unknown resumes where it was left: nothing becomes "finished".
	unknown := harness.play(t, user, chrome(2))
	harness.report(t, user, unknown, at(1, 950_000, 0))
	if got := harness.play(t, user, chrome(2)).ResumePositionMs; got != 950_000 {
		t.Errorf("a file of unknown length resumes at %d, want 950000", got)
	}
}
