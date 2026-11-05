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
	for index, path := range []string{
		"/h/2019/2019-07-14 Beach day.mp4", "/h/2018 Wedding/ceremony.mp4", "/h/Trip/clip.mp4",
	} {
		id := int64(40 + index)
		librarytest.Title(t, harness.database, librarytest.TitleSpec{
			ID: id, Library: 2, Kind: "other", IdentificationState: "identified", FollowedSource: "none", AddedAt: 4,
		})
		librarytest.Version(t, harness.database, librarytest.VersionSpec{ID: id, TitleID: id, CreatedAt: 4})
		librarytest.File(t, harness.database, librarytest.FileSpec{
			ID: id, LibraryID: 2, NodeID: 1, Path: path, SizeBytes: 10, PayloadHash: []byte{byte(0x40 + index)},
			Filing: "listed", VersionID: id, FirstSeenAt: 10 * int64(index+1), ModifiedAt: 5,
		})
	}
	storetest.ExecRaw(t, harness.databasePath,
		`INSERT INTO item_text (item_id, language, is_original, name, search_alternates) VALUES
		 (40, 'und', 1, 'Beach day', 'beach day'), (41, 'und', 1, 'Wedding', 'wedding'),
		 (42, 'und', 1, 'clip', 'clip')`,
		fmt.Sprintf(`INSERT INTO sort_key (item_id, locale, listing, collation_key)
		 VALUES (40, 'en', 'other', %s), (41, 'en', 'other', %s), (42, 'en', 'other', %s)`,
			key("Beach day"), key("Wedding"), key("clip")),
		fmt.Sprintf(`INSERT INTO other_video (work_id, recorded_at, recorded_from, file_name, file_name_key) VALUES
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
