// SPDX-License-Identifier: AGPL-3.0-or-later

package identify_test

import (
	"fmt"
	"maps"
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// seriesOfEach is each series' name by the titles of its episodes, live or not.
func (lib *testLibrary) seriesOfEach() map[int64]string {
	lib.t.Helper()
	series, err := lib.database.Read().ListLibrarySeries(lib.t.Context(), &lib.libraryID)
	if err != nil {
		lib.t.Fatal(err)
	}
	byTitle := map[int64]string{}
	for _, one := range series {
		places, err := lib.database.Read().ListSeriesEpisodePlaces(lib.t.Context(), one.ID)
		if err != nil {
			lib.t.Fatal(err)
		}
		for _, place := range places {
			if place.RemovedAt == nil {
				byTitle[place.TitleID] = *one.Name
			}
		}
	}
	return byTitle
}

// The owner's NAS, 2026-10-13: in a series library, featurettes, a film made of the series and
// episodes numbered without a season were filed as films, and each episode in a folder of its own
// was a series of its own. The owner: "when something is marked as a series, it should be
// impossible for any of its contents to be classified as films".
func TestPass_ASeriesLibraryFilesEveryVideoAsAnEpisodeOrASpecialNeverAFilm(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "shows", "nfo")
	prisoner := "The Wanderer (1967)/The Wanderer - 01 - The Bells of Saint Quill.mkv"
	featurette := "Brindlewood/Brindlewood Season 5  [1080p x265 10bit FS89 Joy]/Featurettes/Six Seasons and A Finale.mkv"
	episode := "Brindlewood/Brindlewood Season 5  [1080p x265 10bit FS89 Joy]/Brindlewood.S05E01.mkv"
	film := "Brackenmere/The Promised Land 2020/Brackenmere The Promised Land 2020 1080p BluRay HEVC x265 5.1 BONE.mkv"
	wing1 := "Wing Vellmoor/Wing.Vellmoor.S01E01.1080p.x265-LANTERN/Wing.Vellmoor.S01E01.1080p.x265-LANTERN.mkv"
	wing2 := "Wing Vellmoor/Wing.Vellmoor.S01E02.1080p.x265-LANTERN/Wing.Vellmoor.S01E02.1080p.x265-LANTERN.mkv"
	var records []nodeproto.ScanRecord
	for _, path := range []string{prisoner, featurette, episode, film, wing1, wing2} {
		records = append(records, videoRecord(path))
	}
	lib.scan(records...)
	titles := lib.titlesByPath()
	seriesOf := lib.seriesOfEach()
	for path, title := range titles {
		if title.Kind != "episode" {
			t.Errorf("%s is filed a %s; in a series library everything is an episode", path, title.Kind)
		}
	}
	want := map[string]string{
		prisoner: "The Wanderer", featurette: "Brindlewood", episode: "Brindlewood", film: "Brackenmere",
		wing1: "Wing Vellmoor", wing2: "Wing Vellmoor",
	}
	for path, series := range want {
		if got := seriesOf[titles[path].TitleID]; got != series {
			t.Errorf("%s is in series %q; want %q", path, got, series)
		}
	}
	if names := slices.Sorted(maps.Values(seriesOf)); len(slices.Compact(names)) != 4 {
		t.Errorf("series %v; want The Wanderer, Brindlewood, Brackenmere and Wing Vellmoor alone", slices.Compact(names))
	}
	if name := titleName(titles[film]); name != "Brackenmere The Promised Land" {
		t.Errorf("the film made of the series is named %q; want its file's name, as a special", name)
	}
	if titles[wing1].TitleID == titles[wing2].TitleID {
		t.Error("Wing Vellmoor's two episodes are one title")
	}
}

// Episodes an older reading of their folders put each in a series of its own, no one having
// matched or confirmed them, are filed again in the one series their folders now name, though no
// file moved: kept where they were, a rescan renamed each series back from the folders and left
// Wing Vellmoor as six series of one name (the owner's NAS, 2026-10-13).
func TestPass_EpisodesAnOlderReadingSplitIntoSeriesOfTheirOwnComeTogether(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "shows", "nfo")
	first, second := "Wing Vellmoor/Season 1/Wing Vellmoor - S01E01.mkv", "Wing Vellmoor/Season 1/Wing Vellmoor - S01E02.mkv"
	lib.scan(videoRecord(first), videoRecord(second))
	titles := lib.titlesByPath()
	seriesID := int64(0)
	for id := range lib.seriesNames() {
		seriesID = id
	}
	// As the older reading filed them: the second episode in a series of its own, each series named
	// for its episode's folder.
	split := fmt.Sprintf
	storetest.ExecRaw(t, lib.database.Path(),
		split(`INSERT INTO item (id, kind, library_id, added_at) VALUES (9001, 'collection', %d, 1)`, lib.libraryID),
		`INSERT INTO collection (id, kind, membership, created_by) VALUES (9001, 'series', 'explicit', 'scanner')`,
		split(`INSERT INTO item_text (item_id, language, is_original, name)
			SELECT 9001, language, is_original, 'Wing Vellmoor S01E02' FROM item_text WHERE item_id = %d`, seriesID),
		`INSERT INTO ordering (id, collection_id, name) VALUES (9001, 9001, 'aired')`,
		split(`UPDATE position SET ordering_id = 9001 WHERE title_id = %d`, titles[second].TitleID),
		split(`UPDATE collection_member SET collection_id = 9001 WHERE member_title = %d`, titles[second].TitleID),
		split(`UPDATE item_text SET name = 'Wing Vellmoor S01E01' WHERE item_id = %d`, seriesID),
	)
	if seriesOf := lib.seriesOfEach(); seriesOf[titles[first].TitleID] == seriesOf[titles[second].TitleID] {
		t.Fatal("the older reading's state was not built: the episodes share a series")
	}
	lib.scan(videoRecord(first), videoRecord(second))
	after := lib.titlesByPath()
	seriesOf := lib.seriesOfEach()
	one, other := seriesOf[after[first].TitleID], seriesOf[after[second].TitleID]
	if one != "Wing Vellmoor" || other != "Wing Vellmoor" {
		t.Errorf("the episodes are in series %q and %q; want Wing Vellmoor", one, other)
	}
	if lib.seriesIDOf(after[first].TitleID) != lib.seriesIDOf(after[second].TitleID) {
		t.Error("the episodes are in two series of one name; want them in one")
	}
	waiting, err := lib.database.Read().ListSeriesByIdentification(t.Context(),
		sqlcgen.ListSeriesByIdentificationParams{IdentificationState: ptr.NilIfZero("unidentified"), IsEveryLibrary: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(waiting) != 1 || ptr.ValueOrZero(waiting[0].Name) != "Wing Vellmoor" {
		t.Errorf("waiting for a match: %+v; want Wing Vellmoor alone, the emptied series waiting for no one", waiting)
	}
}

// seriesIDOf is the series a title is a live episode of, or 0.
func (lib *testLibrary) seriesIDOf(titleID int64) int64 {
	lib.t.Helper()
	for id := range lib.seriesNames() {
		places, err := lib.database.Read().ListSeriesEpisodePlaces(lib.t.Context(), id)
		if err != nil {
			lib.t.Fatal(err)
		}
		for _, place := range places {
			if place.TitleID == titleID && place.RemovedAt == nil {
				return id
			}
		}
	}
	return 0
}

// seriesNames is each series' name by its id.
func (lib *testLibrary) seriesNames() map[int64]string {
	lib.t.Helper()
	series, err := lib.database.Read().ListLibrarySeries(lib.t.Context(), &lib.libraryID)
	if err != nil {
		lib.t.Fatal(err)
	}
	names := map[int64]string{}
	for _, one := range series {
		names[one.ID] = *one.Name
	}
	return names
}
