// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"errors"
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/library/librarytest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

const backupPlace = "/backup/Metropolis.mkv"

// placed is a shelf whose film Metropolis is found at two places: its first under Films (library 1), its
// copy under Backup, another films library.
type placed struct {
	*shelf
	backupID int64
	titleID  int64 // Metropolis
	fileID   int64 // Metropolis's file
}

func placedFilm(t *testing.T) placed {
	t.Helper()
	world := newShelf(t)
	backupID := librarytest.LibraryOn(t, world.database, world.nodeID,
		librarytest.Library{Name: "Backup", Kind: "movies", MetadataSource: "nfo", Root: "/backup"})
	world.scan([]nodeproto.ScanRecord{observedRecord("Metropolis.mkv", "heat")}, true)
	titleID := world.fileAsFilm("Metropolis", "Metropolis.mkv")
	fileID := world.liveFilesByPath()[root+"/Metropolis.mkv"].ID
	librarytest.File(t, world.database, librarytest.FileSpec{
		LibraryID: backupID, NodeID: world.nodeID, Path: backupPlace, Filing: "copy", CopyOf: fileID,
		FirstSeenAt: 3,
	})
	return placed{shelf: world, backupID: backupID, titleID: titleID, fileID: fileID}
}

// D363 §6, §7, §8: a film whose file two libraries cover, one at each of its places, is listed once
// in every list, and to a person reaching either library; one who reaches neither reaches it nowhere.
func TestBrowse_AFilmAtTwoPlacesOfTwoLibrariesIsListedOnceToWhoeverReachesEither(t *testing.T) {
	t.Parallel()
	world := placedFilm(t)
	backupID, titleID := world.backupID, world.titleID
	for _, reach := range []auth.Reach{auth.EveryLibrary(), auth.LibrariesOnly(1), auth.LibrariesOnly(backupID)} {
		page, err := world.browser.List(t.Context(),
			ListQuery{Reach: reach, Kind: ListFilms, Sort: SortName, Limit: 10})
		if err != nil || !slices.Equal(entryNames(page), []string{"Metropolis"}) {
			t.Errorf("films reached by %+v = %v, %v; want Metropolis once", reach, entryNames(page), err)
		}
	}
	nowhere := auth.LibrariesOnly(99)
	page, err := world.browser.List(t.Context(), ListQuery{Reach: nowhere, Kind: ListFilms, Sort: SortName, Limit: 10})
	if err != nil || len(page.Entries) != 0 {
		t.Errorf("films reached by no library of theirs = %v, %v; want none", entryNames(page), err)
	}
	if _, err := world.browser.Title(t.Context(), nowhere, 1, titleID); !errors.Is(err, ErrNoItem) {
		t.Errorf("the film to one who reaches neither library: %v; want ErrNoItem", err)
	}
}

// D363 §8: a title's detail names only the places its person reaches, the first first; an admin's
// names every place.
func TestBrowse_ATitleShowsOnlyThePlacesItsPersonReaches(t *testing.T) {
	t.Parallel()
	world := placedFilm(t)
	backupID, titleID := world.backupID, world.titleID
	for _, tc := range []struct {
		reach auth.Reach
		want  []string
	}{
		{auth.EveryLibrary(), []string{root + "/Metropolis.mkv", backupPlace}},
		{auth.LibrariesOnly(1), []string{root + "/Metropolis.mkv"}},
		{auth.LibrariesOnly(backupID), []string{backupPlace}},
	} {
		detail, err := world.browser.Title(t.Context(), tc.reach, 1, titleID)
		if err != nil || len(detail.Versions) != 1 || len(detail.Versions[0].Files) != 1 {
			t.Fatalf("the title to %+v = %+v, %v; want one version of one file", tc.reach, detail, err)
		}
		file := detail.Versions[0].Files[0]
		if file.Path != tc.want[0] || !slices.Equal(file.Paths, tc.want) {
			t.Errorf("to %+v the file is at %s, %v; want %v", tc.reach, file.Path, file.Paths, tc.want)
		}
	}
}

// D363 §8: a play reads from a place its person reaches, the first place before a copy, and falls
// back, when that place cannot be read, only to another place they reach.
func TestPlaceToRead_IsAReachedPlaceAndFallsBackOnlyAmongThem(t *testing.T) {
	t.Parallel()
	world := placedFilm(t)
	backupID, fileID := world.backupID, world.fileID
	file, err := world.database.Read().GetPlayableFile(t.Context(), fileID)
	if err != nil {
		t.Fatal(err)
	}
	connections := &fakeNodes{id: world.nodeID}
	placeOf := func(reach auth.Reach) string {
		t.Helper()
		located, err := PlaceToRead(t.Context(), world.database.Read(), connections, reach, file)
		if err != nil {
			t.Fatalf("the place to read for %+v: %v", reach, err)
		}
		return string(located.ReportedPathRaw)
	}
	if got := placeOf(auth.EveryLibrary()); got != root+"/Metropolis.mkv" {
		t.Errorf("for every library the place read is %s; want the first place", got)
	}
	if got := placeOf(auth.LibrariesOnly(backupID)); got != backupPlace {
		t.Errorf("for Backup alone the place read is %s; want its copy", got)
	}
	if _, err := PlaceToRead(t.Context(), world.database.Read(), connections, auth.LibrariesOnly(99), file); !errors.Is(
		err, ErrNoFile) {
		t.Errorf("for no library of the file's: %v; want ErrNoFile", err)
	}
	storetest.ExecRaw(t, world.database.Path(),
		"INSERT INTO unavailable_place (node_id, path, root_path, since, cause) VALUES (1, '"+root+"', '"+root+
			"', 5000, 'unread')")
	if got := placeOf(auth.EveryLibrary()); got != backupPlace {
		t.Errorf("with the first place's disk gone, for every library the place read is %s; want the copy", got)
	}
	if got := placeOf(auth.LibrariesOnly(1)); got != root+"/Metropolis.mkv" {
		t.Errorf("with the first place's disk gone, for Films alone the place read is %s; want its own, which "+
			"says why it cannot be read, and never the copy it does not reach", got)
	}
}

// D363 §3, §8: a subtitle beside a copy belongs to that place: it is offered with the file to whoever
// reaches the copy's place, and to no one else.
func TestSubtitleTracks_ASubtitleBesideACopyIsOfferedWhereItsPlaceIsReached(t *testing.T) {
	t.Parallel()
	world := placedFilm(t)
	backupID, fileID := world.backupID, world.fileID
	copyID := world.liveFilesByPath()[backupPlace].ID
	subtitle := librarytest.File(t, world.database, librarytest.FileSpec{
		NodeID: world.nodeID, Path: "/backup/Metropolis.en.srt", SidecarKind: "subtitle", AttachedToFile: copyID,
		FirstSeenAt: 3,
	})
	librarytest.Stream(t, world.database, librarytest.StreamSpec{
		BytesFile: subtitle, VideoFile: fileID, Kind: "subtitle", Codec: "subrip", Language: "en",
	})
	for _, tc := range []struct {
		reach auth.Reach
		want  int
	}{{auth.EveryLibrary(), 1}, {auth.LibrariesOnly(backupID), 1}, {auth.LibrariesOnly(1), 0}} {
		isEvery, libraryIDs := tc.reach.QueryArgs()
		tracks, err := world.database.Read().ListSubtitleTracksOfVideo(t.Context(),
			sqlcgen.ListSubtitleTracksOfVideoParams{
				VideoFile: fileID, IsEveryLibrary: isEvery, LibraryIdsJSON: libraryIDs,
			})
		if err != nil || len(tracks) != tc.want {
			t.Errorf("the subtitles offered to %+v = %+v, %v; want %d", tc.reach, tracks, err, tc.want)
		}
	}
}
