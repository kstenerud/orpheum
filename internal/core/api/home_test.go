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

// continueWatching is the owner's Continue Watching, by the API.
func (harness testHarness) continueWatching(t *testing.T, sessionToken string) []api.InProgressBody {
	t.Helper()
	answer := harness.do(t, http.MethodGet, "/home", sessionToken, nil)
	var home struct {
		ContinueWatching []api.InProgressBody `json:"continueWatching"`
	}
	if err := json.Unmarshal([]byte(answer.body), &home); err != nil || answer.status != http.StatusOK {
		t.Fatalf("GET /home = %d %s", answer.status, answer.body)
	}
	return home.ContinueWatching
}

// setPositionReadAt sets user 1's position in a file, as a report that read it at positionReadAt
// would.
func (harness testHarness) setPositionReadAt(t *testing.T, fileID, positionMs, positionReadAt int64) {
	t.Helper()
	librarytest.Watch(t, harness.database, librarytest.WatchSpec{
		FileID: fileID, PositionMs: positionMs, ReadAt: positionReadAt,
	})
}

// rowsOf names Continue Watching's rows: each work's id and the file it resumes.
func rowsOf(rows []api.InProgressBody) []string {
	names := []string{}
	for _, row := range rows {
		names = append(names, fmt.Sprintf("%d:%d:%s", row.ID, row.FileID, row.FileName))
	}
	return names
}

func TestContinueWatching_EachWorkOnceAtItsLatestPositionPartwayThrough(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelve(t)
	// Metropolis has a second copy, and an extra; Metropolis's first copy is 1000 s long.
	librarytest.Probe(t, harness.database, librarytest.ProbeSpec{
		FileID: 1, ProbeJSON: `{"format":{"durationMs":1000000}}`, DurationMs: 1000000,
	})
	librarytest.Version(t, harness.database, librarytest.VersionSpec{ID: 3, TitleID: 10, CreatedAt: 3})
	librarytest.Title(t, harness.database, librarytest.TitleSpec{ID: 30, Library: 1, Kind: "extra", AddedAt: 3})
	librarytest.Version(t, harness.database, librarytest.VersionSpec{ID: 4, TitleID: 30, CreatedAt: 3})
	librarytest.File(t, harness.database, librarytest.FileSpec{
		ID: 3, LibraryID: 1, NodeID: 1, Path: "/m/Metropolis (1995) - 720p.mkv", SizeBytes: 500,
		PayloadHash: []byte{0x03}, Filing: "listed", VersionID: 3,
	})
	librarytest.File(t, harness.database, librarytest.FileSpec{
		ID: 4, LibraryID: 1, NodeID: 1, Path: "/m/Metropolis (1995)-trailer.mkv", SizeBytes: 5,
		PayloadHash: []byte{0x04}, Filing: "extra", VersionID: 4,
	})
	harness.setPositionReadAt(t, 3, 300_000, 100) // Metropolis's 720p copy, earlier
	harness.setPositionReadAt(t, 2, 60_000, 150)  // the pilot, of unknown length
	harness.setPositionReadAt(t, 1, 500_000, 200) // Metropolis, halfway, latest
	harness.setPositionReadAt(t, 4, 30_000, 250)  // the trailer: an extra is never in progress (D162)

	want := []string{"10:1:Metropolis (1995).mkv", "21:2:"} // Metropolis names its file: two are partway
	if got := rowsOf(harness.continueWatching(t, user)); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("Continue Watching = %v, want %v", got, want)
	}
	rows := harness.continueWatching(t, user)
	if rows[0].PositionMs != 500_000 || rows[0].DurationMs != 1_000_000 || rows[0].PosterSha256 == "" ||
		rows[1].Episode == nil || rows[1].Episode.SeriesName != "Brass Arena" {
		t.Errorf("rows = %+v, want Metropolis halfway with its poster, and the pilot with its series", rows)
	}

	harness.setPositionReadAt(t, 1, 950_000, 300) // Metropolis finished: 95%
	want = []string{"10:3:", "21:2:"}             // its other copy is still partway, and now the only one
	if got := rowsOf(harness.continueWatching(t, user)); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("after finishing one copy, Continue Watching = %v, want %v", got, want)
	}
	harness.setPositionReadAt(t, 1, 20_000, 310) // 2%: barely started
	if got := rowsOf(harness.continueWatching(t, user)); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("after barely starting it again, Continue Watching = %v, want %v", got, want)
	}
}

// An episode has no poster of its own, only a still, so its row shows its series' poster rather
// than the mark of a missing one (the owner, 2026-11-01).
func TestContinueWatching_AnEpisodeShowsItsSeriesPoster(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelve(t)
	harness.setPositionReadAt(t, 2, 60_000, 150) // the pilot of Brass Arena, whose poster is cc…

	rows := harness.continueWatching(t, user)
	if len(rows) != 1 || rows[0].PosterSha256 != strings.Repeat("cc", 32) {
		t.Errorf("Continue Watching = %+v, want the pilot with Brass Arena's poster", rows)
	}
}

func TestContinueWatching_AHiddenWorkLeavesUntilItHasActivityAndUndoBringsItBack(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelve(t)
	harness.setPositionReadAt(t, 1, 500_000, 100)
	harness.setPositionReadAt(t, 2, 60_000, 50)
	hide := func(method string, id int) {
		t.Helper()
		if answer := harness.do(t, method, fmt.Sprintf("/items/%d/hide", id), user, nil); answer.status !=
			http.StatusNoContent {
			t.Fatalf("%s /items/%d/hide = %d %s", method, id, answer.status, answer.body)
		}
	}
	harness.clock.Advance(1)
	hide(http.MethodPost, 10)
	if got := rowsOf(harness.continueWatching(t, user)); fmt.Sprint(got) != "[21:2:]" {
		t.Errorf("after hiding Metropolis: %v, want the pilot only", got)
	}
	hide(http.MethodDelete, 10)
	if got := rowsOf(harness.continueWatching(t, user)); fmt.Sprint(got) != "[10:1: 21:2:]" {
		t.Errorf("after undoing the hide: %v, want Metropolis back", got)
	}
	hide(http.MethodPost, 10)
	harness.setPositionReadAt(t, 1, 510_000, harness.clock.Now().UnixMilli()+1) // watched again after the hide
	if got := rowsOf(harness.continueWatching(t, user)); fmt.Sprint(got) != "[10:1: 21:2:]" {
		t.Errorf("after watching it again: %v, want Metropolis back by itself (D188)", got)
	}
	if answer := harness.do(t, http.MethodPost, "/items/99/hide", user, nil); answer.status != http.StatusNotFound {
		t.Errorf("hiding what is not there: %d, want 404", answer.status)
	}
}

// A work in Continue Watching none of whose files can be read now says why, so its row leads to
// its page rather than to a play that cannot start (D241 §3).
func TestContinueWatching_AWorkThatCannotBeReadNowSaysWhy(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelve(t)
	harness.setPositionReadAt(t, 2, 60_000, 150) // the pilot
	if rows := harness.continueWatching(t, user); len(rows) != 1 || rows[0].Unavailable != nil {
		t.Fatalf("Continue Watching = %+v; want the pilot, playable", rows)
	}
	harness.nodes.isAway.Store(true)
	rows := harness.continueWatching(t, user)
	if len(rows) != 1 || rows[0].Unavailable == nil || rows[0].Unavailable.Reason != "node_away" ||
		rows[0].Unavailable.NodeName != "local" {
		t.Errorf("Continue Watching while its node is away = %+v; want the pilot marked, its node away", rows)
	}
}
