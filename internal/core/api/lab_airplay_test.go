// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
)

// labAirPlayGrant is what POST /lab/airplay/grants answers.
type labAirPlayGrant struct {
	MasterPath string    `json:"masterPath"`
	ExpiresAt  time.Time `json:"expiresAt"`
}

// offerLabAirPlay has the admin turn the AirPlay lab on, as staging's does.
func (harness testHarness) offerLabAirPlay(t *testing.T, isOffered string) {
	t.Helper()
	settingstest.Set(t, harness.server.Dependencies().Load().Settings, settings.ShouldOfferAirPlayLab, isOffered)
}

// makeLabAirPlayGrant asks for the AirPlay lab's grant for the playback mediaToken names, and gives
// its answer back read.
func (harness testHarness) makeLabAirPlayGrant(t *testing.T, user, mediaToken string) (int, labAirPlayGrant) {
	t.Helper()
	answer := harness.do(t, http.MethodPost, "/lab/airplay/grants", user, map[string]string{"mediaToken": mediaToken})
	var made labAirPlayGrant
	if answer.status == http.StatusOK {
		if err := json.Unmarshal([]byte(answer.body), &made); err != nil {
			t.Fatalf("%s: %v", answer.body, err)
		}
	}
	return answer.status, made
}

// getAsAirPlayReceiver GETs path as an AirPlay receiver does: Apple's player, sending no cookie and no
// header of ours.
func (harness testHarness) getAsAirPlayReceiver(t *testing.T, path string) byteAnswer {
	t.Helper()
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, harness.httpServer.URL+path, http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	answer, err := harness.httpServer.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = answer.Body.Close() }() //nolint:errcheck // read to the end below
	text, err := io.ReadAll(answer.Body)
	if err != nil {
		t.Fatal(err)
	}
	return byteAnswer{status: answer.StatusCode, header: answer.Header, body: string(text)}
}

// The AirPlay lab's grant, in the media's path, serves its playback's playlists to a receiver that
// sends no credential, for a day; every URI in them is relative, so the grant carries on to each
// playlist and chunk the receiver fetches (airplay.md item 1, item 3).
func TestLabAirPlay_AGrantInThePathServesItsPlaybackToAReceiverWithNoCredential(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	harness.offerLabAirPlay(t, "true")
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.keepFilmOnesTimeline(t)
	status, made := harness.makeLabAirPlayGrant(t, user, harness.play(t, user, chrome(1)).MediaToken)
	if status != http.StatusOK || !strings.HasPrefix(made.MasterPath, api.Prefix+"/bytes/media/g/") ||
		!strings.HasSuffix(made.MasterPath, "/master.m3u8") {
		t.Fatalf("the grant: %d %+v; want a master playlist under /bytes/media/g/", status, made)
	}
	if want := harness.clock.Now().Add(24 * time.Hour); !made.ExpiresAt.Equal(want) {
		t.Errorf("the grant expires at %v; want a day from now, %v", made.ExpiresAt, want)
	}
	master := harness.getAsAirPlayReceiver(t, made.MasterPath)
	checkFilmOnesMaster(t, master, "sound0-96k-raw")
	if strings.Contains(master.body, "/") && strings.Contains(master.body, "://") ||
		strings.Contains(master.body, "/api/") {
		t.Errorf("the master playlist names an absolute URI, which the grant would not carry to:\n%s", master.body)
	}
	base := strings.TrimSuffix(made.MasterPath, "master.m3u8")
	stream := harness.getAsAirPlayReceiver(t, base+"q1200k480p/stream.m3u8")
	if stream.status != http.StatusOK || !strings.Contains(stream.body, "\n00002.m4s\n#EXT-X-ENDLIST") {
		t.Errorf("a quality's playlist under the grant: %d\n%s", stream.status, stream.body)
	}
	if refused := harness.getAsAirPlayReceiver(t, base+"q3000k720p/stream.m3u8"); refused.status !=
		http.StatusNotFound {
		t.Errorf("a quality the playback is not offered, under the grant: %d, want 404", refused.status)
	}
	unknown := harness.getAsAirPlayReceiver(t, api.Prefix+"/bytes/media/g/not-a-grant/master.m3u8")
	if unknown.status != http.StatusNotFound {
		t.Errorf("an unknown grant: %d, want 404", unknown.status)
	}
}

// A grant is made only for a playback its caller's session holds, and serves only that playback's
// film, under /g/: it signs no one in, so presented as a credential on any other media path it is
// refused, and another session's playback gets none.
func TestLabAirPlay_AGrantIsForItsSessionsPlaybackAloneAndSignsNoOneIn(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	harness.offerLabAirPlay(t, "true")
	user := harness.createAdminAndSignIn(t)
	other := harness.signIn(t, "owner", "owner-password")
	harness.shelveFilms(t)
	harness.keepFilmOnesTimeline(t)
	othersToken := harness.play(t, other, chrome(1)).MediaToken
	if status, _ := harness.makeLabAirPlayGrant(t, user, othersToken); status != http.StatusNotFound {
		t.Errorf("a grant for another session's playback: %d, want 404", status)
	}
	status, made := harness.makeLabAirPlayGrant(t, user, harness.play(t, user, chrome(1)).MediaToken)
	if status != http.StatusOK {
		t.Fatalf("the grant: %d", status)
	}
	grant := strings.TrimSuffix(strings.TrimPrefix(made.MasterPath, api.Prefix+"/bytes/media/g/"), "/master.m3u8")
	othersMaster := api.Prefix + "/bytes/media/chunks/" + othersToken + "/master.m3u8"
	for _, byCookie := range []bool{false, true} {
		refused := harness.getBytePath(t, othersMaster, grant, byCookie, "")
		if refused.status != http.StatusUnauthorized {
			t.Errorf("the grant as a credential (by cookie %v) on another playback's path: %d, want 401",
				byCookie, refused.status)
		}
	}
}

// Stop revokes the session's grants, and says how many: a receiver presenting one is refused from
// then on (airplay.md item 5).
func TestLabAirPlay_StopRevokesTheGrant(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	harness.offerLabAirPlay(t, "true")
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.keepFilmOnesTimeline(t)
	_, made := harness.makeLabAirPlayGrant(t, user, harness.play(t, user, chrome(1)).MediaToken)
	if before := harness.getAsAirPlayReceiver(t, made.MasterPath); before.status != http.StatusOK {
		t.Fatalf("before Stop: %d", before.status)
	}
	stopped := harness.do(t, http.MethodDelete, "/lab/airplay/grants", user, nil)
	if stopped.status != http.StatusOK || strings.TrimSpace(stopped.body) != `{"revoked":1}` {
		t.Errorf("Stop: %d %s; want one grant revoked", stopped.status, stopped.body)
	}
	if after := harness.getAsAirPlayReceiver(t, made.MasterPath); after.status != http.StatusNotFound {
		t.Errorf("after Stop: %d, want 404", after.status)
	}
	base := strings.TrimSuffix(made.MasterPath, "master.m3u8")
	if after := harness.getAsAirPlayReceiver(t, base+"q1200k480p/stream.m3u8"); after.status != http.StatusNotFound {
		t.Errorf("a stream after Stop: %d, want 404", after.status)
	}
}

// The AirPlay lab is off unless an admin turns it on, as staging's does: off, no grant is made, and
// one made before is refused on every path, so the path grant exists nowhere the lab is not offered
// (D154 §6 is not amended beyond the lab). Stop still revokes.
func TestLabAirPlay_IsOffByDefaultAndRefusedWhileOff(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.keepFilmOnesTimeline(t)
	token := harness.play(t, user, chrome(1)).MediaToken
	if status, _ := harness.makeLabAirPlayGrant(t, user, token); status != http.StatusNotFound {
		t.Errorf("a grant from a server whose lab is off by default: %d, want 404", status)
	}
	harness.offerLabAirPlay(t, "true")
	_, made := harness.makeLabAirPlayGrant(t, user, token)
	harness.offerLabAirPlay(t, "false")
	base := strings.TrimSuffix(made.MasterPath, "master.m3u8")
	for _, path := range []string{made.MasterPath, base + "q1200k480p/stream.m3u8", base + "q1200k480p/00000.m4s"} {
		if refused := harness.getAsAirPlayReceiver(t, path); refused.status != http.StatusNotFound ||
			!strings.Contains(refused.body, "turned off") {
			t.Errorf("%s while the lab is off: %d %s, want 404", path, refused.status, refused.body)
		}
	}
	if stopped := harness.do(t, http.MethodDelete, "/lab/airplay/grants", user, nil); stopped.status != http.StatusOK ||
		strings.TrimSpace(stopped.body) != `{"revoked":1}` {
		t.Errorf("Stop while the lab is off: %d %s; want the grant revoked", stopped.status, stopped.body)
	}
}

// A master playlist asked under a grant for a film with no grid yet is held while it is read through,
// for the lab's hold, before the receiver is told to ask again: AVPlayer does not honour the 425 that
// hls.js is answered at once (airplay.md, the 2026-11-12 findings).
func TestLabAirPlay_AMasterWithNoGridIsHeldForTheReceiver(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	harness.offerLabAirPlay(t, "true")
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	token := harness.play(t, user, chrome(1)).MediaToken
	_, made := harness.makeLabAirPlayGrant(t, user, token)
	askedAt := harness.clock.Now()
	held := harness.getAsAirPlayReceiver(t, made.MasterPath)
	if held.status != http.StatusTooEarly || harness.clock.Now().Sub(askedAt) < 30*time.Second {
		t.Errorf("under a grant, a film with no grid: %d after %v; want 425 after a 30 s hold", held.status,
			harness.clock.Now().Sub(askedAt))
	}
	askedAt = harness.clock.Now()
	early := harness.getBytePath(t, api.Prefix+"/bytes/media/chunks/"+token+"/master.m3u8", user, true, "")
	if early.status != http.StatusTooEarly || harness.clock.Now().Sub(askedAt) != 0 {
		t.Errorf("by the token, for hls.js: %d after %v; want 425 at once", early.status,
			harness.clock.Now().Sub(askedAt))
	}
}
