// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/netip"
	"slices"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/home"
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
		librarytest.Library{Name: "Films", Kind: "movie", MetadataSource: "nfo", Root: "/m"})
	librarytest.Title(t, harness.database, librarytest.TitleSpec{ID: 10, Library: 1, Year: 1995, AddedAt: 1})
	librarytest.Item(t, harness.database, librarytest.ItemSpec{
		ID: 20, Kind: "series", LibraryID: 1, AddedAt: 2, IdentificationState: "identified",
	})
	librarytest.Title(t, harness.database, librarytest.TitleSpec{
		ID: 21, Library: 1, Kind: "episode", Year: 2008, AddedAt: 2,
	})
	librarytest.Version(t, harness.database, librarytest.VersionSpec{ID: 1, TitleID: 10})
	librarytest.Version(t, harness.database, librarytest.VersionSpec{ID: 2, TitleID: 21, CreatedAt: 2})
	librarytest.File(t, harness.database, librarytest.FileSpec{
		ID: 1, LibraryID: 1, NodeID: 1, Path: "/m/Metropolis (1927).mkv", SizeBytes: 1000, PayloadHash: []byte{0x01},
		Filing: "listed", VersionID: 1,
	})
	librarytest.File(t, harness.database, librarytest.FileSpec{
		ID: 2, LibraryID: 1, NodeID: 1, Path: "/m/BB/S01E01.mkv", SizeBytes: 10, PayloadHash: []byte{0x02},
		Filing: "listed", VersionID: 2,
	})
	librarytest.Stream(t, harness.database,
		librarytest.StreamSpec{BytesFile: 1, StreamIndex: 0, Kind: "video", Codec: "h264"})
	librarytest.Stream(t, harness.database, librarytest.StreamSpec{
		BytesFile: 1, StreamIndex: 1, Kind: "audio", Codec: "truehd", Language: "en", ChannelCount: 8,
	})
	storetest.ExecRaw(t, harness.databasePath,
		"INSERT INTO sequence (id, kind, created_by) VALUES (20, 'series', 'scanner')",
		`INSERT INTO item_text (item_id, language, is_original, name, overview, search_alternates) VALUES
		 (10, 'und', 1, 'Metropolis', 'A crime saga.', 'metropolis'),
		 (20, 'und', 1, 'Brass Arena', NULL, 'brass arena'),
		 (21, 'und', 1, 'Pilot', NULL, 'pilot')`,
		fmt.Sprintf(`INSERT INTO sort_key (item_id, locale, listing, collation_key)
		 VALUES (10, 'en', 'movie', %s), (20, 'en', 'series', %s)`,
			key("Metropolis"), key("Brass Arena")),
		"INSERT INTO ordering (id, sequence_id, name, kind) VALUES (1, 20, 'aired', 'aired')",
		storetest.Positions(
			"(1, 21, 1, 1, 10001)"),
		// Metropolis's poster is a person's choice over a newer one TMDB named; its backdrop was refused.
		`INSERT INTO artwork (item_id, kind, source, source_ref, source_sha256, is_chosen, removed_at) VALUES
		 (10, 'poster', 'user', NULL, X'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa', 1, NULL),
		 (10, 'poster', 'tmdb', '/p.jpg', X'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb', 0, NULL),
		 (10, 'backdrop', 'tmdb', '/b.jpg', NULL, 0, NULL),
		 (20, 'poster', 'tmdb', '/old.jpg', X'eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee', 0, 5),
		 (20, 'poster', 'tmdb', '/s.jpg', X'cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc', 0, NULL),
		 (21, 'still', 'tmdb', '/e.jpg', X'dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd', 0, NULL)`)
}

// checkDetailsPaths checks a title's detail, its film found at a second place too, a copy of its file
// (D363 §2): a user's has its streams and no path; an admin's every place (§8).
func checkDetailsPaths(t *testing.T, harness testHarness, user, admin string) {
	t.Helper()
	librarytest.File(t, harness.database, librarytest.FileSpec{
		LibraryID: 1, NodeID: 1, Path: "/m/Copy/Metropolis (1927).mkv", Filing: "copy", CopyOf: 1, FirstSeenAt: 2,
	})
	detail := harness.do(t, http.MethodGet, "/items/10", user, nil)
	if !strings.Contains(detail.body, `"codec":"truehd","language":"en","channelCount":8`) ||
		!strings.Contains(detail.body, `"streams":[{"id":`) ||
		strings.Contains(detail.body, "Metropolis (1927).mkv") || strings.Contains(detail.body, "otherPaths") {
		t.Errorf("a user's detail = %d %s; want the streams and no path", detail.status, detail.body)
	}
	if adminDetail := harness.do(t, http.MethodGet, "/items/10", admin, nil); !strings.Contains(adminDetail.body,
		`"path":"/m/Metropolis (1927).mkv","otherPaths":["/m/Copy/Metropolis (1927).mkv"]`) {
		t.Errorf("an admin's detail = %s; want the path and the copy's (D363 §8)", adminDetail.body)
	}
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

	films := harness.do(t, http.MethodGet, "/items?kind=film&sort=name&limit=10", user, nil)
	if !strings.Contains(films.body, `"name":"Metropolis","year":1995`) || strings.Contains(films.body, "Brass") {
		t.Errorf("films = %d %s", films.status, films.body)
	}
	// The first page says the whole list's total, so a client sizes it before the rest comes (the
	// owner, 2026-11-11).
	if !strings.Contains(films.body, `"total":1`) {
		t.Errorf("films = %s; want the list's total, its one film", films.body)
	}
	shows := harness.do(t, http.MethodGet, "/items?kind=series&sort=added&limit=10", user, nil)
	if !strings.Contains(shows.body, `"name":"Brass Arena"`) {
		t.Errorf("series = %d %s", shows.status, shows.body)
	}
	wantError(t, harness.do(t, http.MethodGet, "/items?kind=film&sort=name", user, nil),
		http.StatusUnprocessableEntity, api.CodeInvalidRequest)

	checkDetailsPaths(t, harness, user, admin)
	// From away, where the admin does not administer, no path (the Plex-chain review's T2).
	harness.server.SetHome(home.Listed(netip.MustParsePrefix("192.168.1.0/24")))
	away := harness.do(t, http.MethodGet, "/items/10", admin, nil)
	if strings.Contains(away.body, "Metropolis (1927).mkv") {
		t.Errorf("an admin's detail from away = %s; want no path", away.body)
	}
	harness.server.SetHome(home.Listed([]netip.Prefix{
		netip.MustParsePrefix("127.0.0.0/8"), netip.MustParsePrefix("::1/128"),
	}...))
	episode := harness.do(t, http.MethodGet, "/items/21", user, nil)
	if !strings.Contains(episode.body,
		`"episode":{"seriesId":20,"seriesName":"Brass Arena","seasonNumber":1,"episodeNumber":1}`) {
		t.Errorf("an episode's detail = %s", episode.body)
	}
	// The libraries it is listed in, which its page goes up to (D367 §3).
	if !strings.Contains(episode.body, `"libraries":[{"id":1,"name":`) {
		t.Errorf("an episode's detail = %s; want the library listing it", episode.body)
	}
	wantError(t, harness.do(t, http.MethodGet, "/items/99", user, nil), http.StatusNotFound, api.CodeNotFound)

	series := harness.do(t, http.MethodGet, "/items/20", user, nil)
	// One item answers every kind, the series by its kind (D416 §2).
	if !strings.Contains(series.body, `"kind":"series"`) ||
		!strings.Contains(series.body, `"seasons":[{"seasonNumber":1,"episodeCount":1`) {
		t.Errorf("a series = %d %s; want its one season of one episode", series.status, series.body)
	}
	if !strings.Contains(series.body, `"libraries":[{"id":1,"name":`) {
		t.Errorf("a series = %s; want the library listing its episode", series.body)
	}
	season := harness.do(t, http.MethodGet, "/items/20/groups/1?limit=10", user, nil)
	if !strings.Contains(season.body, `"name":"Pilot"`) || !strings.Contains(season.body, `"hasLiveFile":true`) {
		t.Errorf("a season = %d %s", season.status, season.body)
	}

	found := harness.do(t, http.MethodGet, "/search?query=brass&limit=5", user, nil)
	if !strings.Contains(found.body, `{"id":20,"kind":"series","name":"Brass Arena"`) {
		t.Errorf("search = %d %s", found.status, found.body)
	}
}

// A list by name from a letter, the alphabet rail's, starts at the first title at or after it (the
// owner, 2026-10-25); from is a list by name's start, and newest first, or after a cursor, refused.
func TestBrowse_AListByNameFromALetterStartsAtItsFirstTitle(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelve(t)
	if fromH := harness.do(t, http.MethodGet, "/items?kind=film&sort=name&from=H&limit=10", admin,
		nil); !strings.Contains(fromH.body, `"name":"Metropolis"`) {
		t.Errorf("films from H = %d %s", fromH.status, fromH.body)
	}
	if fromN := harness.do(t, http.MethodGet, "/items?kind=film&sort=name&from=N&limit=10", admin,
		nil); fromN.status != http.StatusOK || strings.Contains(fromN.body, "Metropolis") {
		t.Errorf("films from N = %d %s; want none, Metropolis before it", fromN.status, fromN.body)
	}
	wantError(t, harness.do(t, http.MethodGet, "/items?kind=film&sort=added&from=H&limit=10", admin, nil),
		http.StatusUnprocessableEntity, api.CodeInvalidRequest)
	wantError(t, harness.do(t, http.MethodGet, "/items?kind=film&sort=name&from=H&cursor=abc&limit=10", admin,
		nil), http.StatusUnprocessableEntity, api.CodeInvalidRequest)
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
		{"/items?kind=film&sort=name&limit=10", `"posterSha256":"` + chosen + `"`},
		{"/items?kind=series&sort=added&limit=10", `"posterSha256":"` + seriesPoster + `"`},
		{"/items/10", `"art":{"posterSha256":"` + chosen + `"}`}, // and no refused backdrop
		{"/items/20", `"stillSha256":"` + still + `"`},           // its season's, its first episode's
		{"/items/20/groups/1?limit=10", `"stillSha256":"` + still + `"`},
		{"/items/20", `"art":{"posterSha256":"` + seriesPoster + `"}`},
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
		"UPDATE place SET reported_path_raw = CAST(path AS BLOB)",
		`INSERT INTO root_state (node_id, root_path, last_scan_outcome, last_scan_at)
		 VALUES (1, '/m', 'unreadable', 5000)`,
		`INSERT INTO unavailable_place (node_id, path, root_path, since, cause)
		 VALUES (1, '/m', '/m', 4000, 'unread')`)
	const missing = `"unavailable":{"reason":"rootMissing","nodeName":"local","path":"/m","unavailableAt":4000}`
	for _, path := range []string{
		"/items?kind=film&sort=name&limit=10", "/items?kind=series&sort=added&limit=10", "/items/10",
		"/items/21", "/items/20", "/items/20/groups/1?limit=10",
	} {
		if got := harness.do(t, http.MethodGet, path, user, nil); got.status != http.StatusOK ||
			!strings.Contains(got.body, missing) {
			t.Errorf("GET %s under a missing root = %d %s; want it there, marked unavailable", path, got.status,
				got.body)
		}
	}
	if season := harness.do(t, http.MethodGet, "/items/20/groups/1?limit=10", user,
		nil); !strings.Contains(season.body, `"name":"Pilot"`) || strings.Count(season.body, missing) != 1 {
		t.Errorf("the season = %s; want its episode marked", season.body)
	}
	storetest.ExecRaw(t, harness.databasePath, "DELETE FROM root_state", "DELETE FROM unavailable_place")
	harness.nodes.isAway.Store(true)
	away := harness.do(t, http.MethodGet, "/items/10", user, nil)
	if !strings.Contains(away.body, `"unavailable":{"reason":"nodeAway","nodeName":"local"`) {
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
		"UPDATE place SET reported_path_raw = CAST(path AS BLOB)", "UPDATE file SET payload_hash = NULL")
	const reading = `"unavailable":{"reason":"stillReading","nodeName":"local"}`
	for _, path := range []string{
		"/items?kind=film&sort=name&limit=10", "/items/10", "/items/20", "/items/20/groups/1?limit=10",
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
		id := int64(30 + index)
		librarytest.Title(t, harness.database, librarytest.TitleSpec{ID: id, Library: 1, Kind: "episode", AddedAt: 2})
		librarytest.Version(t, harness.database, librarytest.VersionSpec{TitleID: id, CreatedAt: 2})
		statements = append(statements,
			fmt.Sprintf("INSERT INTO item_text (item_id, language, is_original, name) VALUES (%d, 'und', 1, 'E%d')", id,
				id),
			storetest.Positions(fmt.Sprintf(`(1, %d, %d, %d, %d)`, id, episode.season, episode.number, 10001+index+1)))
	}
	storetest.ExecRaw(t, harness.databasePath, statements...)
	series := harness.do(t, http.MethodGet, "/items/20", user, nil)
	if !strings.Contains(series.body, `"seasons":[{"seasonNumber":1,"episodeCount":3,`) ||
		!strings.Contains(series.body, `{"seasonNumber":2,"episodeCount":1},{"seasonNumber":0,"episodeCount":2}]`) ||
		strings.Contains(series.body, `"episodes"`) {
		t.Errorf("the series = %s; want seasons 1, 2 and the specials, counted, and no episodes", series.body)
	}
	first := harness.do(t, http.MethodGet, "/items/20/groups/1?limit=2", user, nil)
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
	next := harness.do(t, http.MethodGet, "/items/20/groups/1?limit=2&cursor="+page.NextCursor, user, nil)
	if !strings.Contains(next.body, `"name":"E31"`) || strings.Contains(next.body, "nextCursor") {
		t.Errorf("season 1's next page = %s; want E31, the last", next.body)
	}
	if specials := harness.do(t, http.MethodGet, "/items/20/groups/0?limit=10", user,
		nil); !strings.Contains(specials.body, `"name":"E32"`) || !strings.Contains(specials.body, `"name":"E34"`) {
		t.Errorf("the specials = %s; want E32 and E34", specials.body)
	}
	wantError(t, harness.do(t, http.MethodGet, "/items/20/groups/1?limit=2&cursor=nope", user, nil),
		http.StatusUnprocessableEntity, api.CodeInvalidRequest)
	wantError(t, harness.do(t, http.MethodGet, "/items/99/groups/1?limit=2", user, nil),
		http.StatusNotFound, api.CodeNotFound)
}

// pagedEpisodeIDs are the episodes of path, a list of a series' episodes, limit at a time, each
// page after the last's nextCursor, to the page with none.
func (harness testHarness) pagedEpisodeIDs(t *testing.T, path, token string, limit int) []int64 {
	t.Helper()
	ids, cursor := []int64{}, ""
	for range 100 {
		var page struct {
			Episodes []struct {
				ID int64 `json:"id"`
			} `json:"episodes"`
			NextCursor string `json:"nextCursor"`
		}
		harness.readJSON(t, fmt.Sprintf("%s?limit=%d%s", path, limit, cursor), token, &page)
		for _, episode := range page.Episodes {
			ids = append(ids, episode.ID)
		}
		if page.NextCursor == "" {
			return ids
		}
		cursor = "&cursor=" + page.NextCursor
	}
	t.Fatalf("%s never ended", path)
	return nil
}

// A season's episodes, and those an ordering leaves out, are paged by their sort order and then
// their id, as every list's cursor is (D416 §6): episodes at the same place cross a page's edge
// with none repeated and none skipped.
func TestBrowse_EpisodesAtTheSamePlaceArePagedWithNoRepeatAndNoSkip(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveDVDOrder(t)
	rows := []string{}
	for id := int64(30); id < 35; id++ {
		librarytest.Title(t, harness.database, librarytest.TitleSpec{ID: id, Library: 1, Kind: "episode", AddedAt: 2})
		librarytest.Version(t, harness.database, librarytest.VersionSpec{TitleID: id, CreatedAt: 2})
		rows = append(rows, fmt.Sprintf("(1, %d, 3, 1, 30001)", id))
	}
	storetest.ExecRaw(t, harness.databasePath, storetest.Positions(strings.Join(rows, ", ")))
	want := []int64{30, 31, 32, 33, 34}
	for _, limit := range []int{1, 2, 3, 100} {
		if got := harness.pagedEpisodeIDs(t, "/items/20/groups/3", user, limit); !slices.Equal(got, want) {
			t.Errorf("season 3 a page of %d at a time: %v; want %v, each once", limit, got, want)
		}
	}
	if chose := harness.do(t, http.MethodPut, "/items/20/preferences", user,
		map[string]string{"episodeOrdering": "tmdb:dvd"}); chose.status != http.StatusNoContent {
		t.Fatalf("choosing the DVD order = %d %s", chose.status, chose.body)
	}
	want = []int64{22, 30, 31, 32, 33, 34}
	for _, limit := range []int{1, 2, 4, 100} {
		if got := harness.pagedEpisodeIDs(t, "/items/20/not-in-order", user, limit); !slices.Equal(got, want) {
			t.Errorf("not in the DVD order a page of %d at a time: %v; want %v, each once", limit, got, want)
		}
	}
}

// A series' page says its person's next episode, which it offers to play (the owner, 2026-10-25):
// the one they are partway through, with where, which its Play resumes; else the one after the last
// they finished; else the first; with why it cannot be read now, as its own page says; and none once
// the last is finished (D160, D324 §3).
func TestBrowse_ASeriesSaysItsPersonsNextEpisode(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelve(t)
	librarytest.Title(t, harness.database, librarytest.TitleSpec{ID: 22, Library: 1, Kind: "episode", AddedAt: 2})
	librarytest.Title(t, harness.database, librarytest.TitleSpec{ID: 23, Library: 1, Kind: "episode", AddedAt: 2})
	librarytest.Version(t, harness.database, librarytest.VersionSpec{ID: 3, TitleID: 22, CreatedAt: 2})
	librarytest.Version(t, harness.database, librarytest.VersionSpec{ID: 4, TitleID: 23, CreatedAt: 2})
	librarytest.File(t, harness.database, librarytest.FileSpec{
		ID: 3, LibraryID: 1, NodeID: 1, Path: "/m/BB/S01E02.mkv", SizeBytes: 10, PayloadHash: []byte{0x03},
		Filing: "listed", VersionID: 3,
	})
	librarytest.File(t, harness.database, librarytest.FileSpec{
		ID: 4, LibraryID: 1, NodeID: 1, Path: "/m/BB/S01E03.mkv", SizeBytes: 10, PayloadHash: []byte{0x04},
		Filing: "listed", VersionID: 4,
	})
	librarytest.Probe(t, harness.database, librarytest.ProbeSpec{FileID: 2, DurationMs: 1000})
	librarytest.Probe(t, harness.database, librarytest.ProbeSpec{FileID: 3, DurationMs: 1000})
	librarytest.Probe(t, harness.database, librarytest.ProbeSpec{FileID: 4, DurationMs: 1000})
	storetest.ExecRaw(t, harness.databasePath,
		"INSERT INTO item_text (item_id, language, is_original, name) VALUES (22, 'und', 1, 'Cat'), "+
			"(23, 'und', 1, 'Bag')",
		storetest.Positions(
			"(1, 22, 1, 2, 10002), (1, 23, 1, 3, 10003)"))
	type nextUp struct {
		TitleID       int64  `json:"itemId"`
		Name          string `json:"name"`
		GroupNumber   int64  `json:"groupNumber"`
		NumberInGroup int64  `json:"numberInGroup"`
		FileID        int64  `json:"fileId"`
		IsPartway     bool   `json:"isPartway"`
		PositionMs    int64  `json:"positionMs"`
		Unavailable   *struct {
			Reason string `json:"reason"`
		} `json:"unavailable"`
	}
	next := func(t *testing.T) *nextUp {
		t.Helper()
		answer := harness.do(t, http.MethodGet, "/items/20", user, nil)
		var series struct {
			NextUp *nextUp `json:"nextUp"`
		}
		if err := json.Unmarshal([]byte(answer.body), &series); err != nil || answer.status != http.StatusOK {
			t.Fatalf("GET /items/20 = %d %s (%v)", answer.status, answer.body, err)
		}
		return series.NextUp
	}
	if got := next(t); got == nil || *got != (nextUp{
		TitleID: 21, Name: "Pilot", GroupNumber: 1, NumberInGroup: 1, FileID: 2,
	}) {
		t.Errorf("nothing watched: next %+v; want the first, season 1's episode 1, to play from its start", got)
	}
	harness.setPositionReadAt(t, 3, 500, 9)
	if got := next(t); got == nil || *got != (nextUp{
		TitleID: 22, Name: "Cat", GroupNumber: 1, NumberInGroup: 2, FileID: 3, IsPartway: true, PositionMs: 500,
	}) {
		t.Errorf("partway: next %+v; want season 1's episode 2, partway at 500 ms", got)
	}
	harness.setPositionReadAt(t, 3, 1000, 10)
	if got := next(t); got == nil || *got != (nextUp{
		TitleID: 23, Name: "Bag", GroupNumber: 1, NumberInGroup: 3, FileID: 4,
	}) {
		t.Errorf("finished: next %+v; want season 1's episode 3, the one after", got)
	}
	storetest.ExecRaw(t, harness.databasePath, "UPDATE file SET payload_hash = NULL WHERE id = 4")
	if got := next(t); got == nil || got.TitleID != 23 || got.Unavailable == nil ||
		got.Unavailable.Reason != string(library.StillReading) {
		t.Errorf("still being read: next %+v; want season 1's episode 3, still reading", got)
	}
	harness.setPositionReadAt(t, 4, 1000, 11)
	if got := next(t); got != nil {
		t.Errorf("the last finished: next %+v; want none", got)
	}
}

// A film or series whose match is a guess says so in the lists and on its page, for its poster's
// mark; one no guess settled says nothing of it (D301 §4).
func TestBrowse_AGuessedMatchSaysSoWhereItsTitleIsShown(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelve(t)
	storetest.ExecRaw(t, harness.databasePath, "INSERT INTO guessed_match (item_id, guessed_at) VALUES (20, 5)")
	films := harness.do(t, http.MethodGet, "/items?kind=film&sort=name&limit=10", admin, nil)
	if strings.Contains(films.body, "isGuessed") {
		t.Errorf("films = %s; Metropolis's match is no guess", films.body)
	}
	for _, path := range []string{"/items?kind=series&sort=name&limit=10", "/items?kind=series&sort=added&limit=10"} {
		if shows := harness.do(t, http.MethodGet, path, admin, nil); !strings.Contains(shows.body, `"isGuessed":true`) {
			t.Errorf("%s = %s; want Brass Arena's guess said", path, shows.body)
		}
	}
	if series := harness.do(t, http.MethodGet, "/items/20", admin, nil); !strings.Contains(series.body,
		`"isGuessed":true`) {
		t.Errorf("the series' page = %s; want its guess said", series.body)
	}
	if detail := harness.do(t, http.MethodGet, "/items/10", admin, nil); strings.Contains(detail.body, "isGuessed") {
		t.Errorf("Metropolis's page = %s; its match is no guess", detail.body)
	}
	storetest.ExecRaw(t, harness.databasePath, "INSERT INTO guessed_match (item_id, guessed_at) VALUES (10, 6)")
	for _, path := range []string{
		"/items?kind=film&sort=name&limit=10", "/items?kind=film&sort=added&limit=10",
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

// titlesProgress is GET path's entries in short: "id:watchedPercent:finished/episodes" each.
func (harness testHarness) titlesProgress(t *testing.T, path, token string) string {
	t.Helper()
	var page struct {
		Items []api.ItemEntryBody `json:"items"`
	}
	harness.readJSON(t, path, token, &page)
	var entries []string
	for _, entry := range page.Items {
		entries = append(entries, fmt.Sprintf("%d:%d:%d/%d", entry.ID, entry.WatchedPercent,
			entry.FinishedEpisodeCount, entry.EpisodeCount))
	}
	return strings.Join(entries, " ")
}

// A list shows how far the person is through each of its entries (the owner, 2026-10-25): a film
// where Continue Watching would resume it, by the same rule, so not once hidden; a series how many of
// its episodes they finished of how many it has, in the ordering they watch it in.
func TestBrowse_AListShowsHowFarThePersonIsThroughEach(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveDVDOrder(t)
	librarytest.Probe(t, harness.database, librarytest.ProbeSpec{FileID: 1, DurationMs: 1000000})
	harness.setPositionReadAt(t, 1, 300_000, 5) // Metropolis, 30% in
	harness.setPositionReadAt(t, 3, 1000, 6)    // 22, aired season 1's episode 2 and in no DVD disc, finished
	harness.setPositionReadAt(t, 4, 1000, 7)    // 23, finished
	harness.setPositionReadAt(t, 5, 500, 8)     // 24, halfway
	for _, path := range []string{"/items?kind=film&sort=name&limit=10", "/items?kind=film&sort=added&limit=10"} {
		if got, want := harness.titlesProgress(t, path, user), "10:30:0/0"; got != want {
			t.Errorf("%s: %s; want %s, Metropolis 30%% in", path, got, want)
		}
	}
	series := "/items?kind=series&sort=added&limit=10"
	if got, want := harness.titlesProgress(t, series, user), "20:0:2/4"; got != want {
		t.Errorf("series in aired: %s; want %s, two of its four episodes finished", got, want)
	}
	storetest.ExecRaw(t, harness.databasePath,
		`INSERT INTO preference (user_id, scope, scope_id, axis, value_json, set_at)
		 VALUES (1, 'item', 20, 'episode_ordering', '"tmdb:dvd"', 1)`,
		`INSERT INTO item_hidden (user_id, item_id, hidden_at) VALUES (1, 10, 5)`)
	if got, want := harness.titlesProgress(t, "/items?kind=series&sort=name&limit=10", user), "20:0:1/3"; got != want {
		t.Errorf("series in the DVD order: %s; want %s, one of its three finished", got, want)
	}
	if got, want := harness.titlesProgress(t, "/items?kind=film&sort=name&limit=10", user), "10:0:0/0"; got != want {
		t.Errorf("Metropolis hidden: %s; want %s, as Continue Watching leaves it out", got, want)
	}
}

// A series' page gives each season its own poster where one is stored (D379), not its series' own,
// which stays the series'; a season with none has none. Another ordering's groups are discs or parts,
// no provider's seasons, so none has a season's poster.
func TestBrowse_ASeriesSeasonsShowTheirOwnPostersInAiredOrderAlone(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveDVDOrder(t)
	seasonOne, seriesPoster := strings.Repeat("ab", 32), strings.Repeat("cc", 32)
	storetest.ExecRaw(t, harness.databasePath,
		`INSERT INTO artwork (item_id, kind, grp_id, source, source_ref, source_sha256)
		 SELECT 20, 'poster', grp.id, 'tmdb', '/s1.jpg', X'`+seasonOne+`' FROM grp
		 JOIN ordering ON ordering.id = grp.ordering_id AND ordering.sequence_id = 20 AND ordering.name = 'aired'
		 WHERE grp.number = 1`)
	var seen struct {
		Art struct {
			PosterSha256 string `json:"posterSha256"`
		} `json:"art"`
		Seasons []struct {
			SeasonNumber int64  `json:"seasonNumber"`
			PosterSha256 string `json:"posterSha256"`
		} `json:"seasons"`
	}
	harness.readJSON(t, "/items/20", user, &seen)
	got, want := fmt.Sprint(seen.Seasons), "[{1 "+seasonOne+"} {2 }]"
	if got != want || seen.Art.PosterSha256 != seriesPoster {
		t.Errorf("aired: seasons %s, series poster %s; want %s and the series' own poster", got,
			seen.Art.PosterSha256, want)
	}
	storetest.ExecRaw(t, harness.databasePath, `INSERT INTO preference (user_id, scope, scope_id, axis, value_json,
		set_at) VALUES (1, 'item', 20, 'episode_ordering', '"tmdb:dvd"', 1)`)
	seen.Seasons = nil
	harness.readJSON(t, "/items/20", user, &seen)
	if got, want = fmt.Sprint(seen.Seasons), "[{1 } {2 }]"; got != want {
		t.Errorf("the DVD order: seasons %s; want %s, no disc a season's poster", got, want)
	}
}
