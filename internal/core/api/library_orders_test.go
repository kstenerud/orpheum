// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/library/librarytest"
	"github.com/kstenerud/orpheum/internal/core/textindex"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// shelveDatedFilms adds, after shelve's Metropolis (10, 1995-12-15), four films to library 1: Wing (11,
// 1979-05-25), Greed (12, 1985, its year alone), and The Freshman (13) and Faust (14), with no date; and
// dates Brass Arena (20) 2008-01-20, beside a series of no date, Tweedcoat (30), with one episode.
func (harness testHarness) shelveDatedFilms(t *testing.T) {
	t.Helper()
	sorter, err := textindex.NewSorter("en")
	if err != nil {
		t.Fatal(err)
	}
	key := func(name string) string { return "X'" + hex.EncodeToString(sorter.Key(name)) + "'" }
	for index, name := range []string{"Wing", "Greed", "The Freshman", "Faust"} {
		id := int64(11 + index)
		librarytest.Title(t, harness.database, librarytest.TitleSpec{ID: id, Library: 1, Year: 1980, AddedAt: 3})
		librarytest.Version(t, harness.database, librarytest.VersionSpec{ID: id, TitleID: id, CreatedAt: 3})
		librarytest.File(t, harness.database, librarytest.FileSpec{
			ID: id, LibraryID: 1, NodeID: 1, Path: "/m/" + name + ".mkv", SizeBytes: 10,
			PayloadHash: []byte{byte(id)}, Filing: "listed", VersionID: id,
		})
		storetest.ExecRaw(t, harness.databasePath,
			fmt.Sprintf(`INSERT INTO item_text (item_id, language, is_original, name, search_alternates)
			 VALUES (%d, 'und', 1, '%s', '%s')`, id, name, strings.ToLower(name)),
			fmt.Sprintf(`INSERT INTO sort_key (item_id, locale, listing, collation_key) VALUES (%d, 'en', 'movie', %s)`,
				id, key(name)))
	}
	librarytest.Item(t, harness.database, librarytest.ItemSpec{ID: 30, Kind: "collection", LibraryID: 1, AddedAt: 3})
	librarytest.Title(t, harness.database, librarytest.TitleSpec{ID: 31, Library: 1, Kind: "episode", AddedAt: 3})
	librarytest.Version(t, harness.database, librarytest.VersionSpec{ID: 31, TitleID: 31, CreatedAt: 3})
	librarytest.File(t, harness.database, librarytest.FileSpec{
		ID: 31, LibraryID: 1, NodeID: 1, Path: "/m/Tweedcoat/S01E01.mkv", SizeBytes: 10, PayloadHash: []byte{31},
		Filing: "listed", VersionID: 31,
	})
	storetest.ExecRaw(t, harness.databasePath,
		"UPDATE title SET released_on = CASE id WHEN 10 THEN 19951215 WHEN 11 THEN 19790525 WHEN 12 THEN 19850000 END",
		"UPDATE collection SET released_on = 20080120 WHERE id = 20",
		`INSERT INTO collection (id, kind, membership, created_by, identification_state)
		 VALUES (30, 'series', 'explicit', 'scanner', 'identified')`,
		"INSERT INTO collection_member (collection_id, member_title, position) VALUES (30, 31, 10001)",
		`INSERT INTO item_text (item_id, language, is_original, name, search_alternates)
		 VALUES (30, 'und', 1, 'Tweedcoat', 'tweedcoat'), (31, 'und', 1, 'Pilot', 'pilot')`,
		fmt.Sprintf("INSERT INTO sort_key (item_id, locale, listing, collation_key) VALUES (30, 'en', 'series', %s)",
			key("Tweedcoat")))
}

// pagedNames are a list's names read a page of two at a time to its end, ten pages at most.
func (harness testHarness) pagedNames(t *testing.T, path, token string) []string {
	t.Helper()
	cursor, names, pages := "", []string{}, 0
	for {
		page, next := harness.titleNames(t, path+"&limit=2"+cursor, token)
		names, pages = append(names, page...), pages+1
		if next == "" || pages > 10 {
			return names
		}
		cursor = "&cursor=" + next
	}
}

// Films sort by release date, a series by its first air date, the year alone where no day is
// known; every sort runs either way, the default as it was; what has no date is last either way,
// a page at a time across it (D366 §1, §2, §3).
func TestBrowse_EverySortRunsEitherWayAndWhatHasNoDateIsLast(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelve(t)
	harness.shelveDatedFilms(t)
	for path, want := range map[string]string{
		"/titles?kind=movie&sort=released":                  "[Metropolis Greed Wing The Freshman Faust]",
		"/titles?kind=movie&sort=released&order=descending": "[Metropolis Greed Wing The Freshman Faust]",
		"/titles?kind=movie&sort=released&order=ascending":  "[Wing Greed Metropolis The Freshman Faust]",
		"/titles?kind=series&sort=released":                 "[Brass Arena Tweedcoat]",
		"/titles?kind=series&sort=released&order=ascending": "[Brass Arena Tweedcoat]",
		"/titles?kind=movie&sort=name":                      "[Wing Greed The Freshman Faust Metropolis]",
		"/titles?kind=movie&sort=name&order=descending":     "[Metropolis Faust The Freshman Greed Wing]",
		"/titles?kind=series&sort=name&order=descending":    "[Tweedcoat Brass Arena]",
		"/titles?kind=movie&sort=added":                     "[Faust The Freshman Greed Wing Metropolis]",
		"/titles?kind=movie&sort=added&order=ascending":     "[Metropolis Wing Greed The Freshman Faust]",
		"/titles?kind=series&sort=added&order=ascending":    "[Brass Arena Tweedcoat]",
		"/titles?kind=any&sort=name&order=descending":       "[Metropolis Faust Tweedcoat The Freshman Brass Arena Greed Wing]",
	} {
		if names := harness.pagedNames(t, path, admin); fmt.Sprint(names) != want {
			t.Errorf("%s in pages of two = %v; want %s", path, names, want)
		}
	}
	for _, kind := range []string{"other", "any"} {
		path := "/titles?sort=released&limit=5&kind=" + kind
		wantInvalidAt(t, harness.do(t, http.MethodGet, path, admin, nil), http.StatusUnprocessableEntity, "query.sort")
	}
}

// Other videos run either way in each of their four orders, and a folder's videos with them
// (D366 §2).
func TestBrowse_OtherVideosRunEitherWay(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelve(t)
	harness.shelveOtherVideos(t)
	for path, want := range map[string]string{
		"/titles?kind=other&sort=recorded&order=ascending":   "[Wedding Beach day clip]",
		"/titles?kind=other&sort=name&order=descending":      "[Wedding clip Beach day]",
		"/titles?kind=other&sort=file_name&order=descending": "[clip Wedding Beach day]",
		"/titles?kind=other&sort=added&order=ascending":      "[Beach day Wedding clip]",
	} {
		if names := harness.pagedNames(t, path, admin); fmt.Sprint(names) != want {
			t.Errorf("%s in pages of two = %v; want %s", path, names, want)
		}
	}
	librarytest.Title(t, harness.database, librarytest.TitleSpec{
		ID: 43, Library: 2, Kind: "other", IdentificationState: "identified", FollowedSource: "none", AddedAt: 4,
	})
	librarytest.Version(t, harness.database, librarytest.VersionSpec{ID: 43, TitleID: 43, CreatedAt: 4})
	librarytest.File(t, harness.database, librarytest.FileSpec{
		ID: 43, LibraryID: 2, NodeID: 1, Path: "/h/Trip/sparrows.mp4", SizeBytes: 10, PayloadHash: []byte{0x43},
		Filing: "listed", VersionID: 43, FirstSeenAt: 50, ModifiedAt: 5,
	})
	storetest.ExecRaw(t, harness.databasePath,
		"INSERT INTO item_text (item_id, language, is_original, name, search_alternates) "+
			"VALUES (43, 'und', 1, 'arrival', 'arrival')",
		"INSERT INTO other_video (title_id, recorded_at, recorded_from, file_name, file_name_key) "+
			"VALUES (43, -86400000, 'file', 'sparrows.mp4', X'01')")
	// Recorded in 1969, before every other, whichever way the list runs.
	for path, want := range map[string]string{
		"/titles?kind=other&sort=recorded":                 "[clip Beach day Wedding arrival]",
		"/titles?kind=other&sort=recorded&order=ascending": "[arrival Wedding Beach day clip]",
	} {
		if names := harness.pagedNames(t, path, admin); fmt.Sprint(names) != want {
			t.Errorf("%s in pages of two = %v; want %s", path, names, want)
		}
	}
	for order, want := range map[string]string{"": "[arrival clip]", "&order=descending": "[clip arrival]"} {
		var trip struct {
			Titles []struct {
				Name string `json:"name"`
			} `json:"titles"`
		}
		harness.readJSON(t, "/libraries/2/folders?sort=name&folder=Trip"+order, admin, &trip)
		names := []string{}
		for _, title := range trip.Titles {
			names = append(names, title.Name)
		}
		if got := fmt.Sprint(names); got != want {
			t.Errorf("the Trip folder by name%s = %s; want %s", order, got, want)
		}
	}
}

// Z to A, a name starts the list at the last title sorted before it: the letter after a letter of
// the rail starts that letter's titles, from their end (D366 §4).
func TestBrowse_ZToAFromANameStartsAtTheLastTitleBeforeIt(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelve(t)
	harness.shelveDatedFilms(t)
	for from, want := range map[string]string{"E": "[Faust The Freshman]", "D": "[The Freshman Greed]", "A": "[]"} {
		path := "/titles?kind=movie&sort=name&order=descending&limit=2&from=" + from
		if names, _ := harness.titleNames(t, path, admin); fmt.Sprint(names) != want {
			t.Errorf("Z to A from %s = %v; want %s", from, names, want)
		}
	}
}
