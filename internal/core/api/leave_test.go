// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"net/http"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/jobs"
)

// A player that leaves its film's page lets go of the work its playback asked for: the read-through
// its chunks wait on is no longer wanted, so it falls to background at the next demotion (D258 §2).
// Another person cannot let go of it, and the token goes on serving, for the player's page that
// takes it over.
func TestLeave_APlayerThatLeavesLetsGoOfTheWorkItsPlaybackAskedFor(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	token := harness.play(t, admin, chrome(1)).MediaToken
	master := Prefix + "/bytes/media/chunks/" + token + "/master.m3u8"
	harness.getBytePath(t, master, admin, true, "") // queues film 1's read-through at play priority
	// The play's wish is kept in memory and written when the demotion runs (D303, DL10).
	if _, err := harness.queue.DemoteUnwanted(t.Context()); err != nil {
		t.Fatal(err)
	}
	now := harness.clock.Now().UnixMilli()
	if read := harness.openJobs(t, jobs.KindReadThrough); len(read) != 1 || read[0].WantedUntil == nil ||
		*read[0].WantedUntil <= now {
		t.Fatalf("the read-throughs queued = %+v; want film 1's, wanted", read)
	}
	if _, err := harness.authService.CreateUser(t.Context(),
		auth.NewUser{Name: "partner", Password: "partner-password", Role: auth.RoleUser}); err != nil {
		t.Fatal(err)
	}
	guest := harness.signIn(t, "partner", "partner-password")
	if refused := harness.do(t, http.MethodDelete, "/playback/"+token, guest, nil); refused.status !=
		http.StatusNotFound {
		t.Errorf("another person's leave: %d %s, want 404", refused.status, refused.body)
	}
	if read := harness.openJobs(t, jobs.KindReadThrough); *read[0].WantedUntil <= now {
		t.Errorf("another person's leave let go of the read-through, wanted until %d", *read[0].WantedUntil)
	}
	if left := harness.do(t, http.MethodDelete, "/playback/"+token, admin, nil); left.status !=
		http.StatusNoContent {
		t.Fatalf("the leave: %d %s, want 204", left.status, left.body)
	}
	if read := harness.openJobs(t, jobs.KindReadThrough); len(read) != 1 || read[0].WantedUntil == nil ||
		*read[0].WantedUntil >= now {
		t.Errorf("after the leave, the read-through = %+v; want it no longer wanted", read)
	}
	if again := harness.getBytePath(t, master, admin, true, ""); again.status != http.StatusTooEarly {
		t.Errorf("the token after a leave: %d %s, want it still serving, 425 until film 1 has a grid",
			again.status, again.body)
	}
}
