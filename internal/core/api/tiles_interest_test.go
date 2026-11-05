// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"net/http"
	"slices"
	"testing"
	"time"
)

// A session says the tiles a list shows in view, for their pictures' work to come first, and that it
// shows none (D362 §4); a title it does not reach raises nothing
// (TestReach_AnEditorReachesOnlyTheLibrariesTheirRoleLists).
func TestInterest_ASessionSaysTheTilesItShowsAndLeavesThem(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.ambiguousTitle(t)
	shown := harness.do(t, http.MethodPut, "/interest/tiles", admin, map[string][]int64{"items": {10}})
	if shown.status != http.StatusNoContent {
		t.Fatalf("PUT /interest/tiles = %d %s", shown.status, shown.body)
	}
	if looks := harness.looks.Looks(time.Minute); len(looks) != 1 || !slices.Equal(looks[0].Tiles, []int64{10}) {
		t.Errorf("looks %+v; want the session's tiles, film 10's", looks)
	}
	if left := harness.do(t, http.MethodDelete, "/interest/tiles", admin, nil); left.status != http.StatusNoContent {
		t.Fatalf("DELETE /interest/tiles = %d %s", left.status, left.body)
	}
	if looks := harness.looks.Looks(time.Minute); len(looks) != 0 {
		t.Errorf("looks %+v after the list was left; want none", looks)
	}
}
