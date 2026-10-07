// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// stallReport is a report of seq that says the player stalled at positionMs on a link of 2 Mb/s.
func stallReport(seq, positionMs int64) map[string]any {
	body := reportBody(seq, positionMs, 0)
	body["stalls"] = []any{map[string]any{"positionMs": positionMs}}
	body["linkKbps"] = 2000
	return body
}

// lowerQualityOf is the lower quality a report's answer offers, or nil.
func lowerQualityOf(t *testing.T, answer response) *LadderBody {
	t.Helper()
	if answer.status != http.StatusOK {
		t.Fatalf("the report: %d %s", answer.status, answer.body)
	}
	var output ReportOutput
	if err := json.Unmarshal([]byte(answer.body), &output.Body); err != nil {
		t.Fatal(err)
	}
	return output.Body.LowerQuality
}

// A player's first stall starts the quality its link carries, and its sound, where it was, and is
// answered with it, not ready yet; every report after it is answered with it too, so the player
// learns when it is ready (D253 §1, D247 §6). Film 1 averages 2.4 Mb/s, so a link of 2 Mb/s, 1.4
// for the picture, gets 1200k. Its playback started before the film was cut, so not on its ladder
// (D260 §1).
func TestReport_AFirstStallIsAnsweredWithTheLowerQualityItStartedAndSoIsEveryReportAfter(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.shelveAudioStreams(t)
	playing := harness.play(t, user, chrome(1))
	harness.keepFilmOnesTimeline(t)
	if offered := lowerQualityOf(t, harness.report(t, user, playing, reportBody(1, 9_000, 0))); offered != nil {
		t.Errorf("a report before any stall was answered with %+v; want nothing", offered)
	}
	want := LadderBody{
		URL: Prefix + "/bytes/media/chunks/" + playing.MediaToken + "/master.m3u8", StartAtBps: 1_896_000,
		QualityKbps: 1200, Height: 480, IsReady: false, FilmPictureKbps: 2400,
	}
	if offered := lowerQualityOf(t, harness.report(t, user, playing, stallReport(2, 12_000))); offered == nil ||
		*offered != want {
		t.Errorf("the first stall was answered with %+v; want %+v, not ready yet", offered, want)
	}
	if runs := harness.openJobs(t, jobs.KindMakeChunks); len(runs) != 2 {
		t.Errorf("after the first stall, the runs open = %d; want the quality's and the sound's", len(runs))
	}
	if again := lowerQualityOf(t, harness.report(t, user, playing, reportBody(3, 14_000, 0))); again == nil ||
		*again != want {
		t.Errorf("a report after the stall was answered with %+v; want the same offer, %+v", again, want)
	}
	// Each stall, and the offer, is logged with where, the quality and whether it is ready
	// (staging, 2026-10-06).
	logged := harness.logs.String()
	if !strings.Contains(logged, `"msg":"a player reported stalls"`) ||
		!strings.Contains(logged, `"stallPositionsMs":[12000]`) ||
		!strings.Contains(logged, `"msg":"a stalling player was offered a lower quality"`) ||
		!strings.Contains(logged, `"qualityKbps":1200`) || !strings.Contains(logged, `"isReady":false`) {
		t.Errorf("the stall and the offer were not logged as they happened:\n%s", logged)
	}
}

// A Watch Together viewer reports by their membership: nothing is kept of what they watched, and
// their stalls are answered as anyone's (D243 §2, D253 §1), on a playback started before the film
// was cut.
func TestReport_AViewersReportKeepsNothingAndTheirStallsAreAnswered(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	opened := harness.openTogether(t, admin)
	membership := harness.joinByLink(t, admin, opened.Body.SessionID, "my partner", "Anna")
	host := harness.dialTogether(t, opened.Body.MembershipCredential)
	host.awaitWelcome()
	viewer := harness.dialTogether(t, membership.Body.MembershipCredential)
	viewer.awaitWelcome()
	putOn(host, viewer, 1)
	started := harness.viewerPlayback(t, membership.Body.MembershipCredential)
	var playing decision
	if err := json.Unmarshal([]byte(started.body), &playing); err != nil || started.status != http.StatusOK {
		t.Fatalf("POST /together/playback = %d %s", started.status, started.body)
	}
	harness.keepFilmOnesTimeline(t)
	credential := membership.Body.MembershipCredential
	first := stallReport(1, 12_000)
	first["events"] = []any{map[string]any{"kind": "started", "positionMs": 0, "happenedMonoMs": 999_000}}
	if offered := lowerQualityOf(t, harness.report(t, credential, playing, first)); offered == nil ||
		offered.QualityKbps != 1200 {
		t.Errorf("the viewer's first stall was answered with %+v; want 1200k", offered)
	}
	// Nobody's watching is kept: not the host's, whose library it is, nor anyone's.
	for _, userID := range []int64{0, 1} {
		key := sqlcgen.ListPlayEventsOfFileParams{UserID: userID, FileID: 1}
		events, err := harness.database.Read().ListPlayEventsOfFile(t.Context(), key)
		if err != nil || len(events) != 0 {
			t.Errorf("the viewer's reports kept %+v (%v) for user %d; want nothing", events, err, userID)
		}
	}
}

// A report carrying how long the playback took to start is logged, with the link's speed, so a slow
// start shows in the log.
func TestReport_HowLongAPlaybackTookToStartIsLogged(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	playing := harness.play(t, user, chrome(1))
	body := reportBody(1, 4_000, 0)
	body["linkKbps"] = 2500
	body["startup"] = map[string]any{"decisionMs": 180, "firstDataMs": 2400, "playingMs": 3100}
	if answer := harness.report(t, user, playing, body); answer.status != http.StatusOK {
		t.Fatalf("the report: %d %s", answer.status, answer.body)
	}
	logged := harness.logs.String()
	for _, want := range []string{
		`"msg":"a playback started playing"`, `"decisionMs":180`, `"firstDataMs":2400`, `"playingMs":3100`,
		`"linkKbps":2500`,
	} {
		if !strings.Contains(logged, want) {
			t.Errorf("the log has no %s:\n%s", want, logged)
		}
	}
}

// A playback on its film's ladder moves along it by itself: its stalls start nothing and are offered
// nothing (D260 §1).
func TestReport_AStallOnTheLadderIsOfferedNothing(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.shelveAudioStreams(t)
	harness.keepFilmOnesTimeline(t)
	playing := harness.play(t, user, chrome(1))
	if playing.OnLadder == nil {
		t.Fatalf("film 1, cut, started %+v; want it on its ladder", playing)
	}
	started := len(harness.openJobs(t, jobs.KindMakeChunks))
	if offered := lowerQualityOf(t, harness.report(t, user, playing, stallReport(1, 12_000))); offered != nil {
		t.Errorf("a stall on the ladder was answered with %+v; want nothing", offered)
	}
	if runs := len(harness.openJobs(t, jobs.KindMakeChunks)); runs != started {
		t.Errorf("a stall on the ladder opened %d runs; want none", runs-started)
	}
}
