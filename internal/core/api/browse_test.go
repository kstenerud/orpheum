// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/auth"
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
	storetest.ExecRaw(t, harness.database,
		"INSERT INTO node (id, name, enrolled_at) VALUES (1, 'local', 1)",
		`INSERT INTO library (id, name, kind, metadata_source, change_detection, is_writable, created_at)
		 VALUES (1, 'Films', 'mixed', 'nfo', 'manual', 0, 1)`,
		`INSERT INTO item (id, kind, library_id, created_at)
		 VALUES (10, 'title', 1, 1), (20, 'collection', 1, 2), (21, 'title', 1, 2)`,
		`INSERT INTO title (id, kind, year, identification)
		 VALUES (10, 'movie', 1995, 'identified'), (21, 'episode', 2008, 'identified')`,
		`INSERT INTO collection (id, kind, membership, created_by, identification)
		 VALUES (20, 'series', 'explicit', 'scanner', 'identified')`,
		`INSERT INTO item_text (item_id, lang, is_original, name, overview, search_alternates) VALUES
		 (10, 'und', 1, 'Metropolis', 'A crime saga.', 'heat'), (20, 'und', 1, 'Brass Arena', NULL, 'brass arena'),
		 (21, 'und', 1, 'Pilot', NULL, 'pilot')`,
		fmt.Sprintf(`INSERT INTO sort_key (item_id, locale, listing, collation_key)
		 VALUES (10, 'en', 'movie', %s), (20, 'en', 'series', %s)`,
			key("Metropolis"), key("Brass Arena")),
		"INSERT INTO version (id, title_id, created_at) VALUES (1, 10, 1), (2, 21, 2)",
		`INSERT INTO file (id, library_id, node_id, path, size_bytes, filing, version_id, first_seen_at, last_seen_at)
		 VALUES
		 (1, 1, 1, '/m/Metropolis (1995).mkv', 1000, 'listed', 1, 1, 1),
		 (2, 1, 1, '/m/BB/S01E01.mkv', 10, 'listed', 2, 1, 1)`,
		`INSERT INTO stream (bytes_file, video_file, stream_index, kind, codec, lang, channel_count) VALUES
		 (1, 1, 0, 'video', 'h264', NULL, NULL), (1, 1, 1, 'audio', 'truehd', 'en', 8)`,
		"INSERT INTO ordering (id, collection_id, name) VALUES (1, 20, 'aired')",
		"INSERT INTO position (ordering_id, title_id, group_number, number_in_group, sort_order) "+
			"VALUES (1, 21, 1, 1, 10001)",
		"INSERT INTO collection_member (collection_id, member_title, position) VALUES (20, 21, 10001)",
		// Metropolis's poster is a person's choice over a newer one TMDB named; its backdrop was refused.
		`INSERT INTO artwork (item_id, kind, source, source_ref, content_hash, is_chosen, removed_at) VALUES
		 (10, 'poster', 'user', NULL, X'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa', 1, NULL),
		 (10, 'poster', 'tmdb', '/p.jpg', X'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb', 0, NULL),
		 (10, 'backdrop', 'tmdb', '/b.jpg', NULL, 0, NULL),
		 (20, 'poster', 'tmdb', '/old.jpg', X'eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee', 0, 5),
		 (20, 'poster', 'tmdb', '/s.jpg', X'cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc', 0, NULL),
		 (21, 'thumb', 'tmdb', '/e.jpg', X'dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd', 0, NULL)`)
}

func TestBrowse_ListsDetailsSeriesAndSearch(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelve(t)
	if _, err := harness.service.CreateUser(t.Context(),
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
	episode := harness.do(t, http.MethodGet, "/items/21", user, nil)
	if !strings.Contains(episode.body,
		`"episode":{"seriesId":20,"seriesName":"Brass Arena","seasonNumber":1,"episodeNumber":1}`) {
		t.Errorf("an episode's detail = %s", episode.body)
	}
	wantError(t, harness.do(t, http.MethodGet, "/items/99", user, nil), http.StatusNotFound, CodeNotFound)

	series := harness.do(t, http.MethodGet, "/collections/20", user, nil)
	if !strings.Contains(series.body, `"name":"Pilot"`) || !strings.Contains(series.body, `"hasLiveFile":true`) {
		t.Errorf("a series = %d %s", series.status, series.body)
	}

	found := harness.do(t, http.MethodGet, "/search?q=breaking&limit=5", user, nil)
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
		{"/titles?kind=movie&sort=name&limit=10", `"posterHash":"` + chosen + `"`},
		{"/titles?kind=series&sort=added&limit=10", `"posterHash":"` + seriesPoster + `"`},
		{"/items/10", `"art":{"posterHash":"` + chosen + `"}`}, // and no refused backdrop
		{"/collections/20", `"stillHash":"` + still + `"`},
		{"/collections/20", `"art":{"posterHash":"` + seriesPoster + `"}`},
	} {
		if answer := harness.do(t, http.MethodGet, check.path, user, nil); !strings.Contains(answer.body, check.want) {
			t.Errorf("%s = %d %s; want %s", check.path, answer.status, answer.body, check.want)
		}
	}
}
