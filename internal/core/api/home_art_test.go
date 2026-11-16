// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/core/library/librarytest"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// A row of Continue Watching says what its work is and what its poster's place shows with no poster,
// so its card keeps the rule a list's card keeps (D496, D497): an other video's 16:9 picture, picked
// as its card in a list picks it (a frame still only where it has no backdrop), shown alone; a film's
// still above the mark of a missing poster, and an episode's, where neither it nor its series has a
// poster; neither once a poster arrives.
func TestContinueWatching_ARowCarriesItsKindAndThePictureItShowsWithNoPoster(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelve(t)
	librarytest.Title(t, harness.database, librarytest.TitleSpec{ID: 40, Library: 1, Kind: "other", AddedAt: 3})
	librarytest.Title(t, harness.database, librarytest.TitleSpec{ID: 50, Library: 1, AddedAt: 3})
	librarytest.Version(t, harness.database, librarytest.VersionSpec{ID: 40, TitleID: 40, CreatedAt: 3})
	librarytest.Version(t, harness.database, librarytest.VersionSpec{ID: 50, TitleID: 50, CreatedAt: 3})
	librarytest.File(t, harness.database, librarytest.FileSpec{
		ID: 40, LibraryID: 1, NodeID: 1, Path: "/m/home video.mkv", SizeBytes: 40, PayloadHash: []byte{0x40},
		Filing: "listed", VersionID: 40,
	})
	librarytest.File(t, harness.database, librarytest.FileSpec{
		ID: 50, LibraryID: 1, NodeID: 1, Path: "/m/Nosferatu (1922).mkv", SizeBytes: 50, PayloadHash: []byte{0x50},
		Filing: "listed", VersionID: 50,
	})
	otherBackdrop, filmBackdrop := strings.Repeat("41", 32), strings.Repeat("51", 32)
	storetest.ExecRaw(t, harness.databasePath,
		`INSERT INTO artwork (item_id, kind, source, source_ref, source_sha256, is_chosen, removed_at) VALUES
		 (40, 'still', 'frame', NULL, X'4040404040404040404040404040404040404040404040404040404040404040', 0, NULL),
		 (40, 'backdrop', 'local', NULL, X'`+otherBackdrop+`', 0, NULL),
		 (50, 'backdrop', 'tmdb', '/n.jpg', X'`+filmBackdrop+`', 0, NULL)`)
	harness.setPositionReadAt(t, 2, 60_000, 100)  // the pilot of Brass Arena
	harness.setPositionReadAt(t, 40, 20_000, 200) // the other video, of unknown length
	harness.setPositionReadAt(t, 50, 30_000, 300) // the film with no poster

	byID := func() map[int64]api.InProgressBody {
		rows := map[int64]api.InProgressBody{}
		for _, row := range harness.continueWatching(t, user) {
			rows[row.ID] = row
		}
		return rows
	}
	rows := byID()
	if other := rows[40]; other.Kind != "other" || other.NoPosterStillSha256 != otherBackdrop {
		t.Errorf("the other video's row = %+v; want kind other, and its backdrop to show alone", other)
	}
	if film := rows[50]; film.Kind != "movie" || film.NoPosterStillSha256 != filmBackdrop {
		t.Errorf("the film's row = %+v; want kind movie, and its backdrop above the mark", film)
	}
	if episode := rows[21]; episode.Kind != "episode" || episode.NoPosterStillSha256 != "" ||
		episode.PosterSha256 != strings.Repeat("cc", 32) {
		t.Errorf("the pilot's row = %+v; want kind episode, its series' poster and no still", episode)
	}
	// With no poster, its own nor its series', an episode is drawn as a film: its still above the mark.
	storetest.ExecRaw(t, harness.databasePath, "UPDATE artwork SET removed_at = 6 WHERE item_id = 20")
	episode := byID()[21]
	if episode.PosterSha256 != "" || episode.NoPosterStillSha256 != strings.Repeat("dd", 32) {
		t.Errorf("the pilot's row with no poster = %+v; want its own still", episode)
	}

	storetest.ExecRaw(t, harness.databasePath,
		`INSERT INTO artwork (item_id, kind, source, source_ref, source_sha256, is_chosen, removed_at) VALUES
		 (40, 'poster', 'local', NULL, X'`+strings.Repeat("42", 32)+`', 0, NULL),
		 (50, 'poster', 'tmdb', '/p.jpg', X'`+strings.Repeat("52", 32)+`', 0, NULL)`)
	rows = byID()
	for _, id := range []int64{40, 50} {
		if rows[id].PosterSha256 == "" || rows[id].NoPosterStillSha256 != "" {
			t.Errorf("with a poster, row %d = %+v; want its poster and no still", id, rows[id])
		}
	}
}
