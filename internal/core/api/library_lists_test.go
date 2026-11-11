// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// titleNames are a list's names, in its order, and its next cursor.
func (harness testHarness) titleNames(t *testing.T, path, token string) (names []string, nextCursor string) {
	t.Helper()
	var page struct {
		Items      []api.ItemEntryBody `json:"items"`
		NextCursor string              `json:"nextCursor"`
	}
	harness.readJSON(t, path, token, &page)
	for _, entry := range page.Items {
		names = append(names, entry.Name)
	}
	return names, page.NextCursor
}

// railLetters are the alphabet rail's letters a GET /items/letters answers.
func (harness testHarness) railLetters(t *testing.T, path, token string) string {
	t.Helper()
	var letters struct {
		Letters []string `json:"letters"`
	}
	harness.readJSON(t, path, token, &letters)
	return fmt.Sprint(letters.Letters)
}

// One library's list holds its titles alone, and a library the person does not reach is no such
// library, not an empty one (D349 §1).
func TestBrowse_OneLibrarysListAndALibraryNotReached(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelve(t)
	harness.shelveOtherVideos(t)
	names, _ := harness.titleNames(t, "/items?kind=film,series,other&sort=name&limit=10&library=2", admin)
	if fmt.Sprint(names) != "[Beach day clip Wedding]" {
		t.Errorf("library 2 = %v; want its three videos alone", names)
	}
	names, _ = harness.titleNames(t, "/items?kind=film,series,other&sort=name&limit=10", admin)
	if fmt.Sprint(names) != "[Beach day Brass Arena clip Metropolis Wedding]" {
		t.Errorf("every library = %v; want films, series and other videos in one order by name", names)
	}
	storetest.ExecRaw(t, harness.databasePath,
		"DELETE FROM role_library WHERE library_id = 2 AND role_id = (SELECT id FROM role WHERE name = 'user')")
	if _, err := harness.authService.CreateUser(t.Context(),
		auth.NewUser{Name: "partner", Password: "partner-password", Role: auth.RoleUser}); err != nil {
		t.Fatal(err)
	}
	user := harness.signIn(t, "partner", "partner-password")
	for _, path := range []string{
		"/items?kind=other&sort=name&limit=10&library=2", "/items/letters?kind=other&sort=name&library=2",
		"/libraries/2/folders?limit=500&sort=name",
	} {
		wantError(t, harness.do(t, http.MethodGet, path, user, nil), http.StatusNotFound, api.CodeNotFound)
	}
	if found := harness.do(t, http.MethodGet, "/search?query=beach&limit=10", user, nil); found.status !=
		http.StatusOK || strings.Contains(found.body, "Beach") {
		t.Errorf("the partner's search = %d %s; want nothing from library 2", found.status, found.body)
	}
	if names, _ = harness.titleNames(t, "/items?kind=film,series,other&sort=name&limit=10", user); fmt.Sprint(names) !=
		"[Brass Arena Metropolis]" {
		t.Errorf("every library the partner reaches = %v; want library 1's alone", names)
	}
}

// A list names its kinds, comma-separated, and lists those alone, merged in one order; absent, it
// lists films, series and other videos; "any" is no kind, refused as any unknown kind is, so a kind
// a later server adds is in no list a client did not name it in (D432 §1).
func TestBrowse_AListNamesItsKinds(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelve(t)
	harness.shelveOtherVideos(t)
	for path, want := range map[string]string{
		"/items?kind=film,other&sort=name&limit=10":        "[Beach day clip Metropolis Wedding]",
		"/items?kind=series,film&sort=name&limit=10":       "[Brass Arena Metropolis]",
		"/items?kind=film,film&sort=name&limit=10":         "[Metropolis]",
		"/items?sort=name&limit=10":                        "[Beach day Brass Arena clip Metropolis Wedding]",
		"/items?kind=film,series,other&sort=name&limit=10": "[Beach day Brass Arena clip Metropolis Wedding]",
	} {
		if names, _ := harness.titleNames(t, path, admin); fmt.Sprint(names) != want {
			t.Errorf("%s = %v; want %s", path, names, want)
		}
	}
	if got := harness.railLetters(t, "/items/letters?kind=series,other&sort=name", admin); got != "[B C W]" {
		t.Errorf("the letters of series and other videos = %s; want [B C W]", got)
	}
	for path, location := range map[string]string{
		"/items?kind=any&sort=name&limit=10":      "query.kind[0]",
		"/items?kind=film,any&sort=name&limit=10": "query.kind[1]",
		"/items/letters?kind=any&sort=name":       "query.kind[0]",
		"/genres?kind=any&limit=10":               "query.kind[0]",
		"/items?kind=album&sort=name&limit=10":    "query.kind[0]",
	} {
		wantInvalidAt(t, harness.do(t, http.MethodGet, path, admin, nil), http.StatusUnprocessableEntity, location)
	}
}

// Other videos come newest recorded first, by name, by file name and newest added first, a page at a
// time, each saying when it was recorded and where that came from; films and series have no such
// orders (D349 §4).
func TestBrowse_OtherVideosInTheirFourOrders(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelve(t)
	harness.shelveOtherVideos(t)
	for sort, want := range map[string]string{
		"recorded": "[clip Beach day Wedding]", "name": "[Beach day clip Wedding]",
		"fileName": "[Beach day Wedding clip]", "added": "[clip Wedding Beach day]",
	} {
		first, cursor := harness.titleNames(t, "/items?kind=other&limit=2&sort="+sort, admin)
		rest, last := harness.titleNames(t, "/items?kind=other&limit=2&sort="+sort+"&cursor="+cursor, admin)
		if got := fmt.Sprint(append(first, rest...)); got != want || cursor == "" || last != "" {
			t.Errorf("by %s: %s in pages of two (cursor %q, then %q); want %s", sort, got, cursor, last, want)
		}
	}
	var page struct {
		Items []api.ItemEntryBody `json:"items"`
	}
	harness.readJSON(t, "/items?kind=other&sort=recorded&limit=1", admin, &page)
	if entry := page.Items[0]; entry.Kind != "other" || entry.Recorded == nil || entry.Recorded.From != "file" ||
		entry.Recorded.FileName != "clip.mp4" {
		t.Errorf("the newest recorded = %+v; want the clip, an other video dated by its file", entry)
	}
	wantInvalidAt(t, harness.do(t, http.MethodGet, "/items?kind=film&sort=recorded&limit=5", admin, nil),
		http.StatusUnprocessableEntity, "query.sort")
}

// The rail's letters are those a list has titles under in its order, by name or file name, # for
// what comes before A (D349 §9).
func TestBrowse_TheRailsLettersAreThoseTheListHasTitlesUnder(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelve(t)
	harness.shelveOtherVideos(t)
	for path, want := range map[string]string{
		"/items/letters?kind=film,series,other&sort=name": "[B C M W]",
		"/items/letters?kind=film&sort=name":              "[M]",
		"/items/letters?kind=other&sort=fileName":         "[# C]",
		"/items/letters?kind=series&sort=name&library=2":  "[]",
		"/items/letters?kind=other&sort=name&library=2":   "[B C W]",
	} {
		if got := harness.railLetters(t, path, admin); got != want {
			t.Errorf("%s = %s; want %s", path, got, want)
		}
	}
}

// An other library's folders open into their folders and videos, each folder saying how many videos
// it holds; another kind of library has none to browse (D349 §5).
func TestBrowse_AnOtherLibrarysFolders(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelve(t)
	harness.shelveOtherVideos(t)
	var top, trip api.FolderOutput
	harness.readJSON(t, "/libraries/2/folders?limit=500&sort=recorded", admin, &top.Body)
	if got := fmt.Sprint(top.Body.Subfolders); got != "[{2018 Wedding 2018 Wedding 1} {2019 2019 1} {Trip Trip 1}]" ||
		len(top.Body.Titles) != 0 {
		t.Errorf("the top = %s and %d videos; want its three folders and none of its own", got, len(top.Body.Titles))
	}
	harness.readJSON(t, "/libraries/2/folders?limit=500&sort=name&folder=Trip", admin, &trip.Body)
	if len(trip.Body.Titles) != 1 || trip.Body.Titles[0].Name != "clip" || trip.Body.Folder != "Trip" {
		t.Errorf("Trip = %+v; want the clip", trip.Body)
	}
	wantInvalidAt(t, harness.do(t, http.MethodGet, "/libraries/1/folders?limit=500&sort=name", admin, nil),
		http.StatusUnprocessableEntity, "path.id")
}

// An other library's folder comes in pages, its folders first and then its videos, each page's
// cursor taking up where it ended; a cursor the server did not give is refused (D432 §3).
func TestBrowse_AFolderComesInPages(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelve(t)
	harness.shelveOtherVideos(t)
	var names []string
	cursor, pages := "", 0
	for ; pages < 5; pages++ {
		var page api.FolderOutput
		harness.readJSON(t, "/libraries/2/folders?limit=2&sort=name&cursor="+url.QueryEscape(cursor), admin,
			&page.Body)
		for _, subfolder := range page.Body.Subfolders {
			names = append(names, subfolder.Name)
		}
		if cursor = page.Body.NextCursor; cursor == "" {
			break
		}
	}
	if got := strings.Join(names, ","); got != "2018 Wedding,2019,Trip" || pages != 1 {
		t.Errorf("the top's folders by pages of two = %s over %d pages after the first; want all three over two",
			got, pages)
	}
	wantInvalidAt(t, harness.do(t, http.MethodGet, "/libraries/2/folders?limit=2&sort=name&cursor=!", admin, nil),
		http.StatusUnprocessableEntity, "query.cursor")
	wantInvalidAt(t, harness.do(t, http.MethodGet, "/libraries/2/folders?sort=name", admin, nil),
		http.StatusUnprocessableEntity, "query.limit")
}

// A library a person leaves out is gone from their Films and Series, and still on its own (D349 §8).
func TestBrowse_ALibraryLeftOutOfFilmsAndSeries(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelve(t)
	harness.shelveOtherVideos(t)
	if put := harness.do(t, http.MethodPut, "/me/preferences", admin,
		map[string]any{"leftOutLibraryIds": []int64{1}}); put.status != http.StatusNoContent {
		t.Fatalf("PUT /me/preferences = %d %s", put.status, put.body)
	}
	if mine := harness.do(t, http.MethodGet, "/me/preferences", admin, nil); !strings.Contains(mine.body,
		`"leftOutLibraryIds":[1]`) {
		t.Errorf("GET /me/preferences = %s; want library 1 left out", mine.body)
	}
	for path, want := range map[string]string{
		"/items?kind=film&sort=name&limit=5":              "[]",
		"/items?kind=series&sort=added&limit=5":           "[]",
		"/items?kind=film&sort=name&limit=5&library=1":    "[Metropolis]",
		"/items?kind=film,series,other&sort=name&limit=2": "[Beach day Brass Arena]",
	} {
		if names, _ := harness.titleNames(t, path, admin); fmt.Sprint(names) != want {
			t.Errorf("%s = %v; want %s", path, names, want)
		}
	}
	if got := harness.railLetters(t, "/items/letters?kind=film&sort=name", admin); got != "[]" {
		t.Errorf("the films' letters with their one library left out = %s; want none", got)
	}
}

// GET /items' query is a criterion of the one list, as its kind and library are (the owner,
// 2026-10-30): a series matches by its own name, never by an episode's; the rail's letters are those
// of the titles that match.
func TestBrowse_AQueryIsACriterionOfTheList(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelve(t)
	harness.shelveOtherVideos(t)
	for path, want := range map[string]string{
		"/items?kind=film,series,other&sort=name&limit=10&query=beach":                "[Beach day]",
		"/items?kind=film,series,other&sort=name&limit=10&query=b":                    "[Beach day Brass Arena]",
		"/items?kind=film&sort=name&limit=10&query=beach":                             "[]",
		"/items?kind=other&sort=name&limit=10&query=beach&library=1":                  "[]",
		"/items?kind=other&sort=recorded&limit=10&query=beach":                        "[Beach day]",
		"/items?kind=series&sort=added&limit=10&query=brass":                          "[Brass Arena]",
		"/items?kind=series&sort=name&limit=10&query=pilot":                           "[]", // its episode's name
		"/items?kind=film,series,other&sort=name&limit=10&query=%20":                  "[]",
		"/items?kind=film,series,other&sort=name&limit=10&home=true&query=metropolis": "[Metropolis]",
	} {
		if names, _ := harness.titleNames(t, path, admin); fmt.Sprint(names) != want {
			t.Errorf("%s = %v; want %s", path, names, want)
		}
	}
	for path, want := range map[string]string{
		"/items/letters?kind=film,series,other&sort=name&query=b":                 "[B]",
		"/items/letters?kind=film,series,other&sort=name&query=wedding":           "[W]",
		"/items/letters?kind=other&sort=fileName&query=clip":                      "[C]",
		"/items/letters?kind=film,series,other&sort=name&query=wedding&library=1": "[]",
	} {
		if got := harness.railLetters(t, path, admin); got != want {
			t.Errorf("%s = %s; want %s", path, got, want)
		}
	}
}
