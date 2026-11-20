// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/library/librarytest"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// shelveEpisode adds episode number of Brass Arena's season 1, title 20+number on file number+1,
// 1000 ms long, after those before it in its aired order.
func (harness testHarness) shelveEpisode(t *testing.T, number int64) {
	t.Helper()
	title, file := 20+number, number+1
	librarytest.Title(t, harness.database, librarytest.TitleSpec{ID: title, Library: 1, Kind: "episode", AddedAt: 2})
	librarytest.Version(t, harness.database, librarytest.VersionSpec{ID: file, TitleID: title, CreatedAt: 2})
	librarytest.File(t, harness.database, librarytest.FileSpec{
		ID: file, LibraryID: 1, NodeID: 1, Path: fmt.Sprintf("/m/BB/S01E%02d.mkv", number), SizeBytes: 10,
		PayloadHash: []byte(fmt.Sprint(file)), Filing: "listed", VersionID: file,
	})
	librarytest.Probe(t, harness.database, librarytest.ProbeSpec{FileID: file, DurationMs: 1000})
	storetest.ExecRaw(t, harness.databasePath,
		storetest.Positions(fmt.Sprintf("(1, %d, 1, %d, %d)", title, number, 10000+number)))
}

// shelveSixEpisodes is shelve with Brass Arena's season 1 six episodes long: titles 21 to 26, on
// files 2 to 7, each 1000 ms long.
func (harness testHarness) shelveSixEpisodes(t *testing.T) {
	t.Helper()
	harness.shelve(t)
	librarytest.Probe(t, harness.database, librarytest.ProbeSpec{FileID: 2, DurationMs: 1000})
	for number := int64(2); number <= 6; number++ {
		harness.shelveEpisode(t, number)
	}
}

// homeOf is GET /home for token.
func (harness testHarness) homeOf(t *testing.T, token string) api.HomeOutput {
	t.Helper()
	answer := harness.do(t, http.MethodGet, "/home", token, nil)
	var home api.HomeOutput
	if err := json.Unmarshal([]byte(answer.body), &home.Body); err != nil || answer.status != http.StatusOK {
		t.Fatalf("GET /home = %d %s", answer.status, answer.body)
	}
	return home
}

// nextUpRow names Home's Next Up for token, each row its episode's id and the file it plays, and
// Continue Watching's works after a bar: "26:7 | 21".
func (harness testHarness) nextUpRow(t *testing.T, token string) string {
	t.Helper()
	home := harness.homeOf(t, token)
	var nextUp, watching []string
	for _, row := range home.Body.NextUp {
		nextUp = append(nextUp, fmt.Sprintf("%d:%d", row.ID, row.FileID))
	}
	for _, row := range home.Body.ContinueWatching {
		watching = append(watching, fmt.Sprint(row.ID))
	}
	return strings.TrimSpace(strings.Join(nextUp, " ") + " | " + strings.Join(watching, " "))
}

// Home's Next Up is, for each series the person finished an episode of and is partway through none
// of, the episode after the latest they finished, in their ordering (play-state.md, plans/b1.md §A):
// an earlier one watched again does not move it back; one partway through puts the series in
// Continue Watching and out of Next Up; the finale finished leaves both rows; a new episode after the
// finale brings the series back.
func TestNextUp_OnHomeIsTheEpisodeAfterTheLatestFinishedOfASeriesPartwayThroughNone(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveSixEpisodes(t)
	if got, want := harness.nextUpRow(t, user), "|"; got != want {
		t.Errorf("nothing watched: Home %q; want %q, no row", got, want)
	}

	for number := int64(1); number <= 4; number++ {
		harness.setPositionReadAt(t, number+1, 1000, 10*number) // episodes 1 to 4, in order
	}
	harness.setPositionReadAt(t, 6, 1000, 50) // episode 5
	harness.setPositionReadAt(t, 4, 1000, 60) // then episode 3 again, to its end
	if got, want := harness.nextUpRow(t, user), "26:7 |"; got != want {
		t.Errorf("5 then 3 again: Home %q; want %q, episode 6", got, want)
	}
	row := harness.homeOf(t, user).Body.NextUp[0]
	if row.Kind != "episode" || row.PositionMs != 0 || row.Episode == nil ||
		row.Episode.SeriesName != "Brass Arena" || row.Episode.SeasonNumber != 1 || row.Episode.EpisodeNumber != 6 ||
		row.PosterSha256 != strings.Repeat("cc", 32) {
		t.Errorf("Next Up's row = %+v; want episode 6 of Brass Arena's season 1 from its start, with its poster", row)
	}

	harness.setPositionReadAt(t, 7, 500, 70) // partway through episode 6
	if got, want := harness.nextUpRow(t, user), "| 26"; got != want {
		t.Errorf("partway through episode 6: Home %q; want %q, it in Continue Watching alone", got, want)
	}
	harness.setPositionReadAt(t, 7, 1000, 80) // the finale finished
	if got, want := harness.nextUpRow(t, user), "|"; got != want {
		t.Errorf("the finale finished: Home %q; want %q, no row", got, want)
	}
	harness.shelveEpisode(t, 7)
	if got, want := harness.nextUpRow(t, user), "27:8 |"; got != want {
		t.Errorf("a seventh episode added: Home %q; want %q, it", got, want)
	}
}

// A series hidden from Next Up stays out until the person next watches any of its episodes (D188),
// and Undo brings it back at once; hiding it is not watching it.
func TestNextUp_AHiddenSeriesStaysOutUntilItsNextActivity(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveSixEpisodes(t)
	harness.setPositionReadAt(t, 2, 1000, 10)
	harness.setPositionReadAt(t, 3, 1000, 20)
	hide := func(method string) {
		t.Helper()
		if answer := harness.do(t, method, "/items/23/hidden", user, nil); answer.status != http.StatusNoContent {
			t.Fatalf("%s /items/23/hidden = %d %s", method, answer.status, answer.body)
		}
	}
	harness.clock.Advance(1)
	hide(http.MethodPut)
	if got, want := harness.nextUpRow(t, user), "|"; got != want {
		t.Errorf("hidden: Home %q; want %q", got, want)
	}
	hide(http.MethodDelete)
	if got, want := harness.nextUpRow(t, user), "23:4 |"; got != want {
		t.Errorf("the hide undone: Home %q; want %q", got, want)
	}
	hide(http.MethodPut)
	// Episode 1 watched again to its end, after the hide: activity in the series, which leaves its place.
	harness.setPositionReadAt(t, 2, 1000, harness.clock.Now().UnixMilli()+1)
	if got, want := harness.nextUpRow(t, user), "23:4 |"; got != want {
		t.Errorf("an episode of it watched since the hide: Home %q; want %q, it back", got, want)
	}
}

// Next Up holds only the series of the libraries the person reaches (D243 §4), most recent
// watching first.
func TestNextUp_HoldsTheSeriesOfTheLibrariesThePersonReachesMostRecentFirst(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	partnerID, err := harness.authService.CreateUser(t.Context(),
		auth.NewUser{Name: "partner", Password: "partner-password", Role: auth.RoleUser})
	if err != nil {
		t.Fatal(err)
	}
	partner := harness.signIn(t, "partner", "partner-password")
	harness.shelveSixEpisodes(t)
	// A second series, 50, of one episode, 51, and its second, 52, on files 9 and 10.
	librarytest.Item(t, harness.database, librarytest.ItemSpec{
		ID: 50, Kind: "series", LibraryID: 1, AddedAt: 2, IdentificationState: "identified",
	})
	for _, title := range []int64{51, 52} {
		file := title - 42
		librarytest.Title(t, harness.database,
			librarytest.TitleSpec{ID: title, Library: 1, Kind: "episode", AddedAt: 2})
		librarytest.Version(t, harness.database, librarytest.VersionSpec{ID: file, TitleID: title, CreatedAt: 2})
		librarytest.File(t, harness.database, librarytest.FileSpec{
			ID: file, LibraryID: 1, NodeID: 1, Path: fmt.Sprintf("/m/X/S01E%02d.mkv", title-50), SizeBytes: 10,
			PayloadHash: []byte(fmt.Sprint(file)), Filing: "listed", VersionID: file,
		})
		librarytest.Probe(t, harness.database, librarytest.ProbeSpec{FileID: file, DurationMs: 1000})
	}
	storetest.ExecRaw(t, harness.databasePath,
		"INSERT INTO sequence (id, kind, created_by) VALUES (50, 'series', 'scanner')",
		"INSERT INTO ordering (id, sequence_id, name, kind) VALUES (3, 50, 'aired', 'aired')",
		storetest.Positions("(3, 51, 1, 1, 10001), (3, 52, 1, 2, 10002)"))
	watch := func(fileID, at int64) {
		librarytest.Watch(t, harness.database, librarytest.WatchSpec{
			UserID: partnerID, FileID: fileID, PositionMs: 1000, ReadAt: at,
		})
	}
	watch(2, 10) // Brass Arena's first episode
	watch(9, 20) // then series 50's
	if got, want := harness.nextUpRow(t, partner), "52:10 22:3 |"; got != want {
		t.Errorf("both reached: Home %q; want %q, the latest watched first", got, want)
	}
	watch(3, 30) // Brass Arena's second episode, the latest now
	if got, want := harness.nextUpRow(t, partner), "23:4 52:10 |"; got != want {
		t.Errorf("Brass Arena watched since: Home %q; want %q, it first", got, want)
	}

	harness.giveRoleLibraries(t, admin, "user") // the user role reaches no library now
	if got, want := harness.nextUpRow(t, partner), "|"; got != want {
		t.Errorf("the library taken back: Home %q; want %q, no row", got, want)
	}
}
