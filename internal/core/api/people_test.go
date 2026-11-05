// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// shelvePeople adds to shelve's Metropolis and Brass Arena their genres and people: Metropolis a Crime film
// with Ansel Vardo as Mira Holloway-Pent, directed by Oren Talisk; Brass Arena a Drama with Bryan
// Cranston as Corwin Ashby.
func (harness testHarness) shelvePeople(t *testing.T) {
	t.Helper()
	harness.shelve(t)
	storetest.ExecRaw(t, harness.databasePath,
		`INSERT INTO item (id, kind, added_at) VALUES (40, 'person', 1), (41, 'person', 1), (42, 'person', 1)`,
		`INSERT INTO person (id, kind, tmdb_id, name_key, picture_ref) VALUES
		 (40, 'person', '1158', 'ansel vardo', '/vardo.jpg'), (41, 'person', '638', 'oren talisk', NULL),
		 (42, 'person', '17419', 'bela ostrander', NULL)`,
		`INSERT INTO item_text (item_id, language, is_original, name, search_alternates) VALUES
		 (40, 'und', 1, 'Ansel Vardo', 'ansel vardo'), (41, 'und', 1, 'Oren Talisk', 'oren talisk'),
		 (42, 'und', 1, 'Bela Ostrander', 'bela ostrander')`,
		`INSERT INTO genre (id, name, name_key) VALUES (1, 'Crime', 'crime'), (2, 'Drama', 'drama')`,
		`INSERT INTO item_genre (item_id, genre_id, ordinal) VALUES (10, 1, 0), (20, 2, 0)`,
		`INSERT INTO credit (item_id, job, ordinal, person_id, character) VALUES
		 (10, 'cast', 0, 40, 'Mira Holloway-Pent'), (10, 'director', 0, 41, NULL), (20, 'cast', 0, 42, 'Corwin Ashby')`)
}

// A title's page carries its genres and credits, an episode its series'; a person's page is the
// person, and the list by person carries their credit on each item (D425 §7); the genres are
// counted; the list filters by genre and person; a grouped search finds titles, people and genres, a
// group paged by its cursor.
func TestBrowse_GenresAndPeopleAreShownListedFilteredAndSearched(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelvePeople(t)
	wants := []struct{ path, want string }{
		{"/items/10", `"genres":[{"id":1,"name":"Crime"}],"credits":{"cast":[{"personId":40,"name":"Ansel Vardo",` +
			`"character":"Mira Holloway-Pent","pictureRef":"/vardo.jpg"}],"directors":[{"personId":41,` +
			`"name":"Oren Talisk"}],"writers":[]}`},
		{"/items/21", `"genres":[{"id":2,"name":"Drama"}],"credits":{"cast":[{"personId":42,` +
			`"name":"Bela Ostrander",`},
		{"/items/20", `"genres":[{"id":2,"name":"Drama"}],"credits":{"cast":[{"personId":42,`},
		{"/people/40", `{"id":40,"name":"Ansel Vardo","pictureRef":"/vardo.jpg"}`},
		{"/items?limit=10&person=40", `"name":"Metropolis","year":1995,`},
		{"/items?limit=10&person=40", `"credit":{"jobs":["cast"],"character":"Mira Holloway-Pent"}}]}`},
		{"/items?kind=movie&limit=10&person=41", `"credit":{"jobs":["director"]}}]}`},
		{"/genres?kind=movie,series,other&limit=500", `"genres":[{"id":1,"name":"Crime","titleCount":1},` +
			`{"id":2,"name":"Drama","titleCount":1}]`},
		{"/genres?kind=movie&limit=500", `"genres":[{"id":1,"name":"Crime","titleCount":1}]`},
		{"/items?kind=movie,series,other&sort=name&limit=10&genre=2", `"name":"Brass Arena"`},
		{"/items?kind=movie&sort=name&limit=10&person=41&decade=1990", `"name":"Metropolis"`},
		{"/items?kind=movie&sort=name&limit=10&person=42", `{"items":[]}`},
		{"/items/letters?kind=movie&sort=name&genre=1", `"letters":["H"]`},
		{"/items/letters?kind=movie&sort=name&genre=2", `"letters":[]`},
		{"/search?query=r&limit=10", `"titles":{"hits":[`},
		{"/search?query=cr&limit=10", `"genres":{"hits":[{"id":1,"name":"Crime","titleCount":1}]}`},
		{"/search?query=a&limit=500", `"people":{"hits":[{"id":40,"name":"Ansel Vardo",`},
	}
	for _, want := range wants {
		if got := harness.do(t, http.MethodGet, want.path, admin, nil); got.status != http.StatusOK ||
			!strings.Contains(got.body, want.want) {
			t.Errorf("%s = %d %s; want %s", want.path, got.status, got.body, want.want)
		}
	}
	harness.wantSearchPagesPeopleByCursor(t, admin)
	heat := harness.do(t, http.MethodGet, "/items?kind=movie,series,other&sort=name&limit=10&genre=1", admin, nil)
	if strings.Contains(heat.body, "Brass Arena") {
		t.Errorf("the Crime titles = %s; want Metropolis alone", heat.body)
	}
	storetest.ExecRaw(t, harness.databasePath, "DELETE FROM credit WHERE item_id = 10",
		"DELETE FROM item_genre WHERE item_id = 10")
	if bare := harness.do(t, http.MethodGet, "/items/10", admin, nil); strings.Contains(bare.body, `"genres"`) ||
		strings.Contains(bare.body, `"credits"`) {
		t.Errorf("a title with no genres or credits = %s; want neither field", bare.body)
	}
	wantError(t, harness.do(t, http.MethodGet, "/people/99", admin, nil), http.StatusNotFound, api.CodeNotFound)
	// A credited person is no item: their page is /people/{id} (D425 §7).
	wantError(t, harness.do(t, http.MethodGet, "/items/40", admin, nil), http.StatusNotFound, api.CodeNotFound)
	wantError(t, harness.do(t, http.MethodGet, "/search?query=a&limit=1&cursor=1", admin, nil),
		http.StatusUnprocessableEntity, api.CodeInvalidRequest)
	wantError(t, harness.do(t, http.MethodGet, "/search?query=a&limit=501", admin, nil),
		http.StatusUnprocessableEntity, api.CodeInvalidRequest)
}

// wantSearchPagesPeopleByCursor checks that a search's people come a page at a time, each page after
// the last's nextCursor, the cursor of every list (D416 §6); a cursor with no group, or one this
// server did not give, is refused at query.cursor.
func (harness testHarness) wantSearchPagesPeopleByCursor(t *testing.T, admin string) {
	t.Helper()
	type page struct {
		People struct {
			Hits []struct {
				Name string `json:"name"`
			} `json:"hits"`
			NextCursor string `json:"nextCursor"`
		} `json:"people"`
	}
	var first, second page
	harness.readJSON(t, "/search?query=a&limit=1", admin, &first)
	if len(first.People.Hits) != 1 || first.People.Hits[0].Name != "Ansel Vardo" || first.People.NextCursor == "" {
		t.Fatalf("the first page of people = %+v; want Ansel Vardo, and a cursor", first.People)
	}
	harness.readJSON(t, "/search?query=a&limit=1&group=people&cursor="+first.People.NextCursor, admin, &second)
	if len(second.People.Hits) != 1 || second.People.Hits[0].Name != "Bela Ostrander" ||
		second.People.NextCursor == "" {
		t.Errorf("the next page of people = %+v; want Bela Ostrander, and a cursor", second.People)
	}
	for _, path := range []string{
		"/search?query=a&limit=1&cursor=" + first.People.NextCursor, "/search?query=a&limit=1&group=people&cursor=1",
	} {
		wantInvalidAt(t, harness.do(t, http.MethodGet, path, admin, nil), http.StatusUnprocessableEntity,
			"query.cursor")
	}
}
