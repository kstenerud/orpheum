// SPDX-License-Identifier: AGPL-3.0-or-later

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
	body["stalls"] = []any{map[string]any{
		"positionMs": positionMs, "readyState": 1, "isPaused": true,
		"videoBuffered": "306.80-317.06", "audioBuffered": "296.40-317.23", "lastError": "bufferStalledError: stalled",
		"steps": "level 2; frag 2/31 loading; frag 2/31 aborted",
	}}
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
	for _, want := range []string{
		`"msg":"a player reported stalls"`, `"stallPositionsMs":[12000]`,
		`"stallReadyStates":[1]`, `"stallsPaused":[true]`, `"stallVideoBuffered":["306.80-317.06"]`,
		`"stallAudioBuffered":["296.40-317.23"]`, `"stallLastErrors":["bufferStalledError: stalled"]`,
		`"stallSteps":["level 2; frag 2/31 loading; frag 2/31 aborted"]`,
		`"msg":"a stalling player was offered a lower quality"`, `"qualityKbps":1200`, `"isReady":false`,
	} {
		if !strings.Contains(logged, want) {
			t.Errorf("the stall and the offer were not logged as they happened, %s missing:\n%s", want, logged)
		}
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
	body["startup"] = map[string]any{
		"decidedAfterMs": 180, "firstDataAfterMs": 2400, "playingAfterMs": 3100,
		"preloadSteps": "level 4; frag 4/0 loaded; level 0",
	}
	if answer := harness.report(t, user, playing, body); answer.status != http.StatusOK {
		t.Fatalf("the report: %d %s", answer.status, answer.body)
	}
	logged := harness.logs.String()
	for _, want := range []string{
		`"msg":"a playback started playing"`, `"decidedAfterMs":180`, `"firstDataAfterMs":2400`,
		`"playingAfterMs":3100`, `"linkKbps":2500`, `"preloadSteps":"level 4; frag 4/0 loaded; level 0"`,
	} {
		if !strings.Contains(logged, want) {
			t.Errorf("the log has no %s:\n%s", want, logged)
		}
	}
}

// A report carrying how long the person's seeks took to play again is logged, each with where it
// went, the link's speed and the player's last steps, its quality's choices with why, so a jump that
// felt slow is measured (the owner, 2026-10-12) and one that dropped to 360p explained (2026-10-20).
func TestReport_HowLongASeekTookToPlayAgainIsLogged(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	playing := harness.play(t, user, chrome(1))
	body := reportBody(1, 4_000, 0)
	body["linkKbps"] = 10_400
	body["seeks"] = []map[string]any{{
		"fromPositionMs": 2_759_000, "toPositionMs": 1_461_000, "readyAfterMs": 6900,
		"pictureAfterMs": 1300, "soundAfterMs": 4900,
		"steps": "ceiling 4 to 0: link 900 kb/s, 0.0 s ahead; level 0",
	}}
	if answer := harness.report(t, user, playing, body); answer.status != http.StatusOK {
		t.Fatalf("the report: %d %s", answer.status, answer.body)
	}
	logged := harness.logs.String()
	for _, want := range []string{
		`"msg":"a player's seek was ready to play"`, `"fromPositionMs":2759000`, `"toPositionMs":1461000`,
		`"readyAfterMs":6900`, `"isPaused":false`, `"linkKbps":10400`, `"pictureAfterMs":1300`,
		`"soundAfterMs":4900`, `"steps":"ceiling 4 to 0: link 900 kb/s, 0.0 s ahead; level 0"`,
	} {
		if !strings.Contains(logged, want) {
			t.Errorf("the log has no %s:\n%s", want, logged)
		}
	}
}

// A report carrying where the player, asked to play, kept its position is logged with what its video
// showed: a phone's Brave stood at 0:00 "playing", and the log said only that it started (the owner's
// friend, 2026-10-12).
func TestReport_APlayerStandingStillIsLoggedWithWhatItsVideoShowed(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	playing := harness.play(t, user, chrome(1))
	body := reportBody(1, 0, 0)
	body["standstills"] = []map[string]any{{
		"positionMs": 0, "forMs": 5000, "readyState": 4, "networkState": 1, "buffered": "0.00-31.20",
		"lastError": "", "videoWidth": 0, "videoHeight": 0, "decodedFrames": 0, "droppedFrames": 0,
		"isMuted": false, "volume": 1, "isPageHidden": false,
	}}
	if answer := harness.report(t, user, playing, body); answer.status != http.StatusOK {
		t.Fatalf("the report: %d %s", answer.status, answer.body)
	}
	logged := harness.logs.String()
	for _, want := range []string{
		`"msg":"a playing player's position stood still"`, `"forMs":5000`, `"readyState":4`,
		`"networkState":1`, `"buffered":"0.00-31.20"`, `"videoWidth":0`, `"decodedFrames":0`,
		`"isPageHidden":false`,
	} {
		if !strings.Contains(logged, want) {
			t.Errorf("the log has no %s:\n%s", want, logged)
		}
	}
}

// Each change of a player's play state is logged with where, once, a report sent again logging
// nothing more: a Mac playing alone paused and closed, and the log said only that it had started
// (the owner, 2026-10-12).
func TestReport_EachChangeOfAPlayersPlayStateIsLoggedOnce(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	playing := harness.play(t, user, chrome(1))
	body := reportBody(1, 4_000, 0)
	body["events"] = []any{
		map[string]any{"kind": "started", "positionMs": 4_000, "happenedMonoMs": 980_000},
		map[string]any{"kind": "paused", "positionMs": 9_000, "happenedMonoMs": 990_000},
		map[string]any{"kind": "abandoned", "positionMs": 9_000, "happenedMonoMs": 995_000},
	}
	for range 2 { // the second, a report sent again
		if answer := harness.report(t, user, playing, body); answer.status != http.StatusOK {
			t.Fatalf("the report: %d %s", answer.status, answer.body)
		}
	}
	logged := harness.logs.String()
	for _, want := range []string{
		`"kind":"started","positionMs":4000`, `"kind":"paused","positionMs":9000`,
		`"kind":"abandoned","positionMs":9000`,
	} {
		if count := strings.Count(logged, want); count != 1 {
			t.Errorf("the log has %s %d times; want once:\n%s", want, count, logged)
		}
	}
	if !strings.Contains(logged, `"msg":"a player's play state changed"`) {
		t.Errorf("the log names no change of play state:\n%s", logged)
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

// Each quality a player gave up as one its device cannot play is logged with where, so a film that
// freezes on one device says why (D266).
func TestReport_EachQualityAPlayerCannotPlayIsLogged(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	playing := harness.play(t, user, chrome(1))
	body := reportBody(1, 420_500, 0)
	body["unplayables"] = []map[string]any{{
		"qualityKbps": 36_208, "positionMs": 420_500, "readyState": 1, "isPaused": true,
		"videoBuffered": "416.29-499.62", "audioBuffered": "425.70-499.80", "lastError": "",
	}, {"qualityKbps": 20_104, "positionMs": 421_000}}
	if answer := harness.report(t, user, playing, body); answer.status != http.StatusOK {
		t.Fatalf("the report: %d %s", answer.status, answer.body)
	}
	logged := harness.logs.String()
	if !strings.Contains(logged, `"msg":"a player found a quality its device cannot play"`) ||
		!strings.Contains(logged, `"qualityKbps":36208`) || !strings.Contains(logged, `"positionMs":420500`) ||
		!strings.Contains(logged, `"readyState":1`) || !strings.Contains(logged, `"isPaused":true`) ||
		!strings.Contains(logged, `"videoBuffered":"416.29-499.62"`) ||
		!strings.Contains(logged, `"audioBuffered":"425.70-499.80"`) {
		t.Errorf("the first unplayable quality was not logged:\n%s", logged)
	}
	if !strings.Contains(logged, `"qualityKbps":20104`) {
		t.Errorf("the second unplayable quality was not logged:\n%s", logged)
	}
}

// A pause or a jump says where it came from, and each path the page moved to is logged beside it:
// two players' 0.2 s pauses and a 0.25 s jump forward were heard as hiccups, and the log said nothing
// of why (the owner, 2026-10-17).
func TestReport_APausesContextAndThePagesPathMovesAreLogged(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	playing := harness.play(t, user, chrome(1))
	body := reportBody(1, 4_000, 0)
	body["events"] = []any{
		map[string]any{"kind": "paused", "positionMs": 9_000, "happenedMonoMs": 990_000, "context": map[string]any{
			"isPageHidden": true, "mediaSessionAction": "pause", "readyState": 4, "audioBuffered": "8.000-12.000",
		}},
		map[string]any{"kind": "seeked", "positionMs": 9_251, "happenedMonoMs": 991_000, "context": map[string]any{
			"skippedBy": "hole",
		}},
	}
	body["pathMoves"] = []any{map[string]any{"to": "WebRTC", "happenedMonoMs": 989_500}}
	if answer := harness.report(t, user, playing, body); answer.status != http.StatusOK {
		t.Fatalf("the report: %d %s", answer.status, answer.body)
	}
	logged := harness.logs.String()
	for _, want := range []string{
		`"kind":"paused","positionMs":9000,"isPageHidden":true`, `"mediaSessionAction":"pause"`,
		`"audioBuffered":"8.000-12.000"`, `"kind":"seeked","positionMs":9251`, `"skippedBy":"hole"`,
		`"msg":"a player's page moved to another path to the core"`, `"to":"WebRTC","happenedMonoMs":989500`,
	} {
		if !strings.Contains(logged, want) {
			t.Errorf("the log has no %s:\n%s", want, logged)
		}
	}
}
