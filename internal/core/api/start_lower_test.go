// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/api"
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
		second.OnLadder.URL != api.Prefix+"/bytes/media/chunks/"+second.MediaToken+"/master.m3u8" {
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

// A playback's start tells its player the link its device last measured on this network, within a
// day, which its start rule sets against its first chunk's speed: a Fire TV's first chunk loaded at
// 28 Mb/s on a link that then carried 9.6, and it started with 1 s loaded and ran dry (staging,
// 2026-10-28). A device never measured, and a measurement a day old, are told none (D257 §1).
func TestPlayback_AStartTellsItsPlayerTheLinkItsDeviceLastMeasuredOnTheNetwork(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.keepFilmOnesTimeline(t)
	first := harness.play(t, user, chrome(1))
	if first.OnLadder == nil || first.OnLadder.RememberedLinkKbps != nil {
		t.Fatalf("a device never measured starts on %+v; want no remembered link", first.OnLadder)
	}
	measured := reportBody(1, 4_000, 0)
	measured["linkKbps"] = 9636
	if answer := harness.report(t, user, first, measured); answer.status != http.StatusOK {
		t.Fatalf("the report: %d %s", answer.status, answer.body)
	}
	second := harness.play(t, user, chrome(1))
	if second.OnLadder == nil || second.OnLadder.RememberedLinkKbps == nil ||
		*second.OnLadder.RememberedLinkKbps != 9636 {
		t.Errorf("after measuring 9636 kb/s, the next playback starts on %+v; want it told 9636", second.OnLadder)
	}
	if !strings.Contains(harness.logs.String(), `"rememberedLinkKbps":9636`) {
		t.Error("the playback's start does not say the link it remembered")
	}
	harness.clock.Advance(25 * time.Hour)
	if stale := harness.play(t, user, chrome(1)); stale.OnLadder == nil || stale.OnLadder.RememberedLinkKbps != nil {
		t.Errorf("a day after the measurement, a playback starts on %+v; want no remembered link", stale.OnLadder)
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

// A device's playback starts at the steadiness level its last playback on this network reported,
// on its film's ladder or on the lower quality a stall offers it, relaxed by one for each hour
// since; a level of 0 reported after replaces a steadier one (D352 Q2).
func TestPlayback_ADeviceStartsAtTheSteadinessItsLastPlaybackOnTheNetworkReported(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.shelveAudioStreams(t)
	direct := harness.play(t, user, chrome(1))
	harness.keepFilmOnesTimeline(t)
	unsteady := reportBody(1, 4_000, 0)
	unsteady["steadiness"] = 2
	if answer := harness.report(t, user, direct, unsteady); answer.status != http.StatusOK {
		t.Fatalf("the report: %d %s", answer.status, answer.body)
	}
	if offered := lowerQualityOf(t, harness.report(t, user, direct, stallReport(2, 12_000))); offered == nil ||
		offered.Steadiness != 2 {
		t.Errorf("a stall after reporting steadiness 2 was offered %+v; want it at steadiness 2", offered)
	}
	if next := harness.play(t, user, chrome(1)); next.OnLadder == nil || next.OnLadder.Steadiness != 2 {
		t.Errorf("the next playback starts on %+v; want steadiness 2", next.OnLadder)
	}
	if !strings.Contains(harness.logs.String(), `"startsAtSteadiness":2`) {
		t.Error("the playback's start does not say the steadiness it starts at")
	}
	harness.clock.Advance(time.Hour)
	later := harness.play(t, user, chrome(1))
	if later.OnLadder == nil || later.OnLadder.Steadiness != 1 {
		t.Errorf("an hour on, a playback starts on %+v; want steadiness 1, relaxed by one", later.OnLadder)
	}
	steady := reportBody(1, 4_000, 0)
	steady["steadiness"] = 0
	if answer := harness.report(t, user, later, steady); answer.status != http.StatusOK {
		t.Fatalf("the report: %d %s", answer.status, answer.body)
	}
	if last := harness.play(t, user, chrome(1)); last.OnLadder == nil || last.OnLadder.Steadiness != 0 {
		t.Errorf("after reporting steadiness 0, a playback starts on %+v; want steadiness 0", last.OnLadder)
	}
}

// A playback started on its film's ladder is logged as delivered from the ladder, though its decision
// says the device plays the film as it is, naming the quality it starts on and the sound it starts
// with: here the film's own picture and its own stereo AAC, copied, which the browser plays as it is
// (D368 §1, §5).
func TestPlayback_AChunkedPlayLogsItsLadderTheQualityAndTheSoundItStartsOn(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.shelveAudioStreams(t)
	harness.keepFilmOnesTimeline(t)
	if started := harness.play(t, user, chrome(1)); started.OnLadder == nil || started.Delivery != "direct" {
		t.Fatalf("film 1 starts %+v; want it decided direct and started on its ladder", started)
	}
	harness.logs.AwaitLine(t, `"msg":"a playback started"`, `"fileId":1`, `"delivery":"ladder"`,
		`"decidedDelivery":"direct"`, `"ladderQuality":"picture"`, `"ladderSound":"sound0-own"`)
}
