// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/library/librarytest"
	"github.com/kstenerud/orpheum/internal/core/textindex"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// shelveOtherVideos adds, after shelve's library 1, library 2, Home movies, an other library over
// /h with three videos, as the identification pass leaves them (D349): 40, "Beach day",
// /h/2019/2019-07-14 Beach day.mp4, recorded 2019-07-14 by its tag; 41, "Wedding",
// /h/2018 Wedding/ceremony.mp4, recorded 2018-06-01 by its folder's name; and 42, "clip",
// /h/Trip/clip.mp4, recorded 2020-01-01 by the file's time.
func (harness testHarness) shelveOtherVideos(t *testing.T) {
	t.Helper()
	sorter, err := textindex.NewSorter("en")
	if err != nil {
		t.Fatal(err)
	}
	key := func(name string) string { return "X'" + hex.EncodeToString(sorter.Key(name)) + "'" }
	day := func(year int, month time.Month, date int) int64 {
		return time.Date(year, month, date, 0, 0, 0, 0, time.UTC).UnixMilli()
	}
	librarytest.LibraryOn(t, harness.database, 1,
		librarytest.Library{Name: "Home movies", Kind: "other", MetadataSource: "nfo", Root: "/h"})
	storetest.ExecRaw(t, harness.databasePath,
		`INSERT INTO item (id, kind, library_id, added_at) VALUES (40, 'title', 2, 4), (41, 'title', 2, 4),
		 (42, 'title', 2, 4)`,
		`INSERT INTO title (id, kind, identification_state, followed_source)
		 VALUES (40, 'other', 'identified', 'none'), (41, 'other', 'identified', 'none'),
		 (42, 'other', 'identified', 'none')`,
		`INSERT INTO item_text (item_id, language, is_original, name, search_alternates) VALUES
		 (40, 'und', 1, 'Beach day', 'beach day'), (41, 'und', 1, 'Wedding', 'wedding'),
		 (42, 'und', 1, 'clip', 'clip')`,
		fmt.Sprintf(`INSERT INTO sort_key (item_id, locale, listing, collation_key)
		 VALUES (40, 'en', 'other', %s), (41, 'en', 'other', %s), (42, 'en', 'other', %s)`,
			key("Beach day"), key("Wedding"), key("clip")),
		"INSERT INTO version (id, title_id, created_at) VALUES (40, 40, 4), (41, 41, 4), (42, 42, 4)",
		`INSERT INTO file (id, library_id, node_id, path, size_bytes, payload_hash, filing, version_id, first_seen_at,
		 last_seen_at, modified_at) VALUES
		 (40, 2, 1, '/h/2019/2019-07-14 Beach day.mp4', 10, X'40', 'listed', 40, 10, 10, 5),
		 (41, 2, 1, '/h/2018 Wedding/ceremony.mp4', 10, X'41', 'listed', 41, 20, 20, 5),
		 (42, 2, 1, '/h/Trip/clip.mp4', 10, X'42', 'listed', 42, 30, 30, 5)`,
		fmt.Sprintf(`INSERT INTO other_video (title_id, recorded_at, recorded_from, file_name, file_name_key) VALUES
		 (40, %d, 'tag', '2019-07-14 Beach day.mp4', %s), (41, %d, 'name', 'ceremony.mp4', %s),
		 (42, %d, 'file', 'clip.mp4', %s)`,
			day(2019, 7, 14), key("2019-07-14 Beach day.mp4"), day(2018, 6, 1), key("ceremony.mp4"),
			day(2020, 1, 1), key("clip.mp4")))
}

// An other library is not asked where its data comes from: it takes NFO files only, and one asked
// to be matched online is refused; every other kind is still asked (D184, D349 §10).
func TestLibraries_AnOtherLibraryIsAskedNoSourceAndTakesNFOFilesOnly(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.connectNode(t, "/media/shows", "/media/home")
	unasked := newShows("/media/shows")
	delete(unasked, "metadataSource")
	wantInvalidAt(t, harness.do(t, http.MethodPost, "/libraries", admin, unasked),
		http.StatusUnprocessableEntity, "body.metadataSource")
	home := newShows("/media/home")
	home["name"], home["kind"] = "Home movies", "other"
	wantInvalidAt(t, harness.do(t, http.MethodPost, "/libraries", admin, home),
		http.StatusUnprocessableEntity, "body.metadataSource")
	delete(home, "metadataSource")
	if created := harness.do(t, http.MethodPost, "/libraries", admin, home); created.status != http.StatusCreated {
		t.Fatalf("POST /libraries of other videos = %d %s", created.status, created.body)
	}
	listed := harness.do(t, http.MethodGet, "/libraries", admin, nil)
	if !strings.Contains(listed.body, `"name":"Home movies","kind":"other","metadataSource":"nfo"`) {
		t.Errorf("GET /libraries = %s; want the other library, taking NFO files only", listed.body)
	}
}

// An other video partway through is in Continue Watching as a film is (D349 §7).
func TestContinueWatching_HoldsAnOtherVideoPartwayThrough(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelve(t)
	harness.shelveOtherVideos(t)
	harness.setPositionReadAt(t, 40, 3_000, 100)
	if got := fmt.Sprint(rowsOf(harness.continueWatching(t, user))); got != "[40:40:]" {
		t.Errorf("Continue Watching = %s; want the beach day, partway through", got)
	}
}
