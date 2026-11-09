// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/library/librarytest"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// fileFilmsAsTitles files shelveFilms' two films as titles 10 and 11, the first's length known as
// 1,000 s, as a play's threshold reads it.
func (harness testHarness) fileFilmsAsTitles(t *testing.T) {
	t.Helper()
	harness.fileFilmAsTitle(t, "First")
	librarytest.Title(t, harness.database, librarytest.TitleSpec{ID: 11, Library: 1})
	librarytest.Version(t, harness.database, librarytest.VersionSpec{ID: 2, TitleID: 11})
	storetest.ExecRaw(t, harness.databasePath,
		"UPDATE file_filing SET version_id = 2 WHERE file_id = 2",
		"INSERT INTO item_text (item_id, language, is_original, name) VALUES (11, 'und', 1, 'Second')")
}

// isTitleWatched is whether GET /items/{id} says its person watched it.
func (harness testHarness) isTitleWatched(t *testing.T, token string, titleID int64) bool {
	t.Helper()
	var title struct {
		IsWatched bool `json:"isWatched"`
	}
	harness.readJSON(t, fmt.Sprintf("/items/%d", titleID), token, &title)
	return title.IsWatched
}

// homeIDs are the works Continue Watching shows, each with whether it says it was watched before.
func (harness testHarness) homeIDs(t *testing.T, token string) string {
	t.Helper()
	var rows []string
	for _, row := range harness.continueWatching(t, token) {
		rows = append(rows, fmt.Sprintf("%d:%t", row.ID, row.IsWatched))
	}
	return strings.Join(rows, " ")
}

// mark sends a mark by hand and returns what it says was there before.
func (harness testHarness) mark(t *testing.T, method, path, token string) []api.WatchedStateBody {
	t.Helper()
	answer := harness.do(t, method, path, token, nil)
	var marked struct {
		Previous []api.WatchedStateBody `json:"previous"`
	}
	if err := json.Unmarshal([]byte(answer.body), &marked); err != nil || answer.status != http.StatusOK {
		t.Fatalf("%s %s = %d %s (%v)", method, path, answer.status, answer.body, err)
	}
	return marked.Previous
}

// undo puts marks back as a mark's answer gave them.
func (harness testHarness) undo(t *testing.T, token string, previous []api.WatchedStateBody) {
	t.Helper()
	answer := harness.do(t, http.MethodPut, "/me/watched", token, map[string]any{"states": previous})
	if answer.status != http.StatusNoContent {
		t.Fatalf("PUT /me/watched = %d %s", answer.status, answer.body)
	}
}

// A film becomes watched when its person plays it past finished_after_percent of a known length, or to
// its end, and stays watched when they start it again, which shows the mark and resumes as before;
// unattended watching, and a far position in a file of unknown length, make nothing watched (D370 §1).
func TestWatched_APlayToItsFinishMarksItWatchedAndARestartKeepsIt(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.fileFilmsAsTitles(t)
	// Film 2 is 1,200 s long. Unattended past 95%, it is not watched.
	unattended := reportBody(1, 1_150_000, 0)
	unattended["spans"] = []any{map[string]any{
		"fromPositionMs": 0, "toPositionMs": 1_150_000, "startedMonoMs": 900_000, "endedMonoMs": 1_000_000,
		"isUnattended": true,
	}}
	harness.report(t, user, harness.play(t, user, chrome(2)), unattended)
	if harness.isTitleWatched(t, user, 11) {
		t.Errorf("played past its finish unattended: watched; want not (D370 §1)")
	}
	harness.clock.Advance(time.Second)
	harness.report(t, user, harness.play(t, user, chrome(2)), reportBody(1, 1_150_000, 0))
	if !harness.isTitleWatched(t, user, 11) {
		t.Errorf("played past 90%% of its length: not watched; want watched")
	}
	// Started again and stopped a tenth in: still watched, and resumed there.
	harness.clock.Advance(time.Second)
	harness.report(t, user, harness.play(t, user, chrome(2)), reportBody(1, 120_000, 0))
	harness.clock.Advance(time.Second)
	if !harness.isTitleWatched(t, user, 11) {
		t.Errorf("restarted after it was finished: not watched; want it still watched")
	}
	if got, want := harness.homeIDs(t, user), "11:true"; got != want {
		t.Errorf("Continue Watching after a rewatch began: %q; want %q, the rewatch with its mark", got, want)
	}
	if got := harness.play(t, user, chrome(2)).ResumePositionMs; got != 120_000 {
		t.Errorf("the rewatch resumes at %d; want 120000", got)
	}
	// Film 1's length is unknown: far along is not finished; its end is.
	harness.report(t, user, harness.play(t, user, chrome(1)), reportBody(1, 50_000_000, 0))
	if harness.isTitleWatched(t, user, 10) {
		t.Errorf("a file of unknown length far along: watched; want not, only its end finishes it")
	}
	ended := reportBody(1, 50_100_000, 0)
	ended["events"] = []any{
		map[string]any{"kind": "started", "positionMs": 50_000_000, "happenedMonoMs": 990_000},
		map[string]any{"kind": "completed", "positionMs": 50_100_000, "happenedMonoMs": 1_000_000},
	}
	harness.clock.Advance(time.Second)
	harness.report(t, user, harness.play(t, user, chrome(1)), ended)
	if !harness.isTitleWatched(t, user, 10) {
		t.Errorf("a file of unknown length played to its end: not watched; want watched")
	}
	var films struct {
		Items []api.ItemEntryBody `json:"items"`
	}
	harness.readJSON(t, "/items?kind=movie&sort=name&limit=10", user, &films)
	for _, film := range films.Items {
		if !film.IsWatched {
			t.Errorf("the films list: %d not watched; want both watched", film.ID)
		}
	}
}

// wantFilm11 checks film 11 as its person sees it: "watched home from resume", whether it is watched,
// Continue Watching's rows (homeIDs) and where its next play starts.
func (harness testHarness) wantFilm11(t *testing.T, token, what, want string) {
	t.Helper()
	got := fmt.Sprintf("%t %s from %d", harness.isTitleWatched(t, token, 11), harness.homeIDs(t, token),
		harness.play(t, token, chrome(2)).ResumePositionMs)
	if got != want {
		t.Errorf("%s: %q; want %q", what, got, want)
	}
}

// Marked watched by hand, a film leaves Continue Watching and plays next from its start; marked
// unwatched, it is as never watched; each mark's answer is what Undo puts back, the film partway as
// before (D370 §4).
func TestWatched_AMarkByHandIsAFreshStartAndUndoPutsItBack(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.fileFilmsAsTitles(t)
	harness.report(t, user, harness.play(t, user, chrome(2)), reportBody(1, 600_000, 0))
	harness.clock.Advance(time.Second)
	if got, want := harness.homeIDs(t, user), "11:false"; got != want {
		t.Fatalf("partway: Continue Watching %q; want %q", got, want)
	}

	previous := harness.mark(t, http.MethodPut, "/items/11/watched", user)
	if len(previous) != 1 || previous[0].ItemID != 11 || previous[0].WatchedAt != nil || previous[0].MarkedAt != nil {
		t.Errorf("marked watched: previous %+v; want film 11 with no mark", previous)
	}
	harness.clock.Advance(time.Second)
	harness.wantFilm11(t, user, "marked watched: out of Continue Watching, from its start", "true  from 0")

	harness.undo(t, user, previous)
	harness.wantFilm11(t, user, "undone", "false 11:false from 600000")

	harness.mark(t, http.MethodPut, "/items/11/watched", user)
	harness.clock.Advance(time.Second)
	harness.mark(t, http.MethodDelete, "/items/11/watched", user)
	harness.clock.Advance(time.Second)
	harness.wantFilm11(t, user, "marked unwatched, as never watched", "false  from 0")
	// Watching after the mark counts again.
	harness.report(t, user, harness.play(t, user, chrome(2)), reportBody(1, 300_000, 0))
	harness.clock.Advance(time.Second)
	if got, want := harness.homeIDs(t, user), "11:false"; got != want {
		t.Errorf("watched again after the mark: Continue Watching %q; want %q", got, want)
	}
}

// An extra is never marked watched (D370 §2), and an undo cannot put a mark in the future.
func TestWatched_AnExtraIsNeverMarkedAndAnUndoCannotBeOfTheFuture(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelve(t)
	librarytest.Title(t, harness.database, librarytest.TitleSpec{ID: 12, Library: 1, Kind: "extra"})
	librarytest.Version(t, harness.database, librarytest.VersionSpec{ID: 12, TitleID: 12})
	wantError(t, harness.do(t, http.MethodPut, "/items/12/watched", user, nil), http.StatusUnprocessableEntity,
		api.CodeInvalidRequest)
	future := harness.clock.Now().Add(time.Hour).UnixMilli()
	wantError(t, harness.do(t, http.MethodPut, "/me/watched", user, map[string]any{
		"states": []map[string]any{{"itemId": 10, "watchedAt": future}},
	}), http.StatusUnprocessableEntity, api.CodeInvalidRequest)
}

// seriesWatched is a series' page in short: whether it is watched, each season's number and whether
// it is, and its Next Up's title, 0 for none.
func (harness testHarness) seriesWatched(t *testing.T, token string) string {
	t.Helper()
	var series struct {
		IsWatched bool `json:"isWatched"`
		Seasons   []struct {
			SeasonNumber int64 `json:"seasonNumber"`
			IsWatched    bool  `json:"isWatched"`
		} `json:"seasons"`
		NextUp *struct {
			TitleID int64 `json:"itemId"`
		} `json:"nextUp"`
	}
	harness.readJSON(t, "/items/20", token, &series)
	seasons := []string{}
	for _, season := range series.Seasons {
		seasons = append(seasons, fmt.Sprintf("%d:%t", season.SeasonNumber, season.IsWatched))
	}
	next := int64(0)
	if series.NextUp != nil {
		next = series.NextUp.TitleID
	}
	return fmt.Sprintf("%t %s next %d", series.IsWatched, strings.Join(seasons, ","), next)
}

// A season and a whole series are marked by hand on their pages; a season or a series shows the mark
// once every episode in it is watched, its episodes' rows each theirs, and its progress and Next Up
// read the marks; Undo puts the season back (D370 §3, §4, §5).
func TestWatched_ASeasonOrASeriesIsMarkedByHandAndCheckedOnceEveryEpisodeIs(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveDVDOrder(t)
	series := "/items?kind=series&sort=name&limit=10"
	if got, want := harness.seriesWatched(t, user), "false 1:false,2:false next 21"; got != want {
		t.Errorf("nothing watched: %s; want %s", got, want)
	}
	previous := harness.mark(t, http.MethodPut, "/items/20/groups/1/watched", user)
	if len(previous) != 3 {
		t.Errorf("season 1 marked: previous %+v; want its three episodes", previous)
	}
	if got, want := harness.seriesWatched(t, user), "false 1:true,2:false next 24"; got != want {
		t.Errorf("season 1 marked: %s; want %s", got, want)
	}
	if got, want := harness.titlesProgress(t, series, user), "20:0:3/4"; got != want {
		t.Errorf("season 1 marked: the list %s; want %s", got, want)
	}
	var season struct {
		Episodes []api.EpisodeBody `json:"episodes"`
	}
	harness.readJSON(t, "/items/20/groups/1?limit=10", user, &season)
	for _, episode := range season.Episodes {
		if !episode.IsWatched {
			t.Errorf("season 1 marked: its episode %d's row is not watched", episode.ID)
		}
	}
	harness.undo(t, user, previous)
	if got, want := harness.seriesWatched(t, user), "false 1:false,2:false next 21"; got != want {
		t.Errorf("season 1's mark undone: %s; want %s", got, want)
	}
	harness.mark(t, http.MethodPut, "/items/20/watched", user)
	if got, want := harness.seriesWatched(t, user), "true 1:true,2:true next 0"; got != want {
		t.Errorf("the series marked: %s; want %s", got, want)
	}
	var listed struct {
		Items []api.ItemEntryBody `json:"items"`
	}
	harness.readJSON(t, series, user, &listed)
	if len(listed.Items) != 1 || !listed.Items[0].IsWatched {
		t.Errorf("the series marked: the list %+v; want it watched", listed.Items)
	}
	harness.mark(t, http.MethodDelete, "/items/20/groups/2/watched", user)
	if got, want := harness.seriesWatched(t, user), "false 1:true,2:false next 24"; got != want {
		t.Errorf("season 2 marked unwatched: %s; want %s", got, want)
	}
	harness.mark(t, http.MethodDelete, "/items/20/watched", user)
	if got, want := harness.seriesWatched(t, user), "false 1:false,2:false next 21"; got != want {
		t.Errorf("the series marked unwatched: %s; want %s", got, want)
	}
}

// Starting a finished episode again leaves it watched: the series' progress and its Next Up do not
// step back, as they did while finished was read from the position alone (D370 §5).
func TestWatched_RestartingAFinishedEpisodeKeepsTheSeriesWhereItWas(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveDVDOrder(t)
	harness.setPositionReadAt(t, 2, 1000, 5) // 21, finished
	harness.setPositionReadAt(t, 3, 1000, 6) // 22, finished
	harness.setPositionReadAt(t, 2, 30, 7)   // 21, started again
	series := "/items?kind=series&sort=name&limit=10"
	if got, want := harness.titlesProgress(t, series, user), "20:0:2/4"; got != want {
		t.Errorf("the first restarted: %s; want %s, both still finished", got, want)
	}
	if got, want := harness.seriesWatched(t, user), "false 1:false,2:false next 23"; got != want {
		t.Errorf("the first restarted: %s; want %s, Next Up after the second", got, want)
	}
}

// Marking a series or a season is answered as one that does not exist to a person whose libraries do
// not reach it (D243 §4).
func TestWatched_ASeriesNotReachedIsNoSuchSeriesToMark(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelveDVDOrder(t)
	made := harness.do(t, http.MethodPost, "/roles", admin, map[string]any{
		"name": "watchers", "capabilities": []string{}, "libraries": []any{},
	})
	if made.status != http.StatusCreated {
		t.Fatalf("POST /roles = %d %s", made.status, made.body)
	}
	if _, err := harness.authService.CreateUser(t.Context(),
		auth.NewUser{Name: "partner", Password: "partner-password", Role: "watchers"}); err != nil {
		t.Fatal(err)
	}
	partner := harness.signIn(t, "partner", "partner-password")
	harness.giveRoleLibraries(t, admin, "watchers", 1)
	harness.mark(t, http.MethodPut, "/items/20/groups/1/watched", partner)
	harness.giveRoleLibraries(t, admin, "watchers")
	for _, request := range []struct{ method, path string }{
		{http.MethodPut, "/items/20/watched"},
		{http.MethodDelete, "/items/20/watched"},
		{http.MethodPut, "/items/20/groups/1/watched"},
		{http.MethodDelete, "/items/20/groups/1/watched"},
		{http.MethodPut, "/items/21/watched"},
		{http.MethodDelete, "/items/21/watched"},
	} {
		wantError(t, harness.do(t, request.method, request.path, partner, nil), http.StatusNotFound,
			api.CodeNotFound)
	}
}
