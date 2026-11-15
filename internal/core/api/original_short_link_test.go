// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"net/http"
	"testing"
	"time"
)

// originalChosen is a start-playback body for film 1 asking for its own file, as the Play page asks
// with cause, Chrome's formats with it.
func originalChosen(cause string) map[string]any {
	body := chrome(1)
	body["preferredDelivery"], body["cause"] = "direct", cause
	return body
}

// The original chosen on a link its device last measured short of the file's rate starts on the
// highest rung of the film's ladder that link carries, as a film's own start does, and says the
// original waits for the link; asked for again once the link carries it, it plays the file as it is.
// On a link not measured, a measurement a day old, the original plays at once, as it did before
// (the owner, 2026-11-15: "Better to start on a lower rung and then go back up later", "When we
// haven't measured, try the original first"). Film 1's picture averages 2.4 Mb/s, more than 70% of
// 2 Mb/s and within 70% of 9.6.
func TestPlayback_TheOriginalChosenOnALinkShortOfItStartsOnTheLadderAndWaitsForTheLink(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.keepFilmOnesTimeline(t)
	first := harness.play(t, user, chrome(1))
	measured := reportBody(1, 4_000, 0)
	measured["linkKbps"] = 2000
	if answer := harness.report(t, user, first, measured); answer.status != http.StatusOK {
		t.Fatalf("the report: %d %s", answer.status, answer.body)
	}
	short := harness.play(t, user, originalChosen("originalChosen"))
	if short.OnLadder == nil || short.OnLadder.QualityKbps != 1200 || !short.IsOriginalWaitingForTheLink {
		t.Errorf("the original chosen after measuring 2 Mb/s started on %+v, waiting %v; want 1200k, waiting "+
			"for the link", short.OnLadder, short.IsOriginalWaitingForTheLink)
	}
	harness.logs.AwaitEvent(t, "playbackStarted", `"cause":"originalChosen"`, `"delivery":"ladder"`,
		`"isOriginalWaitingForTheLink":true`)
	carried := reportBody(1, 34_000, 0)
	carried["linkKbps"] = 9636
	if answer := harness.report(t, user, short, carried); answer.status != http.StatusOK {
		t.Fatalf("the report: %d %s", answer.status, answer.body)
	}
	moved := harness.play(t, user, originalChosen("linkCarriesOriginal"))
	if moved.Delivery != "direct" || moved.OnLadder != nil || moved.IsOriginalWaitingForTheLink {
		t.Errorf("the original asked for once the link carried it started %+v on %+v; want the file as it is",
			moved, moved.OnLadder)
	}
	harness.clock.Advance(25 * time.Hour)
	stale := harness.play(t, user, originalChosen("originalChosen"))
	if stale.Delivery != "direct" || stale.OnLadder != nil || stale.IsOriginalWaitingForTheLink {
		t.Errorf("the original chosen a day after the link was measured started on %+v; want the file as it is",
			stale.OnLadder)
	}
}
