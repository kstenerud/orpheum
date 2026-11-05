// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"cmp"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library/librarytest"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// shelveDVDOrder adds to shelve's series three more aired episodes, S1E2 (22), S1E3 (23) and S2E1
// (24), each with a file, and a DVD order, tmdb:dvd, whose disc 1 is 23 then 21 and disc 2 is 24:
// 22 is not in it.
func (harness testHarness) shelveDVDOrder(t *testing.T) {
	t.Helper()
	harness.shelve(t)
	for _, episode := range []struct {
		titleID, fileID int64
		path            string
	}{
		{22, 3, "/m/BB/S01E02.mkv"}, {23, 4, "/m/BB/S01E03.mkv"}, {24, 5, "/m/BB/S02E01.mkv"},
	} {
		librarytest.Title(t, harness.database, librarytest.TitleSpec{
			ID: episode.titleID, Library: 1, Kind: "episode", AddedAt: 2,
		})
		librarytest.Version(t, harness.database,
			librarytest.VersionSpec{ID: episode.fileID, TitleID: episode.titleID, CreatedAt: 2})
		librarytest.File(t, harness.database, librarytest.FileSpec{
			ID: episode.fileID, LibraryID: 1, NodeID: 1, Path: episode.path, SizeBytes: 10,
			PayloadHash: []byte{byte(episode.fileID)}, //nolint:gosec // a test's small id, 3-5
			Filing:      "listed", VersionID: episode.fileID,
		})
		librarytest.Probe(t, harness.database, librarytest.ProbeSpec{FileID: episode.fileID, DurationMs: 1000})
	}
	librarytest.Probe(t, harness.database, librarytest.ProbeSpec{FileID: 2, DurationMs: 1000})
	storetest.ExecRaw(t, harness.databasePath,
		"INSERT INTO item_text (item_id, language, is_original, name) VALUES (22, 'und', 1, 'Cat'), "+
			"(23, 'und', 1, 'Bag'), (24, 'und', 1, 'Seven')",
		storetest.Positions(
			"(1, 22, 1, 2, 10002), (1, 23, 1, 3, 10003), (1, 24, 2, 1, 20001)"),
		`INSERT INTO ordering (id, sequence_id, name, provider, kind, source_id, label, listed_count)
		 VALUES (2, 20, 'tmdb:dvd', 'tmdb', 'dvd', 'dvd', 'DVD Order', 5)`,
		storetest.Positions(
			"(2, 23, 1, 1, 10001), (2, 21, 1, 2, 10002), (2, 24, 2, 1, 20001)"),
		`INSERT INTO provider_response (item_id, provider, part, fetched_at, last_asked_at, body_gzip)
		 VALUES (20, 'tmdb', 'episode_group/dvd', 1, 1, X'')`)
}

// playState is every play position and event, which a choice of ordering must leave as they were.
func (harness testHarness) playState(t *testing.T) []string {
	t.Helper()
	return slices.Concat(
		storetest.QueryRaw(t, harness.databasePath, `SELECT user_id, file_id, position_ms, report_seq, device_id,
		 updated_at, position_read_at FROM play_position ORDER BY user_id, file_id`),
		storetest.QueryRaw(t, harness.databasePath, `SELECT id, user_id, file_id, device_id, kind, position_ms,
		 happened_at, happened_mono_ms FROM play_event ORDER BY id`))
}

// seriesSeen is a series' page as a test reads it: the ordering, its seasons and its Next Up.
type seriesSeen struct {
	Ordering string `json:"ordering"`
	Seasons  []struct {
		SeasonNumber int64  `json:"seasonNumber"`
		EpisodeCount int64  `json:"episodeCount"`
		Name         string `json:"name"`
	} `json:"seasons"`
	NextUp *struct {
		TitleID       int64 `json:"titleId"`
		GroupNumber   int64 `json:"groupNumber"`
		NumberInGroup int64 `json:"numberInGroup"`
	} `json:"nextUp"`
}

// summary is the page in short: "ordering seasons[number:count,...] next title:group:number", its
// ordering aired where the page names none.
func (seen seriesSeen) summary() string {
	seasons := []string{}
	for _, season := range seen.Seasons {
		seasons = append(seasons, fmt.Sprintf("%d:%d", season.SeasonNumber, season.EpisodeCount))
	}
	next := "none"
	if seen.NextUp != nil {
		next = fmt.Sprintf("%d:%d:%d", seen.NextUp.TitleID, seen.NextUp.GroupNumber, seen.NextUp.NumberInGroup)
	}
	return fmt.Sprintf("%s seasons[%s] next %s", cmp.Or(seen.Ordering, "aired"), strings.Join(seasons, ","), next)
}

// readJSON is GET path's answer, read into into; a failed request fails the test.
func (harness testHarness) readJSON(t *testing.T, path, token string, into any) {
	t.Helper()
	answer := harness.do(t, http.MethodGet, path, token, nil)
	if err := json.Unmarshal([]byte(answer.body), into); err != nil || answer.status != http.StatusOK {
		t.Fatalf("GET %s = %d %s (%v)", path, answer.status, answer.body, err)
	}
}

// seriesSummary is GET /items/20's summary.
func (harness testHarness) seriesSummary(t *testing.T, token string) string {
	t.Helper()
	var seen seriesSeen
	harness.readJSON(t, "/items/20", token, &seen)
	return seen.summary()
}

// A person who chose a series' other ordering sees it in that one: its seasons are the ordering's
// groups, a season's page a group's episodes in its order, an episode labelled by its place there
// beside its aired place, here and in Continue Watching, and Next Up the one after the last they
// finished there; and choosing it moves nothing they watched (D159's first assertion; D157). Who
// never chose, or chose one the series no longer has, sees aired.
func TestOrderings_APersonsChosenOrderingIsTheSeriesTheySee(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveDVDOrder(t)
	harness.setPositionReadAt(t, 4, 1000, 9) // 23, aired S1E3 and disc 1's first, finished
	storetest.ExecRaw(t, harness.databasePath, `INSERT INTO play_event (user_id, file_id, kind, position_ms,
		happened_at) VALUES (1, 4, 'completed', 1000, 9)`)
	if got, want := harness.seriesSummary(t, user), "aired seasons[1:3,2:1] next 24:2:1"; got != want {
		t.Errorf("never chosen: %s; want %s", got, want)
	}
	before := harness.playState(t)
	storetest.ExecRaw(t, harness.databasePath, `INSERT INTO preference (user_id, scope, scope_id, axis, value_json,
		set_at) VALUES (1, 'item', 20, 'episode_ordering', '"tmdb:dvd"', 1)`)
	if got, want := harness.seriesSummary(t, user), "tmdb:dvd seasons[1:2,2:1] next 21:1:2"; got != want {
		t.Errorf("chosen: %s; want %s, the DVD order's discs and 21 after 23", got, want)
	}
	var disc struct {
		Episodes []struct {
			ID            int64 `json:"id"`
			EpisodeNumber int64 `json:"episodeNumber"`
		} `json:"episodes"`
	}
	harness.readJSON(t, "/items/20/groups/1?limit=10", user, &disc)
	if got := fmt.Sprint(disc.Episodes); got != "[{23 1} {21 2}]" {
		t.Errorf("disc 1 = %s; want 23 then 21, numbered in the DVD order", got)
	}
	episode := harness.do(t, http.MethodGet, "/items/21", user, nil)
	if !strings.Contains(episode.body, `"episode":{"seriesId":20,"seriesName":"Brass Arena","seasonNumber":1,`+
		`"episodeNumber":1,"ordering":"tmdb:dvd","inOrdering":{"groupNumber":1,"numberInGroup":2}}`) {
		t.Errorf("an episode's detail = %s; want its aired place and its DVD place, disc 1's second", episode.body)
	}
	if left := harness.do(t, http.MethodGet, "/items/22", user, nil); !strings.Contains(left.body,
		`"ordering":"tmdb:dvd"`) || strings.Contains(left.body, "inOrdering") {
		t.Errorf("an episode the order leaves out = %s; want its aired place alone", left.body)
	}
	if after := harness.playState(t); !slices.Equal(after, before) {
		t.Errorf("play state after choosing an ordering\n  %q\nwant as it was\n  %q", after, before)
	}
	harness.setPositionReadAt(t, 2, 500, 10) // 21 partway: Continue Watching labels it in the DVD order
	rows := harness.continueWatching(t, user)
	if len(rows) != 1 || rows[0].Episode == nil || fmt.Sprint(rows[0].Episode.InOrdering) != "&{1 2  false}" {
		t.Errorf("Continue Watching = %+v; want 21, at disc 1's second", rows)
	}
	storetest.ExecRaw(t, harness.databasePath,
		`UPDATE preference SET value_json = '"tmdb:gone"' WHERE axis = 'episode_ordering'`)
	if got, want := harness.seriesSummary(t, user), "aired seasons[1:3,2:1] next 21:1:1"; got != want {
		t.Errorf("a choice the series no longer has: %s; want %s", got, want)
	}
}

// A series' page offers every ordering it has, aired first, each with its kind, its provider's label
// and how many of the series' episodes it holds; a person's choice of one is kept, and one the series
// does not have is refused. In the ordering chosen, its groups carry its names for them, and the
// episodes it leaves out are listed apart at their aired places, never as Next Up
// (episode-orderings.md §4, §7 Q4, Q8).
func TestOrderings_APersonChoosesAmongTheOrderingsASeriesHas(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveDVDOrder(t)
	storetest.ExecRaw(t, harness.databasePath,
		"INSERT INTO group_text (grp_id, language, name) "+
			"SELECT id, 'en', 'Disc 1' FROM grp WHERE ordering_id = 2 AND number = 1")
	var offered struct {
		Orderings []api.OrderingBody `json:"orderings"`
	}
	harness.readJSON(t, "/items/20", user, &offered)
	if got, want := fmt.Sprint(offered.Orderings),
		"[{aired aired  0 4 false} {tmdb:dvd dvd DVD Order 5 3 false}]"; got != want {
		t.Errorf("orderings offered %s; want %s", got, want)
	}
	choose := func(ordering string) response {
		return harness.do(t, http.MethodPut, "/items/20/preferences", user,
			map[string]string{"episodeOrdering": ordering})
	}
	wantError(t, choose("tmdb:none"), http.StatusBadRequest, api.CodeInvalidRequest)
	wantError(t, harness.do(t, http.MethodPut, "/items/99/preferences", user,
		map[string]string{"episodeOrdering": "aired"}), http.StatusNotFound, api.CodeNotFound)
	if got, want := harness.seriesSummary(t, user), "aired seasons[1:3,2:1] next 21:1:1"; got != want {
		t.Errorf("after a refused choice: %s; want %s", got, want)
	}
	if answer := choose("tmdb:dvd"); answer.status != http.StatusNoContent {
		t.Fatalf("choosing the DVD order = %d %s", answer.status, answer.body)
	}
	if got, want := harness.seriesSummary(t, user), "tmdb:dvd seasons[1:2,2:1] next 23:1:1"; got != want {
		t.Errorf("chosen: %s; want %s", got, want)
	}
	series := harness.do(t, http.MethodGet, "/items/20", user, nil)
	for _, want := range []string{
		`{"seasonNumber":1,"name":"Disc 1","episodeCount":2,`, `"notInOrderingCount":1`, `"groupName":"Disc 1"`,
	} {
		if !strings.Contains(series.body, want) {
			t.Errorf("the series in the DVD order = %s; want %s", series.body, want)
		}
	}
	if episode := harness.do(t, http.MethodGet, "/items/21", user, nil); !strings.Contains(episode.body,
		`"inOrdering":{"groupNumber":1,"numberInGroup":2,"groupName":"Disc 1"}`) {
		t.Errorf("an episode's detail = %s; want its place on Disc 1", episode.body)
	}
	var left struct {
		Episodes []struct {
			ID            int64 `json:"id"`
			SeasonNumber  int64 `json:"seasonNumber"`
			EpisodeNumber int64 `json:"episodeNumber"`
		} `json:"episodes"`
	}
	harness.readJSON(t, "/items/20/not-in-order?limit=10", user, &left)
	if got := fmt.Sprint(left.Episodes); got != "[{22 1 2}]" {
		t.Errorf("not in the DVD order: %s; want 22, at its aired S1E2", got)
	}
	// Watching every episode the DVD order holds leaves no Next Up, though 22 is unwatched.
	for _, file := range []int64{2, 4, 5} {
		harness.setPositionReadAt(t, file, 1000, file)
	}
	if got, want := harness.seriesSummary(t, user), "tmdb:dvd seasons[1:2,2:1] next none"; got != want {
		t.Errorf("every episode of the DVD order watched: %s; want %s, never the one it leaves out", got, want)
	}
	if answer := choose("aired"); answer.status != http.StatusNoContent {
		t.Fatalf("choosing aired = %d %s", answer.status, answer.body)
	}
	if got, want := harness.seriesSummary(t, user), "aired seasons[1:3,2:1] next none"; got != want {
		t.Errorf("aired chosen again: %s; want %s", got, want)
	}
	harness.readJSON(t, "/items/20/not-in-order?limit=10", user, &left)
	if len(left.Episodes) != 0 {
		t.Errorf("not in aired: %v; want none", left.Episodes)
	}
	harness.wantSeriesOrderingsUnreached(t, user)
}

// wantSeriesOrderingsUnreached checks that a person whose role does not reach series 20's library is
// answered as for a series that does not exist, choosing its ordering or listing what it leaves out
// (D243 §4).
func (harness testHarness) wantSeriesOrderingsUnreached(t *testing.T, admin string) {
	t.Helper()
	if made := harness.do(t, http.MethodPost, "/roles", admin, map[string]any{
		"name": "strangers", "capabilities": []string{}, "libraries": []any{},
	}); made.status != http.StatusCreated {
		t.Fatalf("POST /roles = %d %s", made.status, made.body)
	}
	if _, err := harness.authService.CreateUser(t.Context(),
		auth.NewUser{Name: "partner", Password: "partner-password", Role: "strangers"}); err != nil {
		t.Fatal(err)
	}
	partner := harness.signIn(t, "partner", "partner-password")
	wantError(t, harness.do(t, http.MethodPut, "/items/20/preferences", partner,
		map[string]string{"episodeOrdering": "tmdb:dvd"}), http.StatusNotFound, api.CodeNotFound)
	wantError(t, harness.do(t, http.MethodGet, "/items/20/not-in-order?limit=10", partner, nil),
		http.StatusNotFound, api.CodeNotFound)
}

// Choosing an episode group no one has fetched yet keeps the choice and queues its fetch, and the
// page says it waits for it, holding none of the series' episodes until then (episode-orderings.md
// §3.4, §4; D170: a first fetch, on a person's act).
func TestOrderings_ChoosingAGroupNotFetchedYetQueuesItsFetch(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelveDVDOrder(t)
	storetest.ExecRaw(t, harness.databasePath,
		`UPDATE item SET followed_source = 'tmdb', followed_source_id = '1396' WHERE id = 20`,
		`INSERT INTO ordering (id, sequence_id, name, provider, kind, source_id, label, listed_count)
		 VALUES (3, 20, 'tmdb:intended', 'tmdb', 'absolute', 'intended', 'Intended Order', 14)`)
	var offered struct {
		Orderings []api.OrderingBody `json:"orderings"`
	}
	harness.readJSON(t, "/items/20", admin, &offered)
	if got := fmt.Sprint(offered.Orderings[2]); got != "{tmdb:intended absolute Intended Order 14 0 true}" {
		t.Errorf("a group not fetched is offered as %s; want it awaiting its fetch, holding none", got)
	}
	if answer := harness.do(t, http.MethodPut, "/items/20/preferences", admin,
		map[string]string{"episodeOrdering": "tmdb:intended"}); answer.status != http.StatusNoContent {
		t.Fatalf("choosing it = %d %s", answer.status, answer.body)
	}
	job := newestOf(t, harness.listedJobs(t, admin), jobs.KindFetchMetadata)
	if job.ItemID != 20 || job.Reason != "an episode ordering a person chose" {
		t.Errorf("queued %+v; want series 20's fetch of the ordering chosen", job)
	}
	if got, want := harness.seriesSummary(t, admin), "tmdb:intended seasons[] next none"; got != want {
		t.Errorf("while it waits: %s; want %s, its episodes all listed apart", got, want)
	}
}
