// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/api"
)

// makeTogetherAirPlayGrant asks, by a Watch Together membership credential, for an AirPlay grant for
// the playback mediaToken names.
func (harness testHarness) makeTogetherAirPlayGrant(t *testing.T, credential, mediaToken string) response {
	t.Helper()
	return harness.do(t, http.MethodPost, "/together/airplay-grants", credential,
		map[string]string{"mediaToken": mediaToken})
}

// togetherAirPlayMaster is the master playlist's path a 200 from POST /together/airplay-grants
// answered.
func togetherAirPlayMaster(t *testing.T, answer response) string {
	t.Helper()
	var made airPlayGrant
	if err := json.Unmarshal([]byte(answer.body), &made); err != nil || answer.status != http.StatusOK ||
		!strings.HasPrefix(made.MasterPath, api.Prefix+"/bytes/media/g/") {
		t.Fatalf("POST /together/airplay-grants = %d %s; want a master playlist under /bytes/media/g/",
			answer.status, answer.body)
	}
	return made.MasterPath
}

// A Watch Together guest by link, signed in to nothing, AirPlays the group's film as their membership
// while they are in (D519 §1): their membership credential asks for a grant for the playback it holds,
// which a receiver sending no credential is served; the host's credential and no one's are refused;
// the page's Stop revokes it; the guest's next cast replaces it (D519 §2); and once the guest is
// removed it serves nothing, nor is another made.
func TestTogetherAirPlay_AGuestByLinkAirPlaysAsTheirMembershipWhileTheyAreIn(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
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
	wantError(t, harness.makeTogetherAirPlayGrant(t, opened.Body.MembershipCredential, playing.MediaToken),
		http.StatusNotFound, api.CodeNotFound)
	wantError(t, harness.makeTogetherAirPlayGrant(t, "no one's", playing.MediaToken),
		http.StatusNotFound, api.CodeNotFound)
	master := togetherAirPlayMaster(t, harness.makeTogetherAirPlayGrant(t, credential, playing.MediaToken))
	if served := harness.getAsAirPlayReceiver(t, master); served.status != http.StatusOK {
		t.Fatalf("the guest's grant's master playlist = %d %s", served.status, served.body)
	}
	harness.logs.AwaitEvent(t, "togetherAirPlayGranted", fmt.Sprintf(`"memberId":%d`, membership.Body.MemberID))
	stopped := harness.do(t, http.MethodDelete, "/together/airplay-grants", credential, nil)
	if stopped.status != http.StatusOK || strings.TrimSpace(stopped.body) != `{"revoked":1}` {
		t.Errorf("the guest's Stop: %d %s; want their one grant revoked", stopped.status, stopped.body)
	}
	if after := harness.getAsAirPlayReceiver(t, master); after.status != http.StatusNotFound {
		t.Errorf("the guest's grant after their Stop: %d, want 404", after.status)
	}
	first := togetherAirPlayMaster(t, harness.makeTogetherAirPlayGrant(t, credential, playing.MediaToken))
	second := togetherAirPlayMaster(t, harness.makeTogetherAirPlayGrant(t, credential, playing.MediaToken))
	if replaced := harness.getAsAirPlayReceiver(t, first); replaced.status != http.StatusNotFound {
		t.Errorf("the guest's grant replaced by their next cast: %d, want 404", replaced.status)
	}
	if served := harness.getAsAirPlayReceiver(t, second); served.status != http.StatusOK {
		t.Fatalf("the guest's newest grant = %d %s", served.status, served.body)
	}
	ejected := harness.do(t, http.MethodPost, fmt.Sprintf("/together/%d/members/%d/eject",
		opened.Body.SessionID, membership.Body.MemberID), admin, nil)
	if ejected.status != http.StatusOK && ejected.status != http.StatusNoContent {
		t.Fatalf("the guest's removal = %d %s", ejected.status, ejected.body)
	}
	if after := harness.getAsAirPlayReceiver(t, second); after.status != http.StatusNotFound {
		t.Errorf("the removed guest's grant: %d, want 404", after.status)
	}
	wantError(t, harness.makeTogetherAirPlayGrant(t, credential, playing.MediaToken),
		http.StatusNotFound, api.CodeNotFound)
}
