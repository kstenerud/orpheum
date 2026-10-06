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
func lowerQualityOf(t *testing.T, answer response) *LowerQualityBody {
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

// A player's first stall starts the quality its link carries, and its sound, where it was; its
// second is answered with that quality, once (D247 §2, §6). Film 1 averages 2.4 Mb/s, so a link of
// 2 Mb/s, 1.4 for the picture, gets 1200k.
func TestReport_ASecondStallIsAnsweredWithTheLowerQualityTheFirstStarted(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.shelveAudioStreams(t)
	harness.keepFilmOnesTimeline(t)
	playing := harness.play(t, user, chrome(1))
	if offered := lowerQualityOf(t, harness.report(t, user, playing, stallReport(1, 12_000))); offered != nil {
		t.Errorf("a first stall was answered with %+v; want nothing yet, the quality only started", offered)
	}
	if runs := harness.openJobs(t, jobs.KindMakeChunks); len(runs) != 2 {
		t.Errorf("after the first stall, the runs open = %d; want the quality's and the sound's", len(runs))
	}
	offered := lowerQualityOf(t, harness.report(t, user, playing, stallReport(2, 15_000)))
	want := LowerQualityBody{
		URL: Prefix + "/bytes/media/chunks/" + playing.MediaToken + "/master.m3u8", StartAtBps: 1_992_000,
		QualityKbps: 1200, Height: 480,
	}
	if offered == nil || *offered != want {
		t.Errorf("the second stall was answered with %+v; want %+v", offered, want)
	}
	if again := lowerQualityOf(t, harness.report(t, user, playing, stallReport(3, 18_000))); again != nil {
		t.Errorf("a third stall was answered with %+v; want nothing: the player was told", again)
	}
	// Each stall, and the offer, is logged with where and the quality (staging, 2026-10-06).
	logged := harness.logs.String()
	if !strings.Contains(logged, `"msg":"a player reported stalls"`) ||
		!strings.Contains(logged, `"stallPositionsMs":[15000]`) ||
		!strings.Contains(logged, `"msg":"a stalling player was offered a lower quality"`) ||
		!strings.Contains(logged, `"qualityKbps":1200`) {
		t.Errorf("the stalls and the offer were not logged as they happened:\n%s", logged)
	}
}

// A Watch Together viewer reports by their membership: nothing is kept of what they watched, and
// their stalls are answered as anyone's (D243 §2, D247 §2).
func TestReport_AViewersReportKeepsNothingAndTheirStallsAreAnswered(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.keepFilmOnesTimeline(t)
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
	credential := membership.Body.MembershipCredential
	first := stallReport(1, 12_000)
	first["events"] = []any{map[string]any{"kind": "started", "positionMs": 0, "happenedMonoMs": 999_000}}
	lowerQualityOf(t, harness.report(t, credential, playing, first))
	if offered := lowerQualityOf(t, harness.report(t, credential, playing, stallReport(2, 15_000))); offered ==
		nil || offered.QualityKbps != 1200 {
		t.Errorf("the viewer's second stall was answered with %+v; want 1200k", offered)
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
