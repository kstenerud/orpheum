// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"net/http"
	"strings"
	"testing"
)

// A start and a seek that waited for enough film loaded ahead before they played say so in their
// lines, with how much was loaded ahead as they played, so a log reader sees the start gate work: a
// Fire TV that played with 1.4 s loaded stalled (the owner's cast, staging, 2026-10-28).
func TestReport_AStartAndASeekSayHowMuchWasLoadedAheadAndThatTheyWaitedForIt(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	playing := harness.play(t, user, chrome(1))
	body := reportBody(1, 1_798_900, 0)
	body["startup"] = map[string]any{
		"decidedAfterMs": 180, "firstDataAfterMs": 2400, "playingAfterMs": 4700,
		"loadedAheadMs": 11_970, "waitedForAheadMs": 3450, "nextChunkLoadMs": 2300,
		"nextChunkLoadFrom": "remembered",
	}
	body["seeks"] = []map[string]any{{
		"fromPositionMs": 2_759_000, "toPositionMs": 1_461_000, "readyAfterMs": 6900,
		"loadedAheadMs": 10_400, "waitedForAheadMs": 10_000,
	}}
	if answer := harness.report(t, user, playing, body); answer.status != http.StatusOK {
		t.Fatalf("the report: %d %s", answer.status, answer.body)
	}
	logged := harness.logs.String()
	for _, want := range []string{
		`"playingAfterMs":4700,"linkKbps":0,"preloadSteps":"","loadedAheadMs":11970,` +
			`"waitedForAheadMs":3450,"nextChunkLoadMs":2300,"nextChunkLoadFrom":"remembered"`,
		`"readyAfterMs":6900,"isPaused":false,"linkKbps":0,"loadedAheadMs":10400,"waitedForAheadMs":10000`,
	} {
		if !strings.Contains(logged, want) {
			t.Errorf("the log has no %s:\n%s", want, logged)
		}
	}
}

// A report whose frames dropped since the last are more than a small share of those shown is logged
// on a line of its own, where the player was, so a stutter shows near when it happened; one with a
// frame or two dropped is not (the owner, 2026-10-28).
func TestReport_DroppedFramesBeyondASmallShareAreLogged(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	playing := harness.play(t, user, chrome(1))
	few := reportBody(1, 10_000, 0)
	few["totalFrames"], few["droppedFrames"] = 240, 2
	if answer := harness.report(t, user, playing, few); answer.status != http.StatusOK {
		t.Fatalf("the report: %d %s", answer.status, answer.body)
	}
	if logged := harness.logs.String(); strings.Contains(logged, "a player dropped frames") {
		t.Errorf("two frames of 240 dropped were logged:\n%s", logged)
	}
	many := reportBody(2, 20_000, 0)
	many["totalFrames"], many["droppedFrames"] = 240, 31
	if answer := harness.report(t, user, playing, many); answer.status != http.StatusOK {
		t.Fatalf("the report: %d %s", answer.status, answer.body)
	}
	want := `"msg":"a player dropped frames"`
	if logged := harness.logs.String(); !strings.Contains(logged, want) ||
		!strings.Contains(logged, `"positionMs":20000,"droppedFrames":31,"totalFrames":240`) {
		t.Errorf("the log has no %s with where and how many:\n%s", want, logged)
	}
}
