// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

// A device whose link, measured on real video, was too slow for a film starts its next playback on
// this network on the lower quality of the film's ladder its link carries, chosen before play; a
// speed a day old starts at the ladder's top, the film's own picture, as one never measured does
// (D257, D260 §2). Film 1's picture averages 2.4 Mb/s, more than 70% of 2 Mb/s.
func TestPlayback_ADeviceRememberedSlowStartsOnALowerQualityOnTheSameNetwork(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.keepFilmOnesTimeline(t)
	first := harness.play(t, user, chrome(1))
	if first.OnLadder == nil || first.OnLadder.QualityKbps != 2400 {
		t.Errorf("a device never measured starts on %+v; want the film's own picture, 2400k", first.OnLadder)
	}
	measured := reportBody(1, 4_000, 0)
	measured["linkKbps"] = 2000
	if answer := harness.report(t, user, first, measured); answer.status != http.StatusOK {
		t.Fatalf("the report: %d %s", answer.status, answer.body)
	}
	second := harness.play(t, user, chrome(1))
	if second.OnLadder == nil || second.OnLadder.QualityKbps != 1200 ||
		second.OnLadder.URL != Prefix+"/bytes/media/chunks/"+second.MediaToken+"/master.m3u8" {
		t.Errorf("after measuring 2 Mb/s, the next playback starts on %+v; want 1200k, by its own token",
			second.OnLadder)
	}
	if !strings.Contains(harness.logs.String(), `"startsOnLadderKbps":1200`) {
		t.Error("the playback's start does not say it starts lower")
	}
	harness.clock.Advance(25 * time.Hour)
	if stale := harness.play(t, user, chrome(1)); stale.OnLadder == nil || stale.OnLadder.QualityKbps != 2400 {
		t.Errorf("a day after the measurement, a playback starts on %+v; want the film's own picture", stale.OnLadder)
	}
}

// A Watch Together viewer, who has no device row, is remembered by their session on their network,
// and their next playback of the session's film starts lower too (D257).
func TestPlayback_AViewerRememberedSlowStartsOnALowerQuality(t *testing.T) {
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
	credential := membership.Body.MembershipCredential
	playViewer := func() decision {
		t.Helper()
		started := harness.viewerPlayback(t, credential)
		var playing decision
		if err := json.Unmarshal([]byte(started.body), &playing); err != nil || started.status != http.StatusOK {
			t.Fatalf("POST /together/playback = %d %s", started.status, started.body)
		}
		return playing
	}
	first := playViewer()
	measured := reportBody(1, 4_000, 0)
	measured["linkKbps"] = 2000
	if answer := harness.report(t, credential, first, measured); answer.status != http.StatusOK {
		t.Fatalf("the viewer's report: %d %s", answer.status, answer.body)
	}
	if second := playViewer(); second.OnLadder == nil || second.OnLadder.QualityKbps != 1200 {
		t.Errorf("after the viewer measured 2 Mb/s, their next playback starts on %+v; want 1200k",
			second.OnLadder)
	}
}
