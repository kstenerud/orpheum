// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/netip"
	"slices"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/library/librarytest"
	"github.com/kstenerud/orpheum/internal/core/textindex"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// shelve adds a library with a film, Metropolis, with one file and its streams, and a series with one
// episode, as the identification pass leaves them.
func (harness testHarness) shelve(t *testing.T) {
	t.Helper()
	sorter, err := textindex.NewSorter("en")
	if err != nil {
		t.Fatal(err)
	}
	key := func(name string) string { return "X'" + hex.EncodeToString(sorter.Key(name)) + "'" }
	librarytest.NodeWithLibrary(t, harness.database,
		librarytest.Library{Name: "Films", Kind: "mixed", MetadataSource: "nfo"})
	storetest.ExecRaw(t, harness.databasePath,
		`INSERT INTO item (id, kind, library_id, added_at)
		 VALUES (10, 'title', 1, 1), (20, 'collection', 1, 2), (21, 'title', 1, 2)`,
		`INSERT INTO title (id, kind, year, identification_state)
		 VALUES (10, 'movie', 1995, 'identified'), (21, 'episode', 2008, 'identified')`,
		`INSERT INTO collection (id, kind, membership, created_by, identification_state)
		 VALUES (20, 'series', 'explicit', 'scanner', 'identified')`,
		`INSERT INTO item_text (item_id, language, is_original, name, overview, search_alternates) VALUES
		 (10, 'und', 1, 'Metropolis', 'A crime saga.', 'heat'), (20, 'und', 1, 'Brass Arena', NULL, 'brass arena'),
		 (21, 'und', 1, 'Pilot', NULL, 'pilot')`,
		fmt.Sprintf(`INSERT INTO sort_key (item_id, locale, listing, collation_key)
		 VALUES (10, 'en', 'movie', %s), (20, 'en', 'series', %s)`,
			key("Metropolis"), key("Brass Arena")),
		"INSERT INTO version (id, title_id, created_at) VALUES (1, 10, 1), (2, 21, 2)",
		`INSERT INTO file (id, library_id, node_id, path, size_bytes, payload_hash, filing, version_id, first_seen_at,
		 last_seen_at) VALUES
		 (1, 1, 1, '/m/Metropolis (1995).mkv', 1000, X'01', 'listed', 1, 1, 1),
		 (2, 1, 1, '/m/BB/S01E01.mkv', 10, X'02', 'listed', 2, 1, 1)`,
		`INSERT INTO stream (bytes_file, video_file, stream_index, kind, codec, language, channel_count) VALUES
		 (1, 1, 0, 'video', 'h264', NULL, NULL), (1, 1, 1, 'audio', 'truehd', 'en', 8)`,
		"INSERT INTO ordering (id, collection_id, name) VALUES (1, 20, 'aired')",
		"INSERT INTO position (ordering_id, title_id, group_number, number_in_group, sort_order) "+
			"VALUES (1, 21, 1, 1, 10001)",
		"INSERT INTO collection_member (collection_id, member_title, position) VALUES (20, 21, 10001)",
		// Metropolis's poster is a person's choice over a newer one TMDB named; its backdrop was refused.
		`INSERT INTO artwork (item_id, kind, source, source_ref, source_sha256, is_chosen, removed_at) VALUES
		 (10, 'poster', 'user', NULL, X'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa', 1, NULL),
		 (10, 'poster', 'tmdb', '/p.jpg', X'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb', 0, NULL),
		 (10, 'backdrop', 'tmdb', '/b.jpg', NULL, 0, NULL),
		 (20, 'poster', 'tmdb', '/old.jpg', X'eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee', 0, 5),
		 (20, 'poster', 'tmdb', '/s.jpg', X'cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc', 0, NULL),
		 (21, 'still', 'tmdb', '/e.jpg', X'dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd', 0, NULL)`)
}

func TestBrowse_ListsDetailsSeriesAndSearch(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelve(t)
	if _, err := harness.authService.CreateUser(t.Context(),
		auth.NewUser{Name: "partner", Password: "partner-password", Role: auth.RoleUser}); err != nil {
		t.Fatal(err)
	}
	user := harness.signIn(t, "partner", "partner-password")

	films := harness.do(t, http.MethodGet, "/titles?kind=movie&sort=name&limit=10", user, nil)
	if !strings.Contains(films.body, `"name":"Metropolis","year":1995`) || strings.Contains(films.body, "Breaking") {
		t.Errorf("films = %d %s", films.status, films.body)
	}
	shows := harness.do(t, http.MethodGet, "/titles?kind=series&sort=added&limit=10", user, nil)
	if !strings.Contains(shows.body, `"name":"Brass Arena"`) {
		t.Errorf("series = %d %s", shows.status, shows.body)
	}
	wantError(t, harness.do(t, http.MethodGet, "/titles?kind=movie&sort=name", user, nil),
		http.StatusUnprocessableEntity, CodeInvalidRequest)

	detail := harness.do(t, http.MethodGet, "/items/10", user, nil)
	if !strings.Contains(detail.body, `"codec":"truehd","language":"en","channelCount":8`) ||
		!strings.Contains(detail.body, `"streams":[{"id":`) ||
		strings.Contains(detail.body, "Metropolis (1995).mkv") {
		t.Errorf("a user's detail = %d %s; want the streams and no path", detail.status, detail.body)
	}
	if adminDetail := harness.do(t, http.MethodGet, "/items/10", admin, nil); !strings.Contains(adminDetail.body,
		`"path":"/m/Metropolis (1995).mkv"`) {
		t.Errorf("an admin's detail = %s; want the path", adminDetail.body)
	}
	// From away, where the admin does not administer, no path (the Plex-chain review's T2).
	harness.server.SetHomeNetworks([]netip.Prefix{netip.MustParsePrefix("192.168.1.0/24")})
	if away := harness.do(t, http.MethodGet, "/items/10", admin, nil); strings.Contains(away.body, "Metropolis (1995).mkv") {
		t.Errorf("an admin's detail from away = %s; want no path", away.body)
	}
	harness.server.SetHomeNetworks([]netip.Prefix{
		netip.MustParsePrefix("127.0.0.0/8"), netip.MustParsePrefix("::1/128"),
	})
	episode := harness.do(t, http.MethodGet, "/items/21", user, nil)
	if !strings.Contains(episode.body,
		`"episode":{"seriesId":20,"seriesName":"Brass Arena","seasonNumber":1,"episodeNumber":1}`) {
		t.Errorf("an episode's detail = %s", episode.body)
	}
	wantError(t, harness.do(t, http.MethodGet, "/items/99", user, nil), http.StatusNotFound, CodeNotFound)

	series := harness.do(t, http.MethodGet, "/collections/20", user, nil)
	if !strings.Contains(series.body, `"seasons":[{"seasonNumber":1,"episodeCount":1`) {
		t.Errorf("a series = %d %s; want its one season of one episode", series.status, series.body)
	}
	season := harness.do(t, http.MethodGet, "/collections/20/seasons/1?limit=10", user, nil)
	if !strings.Contains(season.body, `"name":"Pilot"`) || !strings.Contains(season.body, `"hasLiveFile":true`) {
		t.Errorf("a season = %d %s", season.status, season.body)
	}

	found := harness.do(t, http.MethodGet, "/search?query=breaking&limit=5", user, nil)
	if !strings.Contains(found.body, `{"id":20,"kind":"series","name":"Brass Arena"}`) {
		t.Errorf("search = %d %s", found.status, found.body)
	}
}

func TestBrowse_EachItemShowsItsChosenOrNewestStoredImage(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelve(t)
	chosen, seriesPoster, still := strings.Repeat("aa", 32), strings.Repeat("cc", 32), strings.Repeat("dd", 32)
	for _, check := range []struct {
		path, want string
	}{
		{"/titles?kind=movie&sort=name&limit=10", `"posterSha256":"` + chosen + `"`},
		{"/titles?kind=series&sort=added&limit=10", `"posterSha256":"` + seriesPoster + `"`},
		{"/items/10", `"art":{"posterSha256":"` + chosen + `"}`}, // and no refused backdrop
		{"/collections/20", `"stillSha256":"` + still + `"`},     // its season's, its first episode's
		{"/collections/20/seasons/1?limit=10", `"stillSha256":"` + still + `"`},
		{"/collections/20", `"art":{"posterSha256":"` + seriesPoster + `"}`},
	} {
		if answer := harness.do(t, http.MethodGet, check.path, user, nil); !strings.Contains(answer.body, check.want) {
			t.Errorf("%s = %d %s; want %s", check.path, answer.status, answer.body, check.want)
		}
	}
}

// A title none of whose files can be read now stays listed, marked unavailable with why and since
// when: in the lists, on its page and its series', and for its episode (D241 §3).
func TestBrowse_ATitleThatCannotBeReadNowIsListedAndMarkedUnavailable(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelve(t)
	storetest.ExecRaw(t, harness.databasePath,
		"UPDATE file SET reported_path_raw = CAST(path AS BLOB)",
		`INSERT INTO root_state (node_id, root_path, last_scan_outcome, last_scan_at)
		 VALUES (1, '/m', 'unreadable', 5000)`,
		`INSERT INTO unavailable_place (node_id, path, root_path, since, cause)
		 VALUES (1, '/m', '/m', 4000, 'unread')`)
	const missing = `"unavailable":{"reason":"root_missing","nodeName":"local","path":"/m","unavailableAt":4000}`
	for _, path := range []string{
		"/titles?kind=movie&sort=name&limit=10", "/titles?kind=series&sort=added&limit=10", "/items/10",
		"/items/21", "/collections/20", "/collections/20/seasons/1?limit=10",
	} {
		if got := harness.do(t, http.MethodGet, path, user, nil); got.status != http.StatusOK ||
			!strings.Contains(got.body, missing) {
			t.Errorf("GET %s under a missing root = %d %s; want it there, marked unavailable", path, got.status,
				got.body)
		}
	}
	if season := harness.do(t, http.MethodGet, "/collections/20/seasons/1?limit=10", user,
		nil); !strings.Contains(season.body, `"name":"Pilot"`) || strings.Count(season.body, missing) != 1 {
		t.Errorf("the season = %s; want its episode marked", season.body)
	}
	storetest.ExecRaw(t, harness.databasePath, "DELETE FROM root_state", "DELETE FROM unavailable_place")
	harness.nodes.isAway.Store(true)
	away := harness.do(t, http.MethodGet, "/items/10", user, nil)
	if !strings.Contains(away.body, `"unavailable":{"reason":"node_away","nodeName":"local"`) {
		t.Errorf("a title whose node is away = %s; want it marked so", away.body)
	}
	harness.nodes.isAway.Store(false)
	if back := harness.do(t, http.MethodGet, "/items/10", user, nil); strings.Contains(back.body, `"unavailable"`) {
		t.Errorf("a title that can be read = %s; want it unmarked", back.body)
	}
}

// A film shown from its listing whose file is not read yet is listed, and marked still reading in
// the lists and on its page, its series and its episode too; once read it is unmarked (D304 §4).
func TestBrowse_ATitleStillBeingReadIsListedAndMarkedSo(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelve(t)
	storetest.ExecRaw(t, harness.databasePath,
		"UPDATE file SET reported_path_raw = CAST(path AS BLOB), payload_hash = NULL")
	const reading = `"unavailable":{"reason":"still_reading","nodeName":"local"}`
	for _, path := range []string{
		"/titles?kind=movie&sort=name&limit=10", "/items/10", "/collections/20", "/collections/20/seasons/1?limit=10",
	} {
		if got := harness.do(t, http.MethodGet, path, user, nil); got.status != http.StatusOK ||
			!strings.Contains(got.body, reading) {
			t.Errorf("GET %s still being read = %d %s; want it there, marked still reading", path, got.status, got.body)
		}
	}
	storetest.ExecRaw(t, harness.databasePath, "UPDATE file SET payload_hash = X'01'")
	if read := harness.do(t, http.MethodGet, "/items/10", user, nil); strings.Contains(read.body, `"unavailable"`) {
		t.Errorf("a title read = %s; want it unmarked", read.body)
	}
}

// A series' page lists its seasons, numbered in order and the specials last, each with how many
// episodes it has; a season's episodes come a page at a time, in aired order, so no answer grows
// with a series however damaged its library (D306 §9; the owner's NAS, 2026-10-16: 80,269 episodes in
// one answer).
func TestBrowse_ASeriesListsItsSeasonsAndASeasonComesAPageAtATime(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelve(t)
	statements := []string{}
	for index, episode := range []struct{ season, number int }{{1, 2}, {1, 3}, {0, 1}, {2, 1}, {0, 0}} {
		id := 30 + index
		statements = append(statements,
			fmt.Sprintf("INSERT INTO item (id, kind, library_id, added_at) VALUES (%d, 'title', 1, 2)", id),
			fmt.Sprintf("INSERT INTO title (id, kind, identification_state) VALUES (%d, 'episode', 'identified')", id),
			fmt.Sprintf("INSERT INTO version (title_id, created_at) VALUES (%d, 2)", id),
			fmt.Sprintf("INSERT INTO item_text (item_id, language, is_original, name) VALUES (%d, 'und', 1, 'E%d')", id,
				id),
			fmt.Sprintf(`INSERT INTO position (ordering_id, title_id, group_number, number_in_group, sort_order)
			 VALUES (1, %d, %d, %d, %d)`, id, episode.season, episode.number, 10001+index+1))
	}
	storetest.ExecRaw(t, harness.databasePath, statements...)
	series := harness.do(t, http.MethodGet, "/collections/20", user, nil)
	if !strings.Contains(series.body, `"seasons":[{"seasonNumber":1,"episodeCount":3,`) ||
		!strings.Contains(series.body, `{"seasonNumber":2,"episodeCount":1},{"seasonNumber":0,"episodeCount":2}]`) ||
		strings.Contains(series.body, `"episodes"`) {
		t.Errorf("the series = %s; want seasons 1, 2 and the specials, counted, and no episodes", series.body)
	}
	first := harness.do(t, http.MethodGet, "/collections/20/seasons/1?limit=2", user, nil)
	var page struct {
		Episodes []struct {
			Name string `json:"name"`
		} `json:"episodes"`
		NextCursor string `json:"nextCursor"`
	}
	if err := json.Unmarshal([]byte(first.body), &page); err != nil || len(page.Episodes) != 2 ||
		page.Episodes[0].Name != "Pilot" || page.Episodes[1].Name != "E30" || page.NextCursor == "" {
		t.Fatalf("season 1's first page = %s (%v); want Pilot and E30, and a cursor", first.body, err)
	}
	next := harness.do(t, http.MethodGet, "/collections/20/seasons/1?limit=2&cursor="+page.NextCursor, user, nil)
	if !strings.Contains(next.body, `"name":"E31"`) || strings.Contains(next.body, "nextCursor") {
		t.Errorf("season 1's next page = %s; want E31, the last", next.body)
	}
	if specials := harness.do(t, http.MethodGet, "/collections/20/seasons/0?limit=10", user,
		nil); !strings.Contains(specials.body, `"name":"E32"`) || !strings.Contains(specials.body, `"name":"E34"`) {
		t.Errorf("the specials = %s; want E32 and E34", specials.body)
	}
	wantError(t, harness.do(t, http.MethodGet, "/collections/20/seasons/1?limit=2&cursor=nope", user, nil),
		http.StatusUnprocessableEntity, CodeInvalidRequest)
	wantError(t, harness.do(t, http.MethodGet, "/collections/99/seasons/1?limit=2", user, nil),
		http.StatusNotFound, CodeNotFound)
}

// A film or series whose match is a guess says so in the lists and on its page, for its poster's
// mark; one no guess settled says nothing of it (D301 §4).
func TestBrowse_AGuessedMatchSaysSoWhereItsTitleIsShown(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelve(t)
	storetest.ExecRaw(t, harness.databasePath, "INSERT INTO guessed_match (item_id, guessed_at) VALUES (20, 5)")
	films := harness.do(t, http.MethodGet, "/titles?kind=movie&sort=name&limit=10", admin, nil)
	if strings.Contains(films.body, "isGuessed") {
		t.Errorf("films = %s; Metropolis's match is no guess", films.body)
	}
	for _, path := range []string{"/titles?kind=series&sort=name&limit=10", "/titles?kind=series&sort=added&limit=10"} {
		if shows := harness.do(t, http.MethodGet, path, admin, nil); !strings.Contains(shows.body, `"isGuessed":true`) {
			t.Errorf("%s = %s; want Brass Arena's guess said", path, shows.body)
		}
	}
	if series := harness.do(t, http.MethodGet, "/collections/20", admin, nil); !strings.Contains(series.body,
		`"isGuessed":true`) {
		t.Errorf("the series' page = %s; want its guess said", series.body)
	}
	if detail := harness.do(t, http.MethodGet, "/items/10", admin, nil); strings.Contains(detail.body, "isGuessed") {
		t.Errorf("Metropolis's page = %s; its match is no guess", detail.body)
	}
	storetest.ExecRaw(t, harness.databasePath, "INSERT INTO guessed_match (item_id, guessed_at) VALUES (10, 6)")
	for _, path := range []string{
		"/titles?kind=movie&sort=name&limit=10", "/titles?kind=movie&sort=added&limit=10",
		"/items/10",
	} {
		if got := harness.do(t, http.MethodGet, path, admin, nil); !strings.Contains(got.body, `"isGuessed":true`) {
			t.Errorf("%s = %s; want Metropolis's guess said", path, got.body)
		}
	}
}

// Every reason a file cannot be read now is one the schema names, wherever the API says one: a
// reason the library gives that the schema lacks reaches a client its types do not know (D304 §4).
func TestSchema_NamesEveryReasonAFileCannotBeReadNow(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	schemas := harness.server.OpenAPI().Components.Schemas.Map()
	for _, name := range []string{"UnavailableBody", "MediaUnreachableDetailBody"} {
		schema, found := schemas[name]
		if !found {
			t.Fatalf("the schema has no %s", name)
		}
		var named []string
		for _, value := range schema.Properties["reason"].Enum {
			named = append(named, fmt.Sprint(value))
		}
		for _, reason := range library.UnreadableReasons() {
			if !slices.Contains(named, string(reason)) {
				t.Errorf("%s's reasons %v lack %q", name, named, reason)
			}
		}
	}
}
