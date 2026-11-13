// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/core/together"
)

// A guest whose link gives canNext is told what plays after the session's film and before it, in the
// host's ordering, and puts the next one on for everyone; a guest whose link withholds it is told
// nothing of it, and their next is refused (D471 §2, §3).
func TestTogether_AGuestWithTheRightSeesUpNextAndPutsItOn(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelveDVDOrder(t) // aired: 21 (file 2), 22 (3), 23 (4), then 24 (5)
	opened := harness.openTogether(t, admin)
	joinWith := func(rights map[string]bool, name string) api.JoinTogetherOutput {
		made := harness.do(t, http.MethodPost, fmt.Sprintf("/together/%d/links", opened.Body.SessionID), admin,
			map[string]any{"hostNote": name, "rights": rights})
		var link api.TogetherLinkOutput
		if err := json.Unmarshal([]byte(made.body), &link.Body); err != nil || made.status != http.StatusCreated {
			t.Fatalf("POST /together/%d/links = %d %s", opened.Body.SessionID, made.status, made.body)
		}
		joined := harness.do(t, http.MethodPost, "/together/join", "",
			map[string]any{"secret": link.Body.Secret, "name": name})
		var membership api.JoinTogetherOutput
		if err := json.Unmarshal([]byte(joined.body), &membership.Body); err != nil || joined.status != http.StatusOK {
			t.Fatalf("POST /together/join = %d %s", joined.status, joined.body)
		}
		return membership
	}
	given := joinWith(map[string]bool{"canPlayPause": true, "canSeek": true, "canNext": true}, "Anna")
	withheld := joinWith(map[string]bool{"canPlayPause": true, "canSeek": true, "canNext": false}, "Ben")
	host := harness.dialTogether(t, opened.Body.MembershipCredential)
	host.awaitWelcome()
	givenDevice := harness.dialTogether(t, given.Body.MembershipCredential)
	if welcome := givenDevice.awaitWelcome(); welcome.Rights == nil || !welcome.Rights.CanNext {
		t.Fatalf("the guest given canNext was welcomed with %+v", welcome.Rights)
	}
	withheldDevice := harness.dialTogether(t, withheld.Body.MembershipCredential)
	withheldDevice.awaitWelcome()
	putOn(host, givenDevice, 2)
	var around struct {
		UpNext *struct {
			Episode struct {
				FileID int64 `json:"fileId"`
			} `json:"episode"`
		} `json:"upNext"`
		Previous *struct{} `json:"previous"`
	}
	harness.readJSON(t, "/together/up-next", given.Body.MembershipCredential, &around)
	if around.UpNext == nil || around.UpNext.Episode.FileID != 3 || around.Previous != nil {
		t.Errorf("the guest given canNext was told up next %+v, previous %v; want file 3, and none before the first",
			around.UpNext, around.Previous)
	}
	wantError(t, harness.do(t, http.MethodGet, "/together/up-next", withheld.Body.MembershipCredential, nil),
		http.StatusForbidden, api.CodeForbidden)
	withheldDevice.send(together.Inbound{
		V: together.ProtocolVersion, Type: together.MessageCommand, Command: together.CommandNext,
	})
	if refused := withheldDevice.awaitRejected(); refused.Code != "forbidden" {
		t.Errorf("the guest without canNext had their next refused %q; want forbidden", refused.Code)
	}
	givenDevice.send(together.Inbound{
		V: together.ProtocolVersion, Type: together.MessageCommand, Command: together.CommandNext,
	})
	for {
		if playing := givenDevice.awaitGroup(together.GroupPlaying); playing.Group.FileID == 3 {
			break
		}
	}
}
