// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/library/librarytest"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// storeArt stores an image of a kind from source for an item, and returns its source SHA-256 in hex.
func storeArt(t *testing.T, world *shelf, itemID int64, kind, source string, first byte) string {
	t.Helper()
	sha := make([]byte, 32)
	sha[0] = first
	err := world.database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
		_, err := queries.InsertArtwork(t.Context(), sqlcgen.InsertArtworkParams{
			ItemID: itemID, Kind: kind, Source: source, SourceSha256: sha,
		})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(sha)
}

// readPosters reads entries' posters, and what stands in for one, as a page of a list does.
func readPosters(t *testing.T, world *shelf, entries ...Entry) map[int64]Entry {
	t.Helper()
	page, err := world.browser.withPosters(t.Context(), Page{Entries: entries})
	if err != nil {
		t.Fatal(err)
	}
	byID := map[int64]Entry{}
	for _, entry := range page.Entries {
		byID[entry.ID] = entry
	}
	return byID
}

// A frame still is a last resort: an other video's tile shows it only when it has no still and no
// backdrop of its own (D362 §2).
func TestWithLandscapes_AFrameStillShowsOnlyWhereNoOtherPictureIs(t *testing.T) {
	t.Parallel()
	world := newShelf(t)
	frameAndBackdrop := librarytest.Item(t, world.database, librarytest.ItemSpec{LibraryID: 1})
	storeArt(t, world, frameAndBackdrop, ArtStill, ArtSourceFrame, 0x11)
	backdrop := storeArt(t, world, frameAndBackdrop, ArtBackdrop, "local", 0x12)
	frameAndStill := librarytest.Item(t, world.database, librarytest.ItemSpec{LibraryID: 1})
	// The real still first and the frame newer: the newest stored does not outrank a real picture.
	still := storeArt(t, world, frameAndStill, ArtStill, "tmdb", 0x22)
	storeArt(t, world, frameAndStill, ArtStill, ArtSourceFrame, 0x21)
	frameOnly := librarytest.Item(t, world.database, librarytest.ItemSpec{LibraryID: 1})
	frame := storeArt(t, world, frameOnly, ArtStill, ArtSourceFrame, 0x31)
	got := readPosters(t, world, Entry{ID: frameAndBackdrop, Kind: ListOther},
		Entry{ID: frameAndStill, Kind: ListOther}, Entry{ID: frameOnly, Kind: ListOther})
	for id, want := range map[int64]string{frameAndBackdrop: backdrop, frameAndStill: still, frameOnly: frame} {
		if got[id].LandscapeSha256 != want {
			t.Errorf("item %d's landscape: %q; want %q: any picture before a frame still", id,
				got[id].LandscapeSha256, want)
		}
	}
}

// A film or series with no poster shows a 16:9 picture in its poster's place: its backdrop, else a
// film's frame still, a series' first episode's still; read anew with each page, so a poster that
// arrives replaces it at the next read (the owner, 2026-10-27; D362 §2).
func TestWithPosters_APosterlessFilmOrSeriesShowsAStillUntilAPosterArrives(t *testing.T) {
	t.Parallel()
	world := newShelf(t)
	film := librarytest.Item(t, world.database, librarytest.ItemSpec{LibraryID: 1})
	frame := storeArt(t, world, film, ArtStill, ArtSourceFrame, 0x11)
	withBackdrop := librarytest.Item(t, world.database, librarytest.ItemSpec{LibraryID: 1})
	storeArt(t, world, withBackdrop, ArtStill, ArtSourceFrame, 0x21)
	backdrop := storeArt(t, world, withBackdrop, ArtBackdrop, "tmdb", 0x22)
	series, episodeStill := seriesWithEpisodes(t, world)
	entries := func() []Entry { // each read's page is read anew
		return []Entry{
			{ID: film, Kind: ListFilms}, {ID: withBackdrop, Kind: ListFilms}, {ID: series, Kind: ListSeries},
		}
	}
	got := readPosters(t, world, entries()...)
	for id, want := range map[int64]string{film: frame, withBackdrop: backdrop, series: episodeStill} {
		if got[id].NoPosterStillSha256 != want || got[id].PosterSha256 != "" {
			t.Errorf("item %d with no poster: still %q; want %q", id, got[id].NoPosterStillSha256, want)
		}
	}
	poster := storeArt(t, world, film, ArtPoster, "tmdb", 0x41)
	got = readPosters(t, world, entries()...)
	if got[film].PosterSha256 != poster || got[film].NoPosterStillSha256 != "" {
		t.Errorf("the film once its poster arrived: poster %q, still %q; want the poster and no still",
			got[film].PosterSha256, got[film].NoPosterStillSha256)
	}
}

// seriesWithEpisodes makes a series whose aired ordering has a special, a second season's episode and
// a first season's two, each with a live file and its own still, and returns the series and the
// still of the first season's first episode, which it shows with no poster.
func seriesWithEpisodes(t *testing.T, world *shelf) (seriesID int64, firstStill string) {
	t.Helper()
	seriesID = librarytest.Item(t, world.database, librarytest.ItemSpec{Kind: "collection", LibraryID: 1})
	statements := []string{
		fmt.Sprintf("INSERT INTO collection (id, kind, membership, created_by) VALUES (%d, 'series', 'explicit', "+
			"'scanner')", seriesID),
		fmt.Sprintf("INSERT INTO ordering (id, collection_id, name) VALUES (%d, %d, 'aired')", seriesID, seriesID),
	}
	for index, place := range []struct{ season, number int64 }{{0, 1}, {2, 1}, {1, 2}, {1, 1}} {
		episode := librarytest.Title(t, world.database, librarytest.TitleSpec{Library: 1, Kind: "episode"})
		version := librarytest.Version(t, world.database, librarytest.VersionSpec{TitleID: episode})
		librarytest.File(t, world.database, librarytest.FileSpec{
			LibraryID: 1, NodeID: world.nodeID, Path: fmt.Sprintf("%s/S%dE%d.mkv", world.rootPath, place.season,
				place.number), VersionID: version,
		})
		statements = append(statements, fmt.Sprintf("INSERT INTO position (ordering_id, title_id, group_number, "+
			"number_in_group, sort_order) VALUES (%d, %d, %d, %d, %d)", seriesID, episode, place.season, place.number,
			place.season*100+place.number))
		still := storeArt(t, world, episode, ArtStill, ArtSourceFrame, byte(0x50+index))
		if place.season == 1 && place.number == 1 {
			firstStill = still
		}
	}
	storetest.ExecRaw(t, world.database.Path(), statements...)
	return seriesID, firstStill
}
