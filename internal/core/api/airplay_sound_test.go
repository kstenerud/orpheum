// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/core/library/librarytest"
)

// shelveFilmOnesTwoSounds gives film 1 two audio tracks, stereo AAC each, as its probe and its tracks
// say: its first, the playback's starting track, and a second in another language.
func (harness testHarness) shelveFilmOnesTwoSounds(t *testing.T) {
	t.Helper()
	sound := func(index int, language string) string {
		librarytest.Stream(t, harness.database, librarytest.StreamSpec{
			BytesFile: 1, StreamIndex: int64(index), Kind: "audio", Codec: "aac", Language: language,
			ChannelCount: 2,
		})
		return fmt.Sprintf(`{"streamIndex":%d,"kind":"audio","codec":"aac","codecProfile":"LC",`+
			`"language":%q,"audio":{"channelCount":2}}`, index, language)
	}
	probe := `{"format":{"formatNames":["mov","mp4","m4a"],"bitRateBps":2400000},"streams":[` +
		`{"kind":"video","codec":"h264","codecProfile":"High","video":{"pixelFormat":"yuv420p"}},` +
		sound(1, "en") + "," + sound(2, "ja") + "]}"
	librarytest.Probe(t, harness.database, librarytest.ProbeSpec{FileID: 1, ProbeJSON: probe})
}

// grantBody is a grant's request: the playback's token, and the audio track the page plays, by its
// place, where track is not nil.
func grantBody(mediaToken string, track *int) map[string]any {
	body := map[string]any{"mediaToken": mediaToken}
	if track != nil {
		body["audioTrackIndex"] = *track
	}
	return body
}

// grantMaster is the master playlist's path a 200 from a grant's POST answered.
func grantMaster(t *testing.T, answer response) string {
	t.Helper()
	var made airPlayGrant
	if err := json.Unmarshal([]byte(answer.body), &made); err != nil || answer.status != http.StatusOK {
		t.Fatalf("the grant's POST = %d %s", answer.status, answer.body)
	}
	return made.MasterPath
}

// checkDefaultTrack fails the test unless every sound a master playlist names its default, in each of
// its audio groups, is one of the audio track track's, and no other track's is.
func checkDefaultTrack(t *testing.T, master byteAnswer, track, why string) {
	t.Helper()
	defaults := 0
	for line := range strings.SplitSeq(master.body, "\n") {
		if !strings.HasPrefix(line, "#EXT-X-MEDIA:") || !strings.Contains(line, "DEFAULT=YES") {
			continue
		}
		defaults++
		if !strings.Contains(line, `URI="sound`+track+"-") {
			t.Errorf("%s: a default sound of another track than %s: %s\n%s", why, track, line, master.body)
		}
	}
	if master.status != http.StatusOK || defaults == 0 {
		t.Errorf("%s: the master playlist = %d, %d defaults\n%s", why, master.status, defaults, master.body)
	}
}

// An AirPlay grant's master playlist names its default the sound of the audio track the page played
// as it asked for the grant, in place of the playback's starting track; with no track named, or one
// the film does not have, the starting track's, as before; and the page's own master is unchanged
// (D521 §2: the owner's Mac played another track than the one chosen on the phone, 2026-11-18).
func TestAirPlay_AGrantsMasterDefaultsToTheTrackThePagePlays(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.shelveFilmOnesTwoSounds(t)
	harness.keepFilmOnesTimeline(t)
	token := harness.play(t, user, chrome(1)).MediaToken
	ask := func(track *int) byteAnswer {
		path := grantMaster(t, harness.do(t, http.MethodPost, "/airplay/grants", user, grantBody(token, track)))
		return harness.getAsAirPlayReceiver(t, path)
	}
	second, beyond := 1, 7
	checkDefaultTrack(t, ask(&second), "1", "a grant asked with the second track")
	checkDefaultTrack(t, ask(nil), "0", "a grant asked with no track")
	checkDefaultTrack(t, ask(&beyond), "0", "a grant asked with a track the film does not have")
	own := harness.getBytePath(t, api.Prefix+"/bytes/media/chunks/"+token+"/master.m3u8", user, true, "")
	checkDefaultTrack(t, own, "0", "the page's own master, after the grants'")
}

// A Watch Together guest by link's AirPlay grant names its default the track their page plays too,
// with none named the starting track's (D521 §2: it was a guest who chose the other sound).
func TestTogetherAirPlay_AGuestsGrantsMasterDefaultsToTheTrackTheirPagePlays(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.shelveFilmOnesTwoSounds(t)
	harness.keepFilmOnesTimeline(t)
	opened := harness.openTogether(t, admin)
	membership := harness.joinByLink(t, admin, opened.Body.SessionID, "a friend", "Sam")
	credential := membership.Body.MembershipCredential
	host := harness.dialTogether(t, opened.Body.MembershipCredential)
	host.awaitWelcome()
	viewer := harness.dialTogether(t, credential)
	viewer.awaitWelcome()
	putOn(host, viewer, 1)
	started := harness.viewerPlayback(t, credential)
	var playing decision
	if err := json.Unmarshal([]byte(started.body), &playing); err != nil || started.status != http.StatusOK {
		t.Fatalf("POST /together/playback = %d %s", started.status, started.body)
	}
	ask := func(track *int) byteAnswer {
		answer := harness.do(t, http.MethodPost, "/together/airplay-grants", credential,
			grantBody(playing.MediaToken, track))
		return harness.getAsAirPlayReceiver(t, grantMaster(t, answer))
	}
	second := 1
	checkDefaultTrack(t, ask(&second), "1", "the guest's grant asked with the second track")
	checkDefaultTrack(t, ask(nil), "0", "the guest's grant asked with no track")
}
