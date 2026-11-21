// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"fmt"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/library/librarytest"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// seriesWithAnExtra makes a series whose aired ordering is S1E1, S1E2, S2E1 and two specials last, as
// aired places them, each with a live file, and an extra of S1E1 with a file of its own; it returns
// the files, by the names "S1E1", "S1E2", "S2E1", "special", "special 2" and "extra", and the titles
// by the same.
func seriesWithAnExtra(t *testing.T, world *shelf) (files, titles map[string]int64) {
	t.Helper()
	return seriesWithAnExtraIn(t, world, 1)
}

// seriesWithAnExtraIn is seriesWithAnExtra in the library libraryID.
func seriesWithAnExtraIn(t *testing.T, world *shelf, libraryID int64) (files, titles map[string]int64) {
	t.Helper()
	seriesID := librarytest.Item(t, world.database, librarytest.ItemSpec{Kind: "series", LibraryID: libraryID})
	statements := []string{
		fmt.Sprintf("INSERT INTO sequence (id, kind, created_by) VALUES (%d, 'series', "+
			"'scanner')", seriesID),
		fmt.Sprintf("INSERT INTO ordering (id, sequence_id, name) VALUES (%d, %d, 'aired')", seriesID, seriesID),
	}
	files, titles = map[string]int64{}, map[string]int64{}
	for _, each := range []struct {
		name                   string
		kind, filing           string
		season, number, sortAt int64
	}{
		{"S1E1", "episode", "listed", 1, 1, 101},
		{"S1E2", "episode", "listed", 1, 2, 102},
		{"S2E1", "episode", "listed", 2, 1, 201},
		{"special", "episode", "listed", 0, 1, 900001},
		{"special 2", "episode", "listed", 0, 2, 900002},
		{"extra", "extra", "extra", 0, 0, 0},
	} {
		title := librarytest.Title(t, world.database, librarytest.TitleSpec{Library: libraryID, Kind: each.kind})
		version := librarytest.Version(t, world.database, librarytest.VersionSpec{TitleID: title})
		files[each.name] = librarytest.File(t, world.database, librarytest.FileSpec{
			LibraryID: libraryID, NodeID: world.nodeID, Path: fmt.Sprintf("%s/%s.mkv", world.rootPath, each.name),
			VersionID: version, Filing: each.filing,
		})
		titles[each.name] = title
		if each.kind == "episode" {
			statements = append(statements, storetest.Positions(fmt.Sprintf("(%d, %d, %d, %d, %d)",
				seriesID, title, each.season, each.number, each.sortAt)))
		}
	}
	statements = append(statements, fmt.Sprintf("INSERT INTO extra_relation (owner_work, extra_work, extra_type, "+
		"asserted_by) VALUES (%d, %d, 'featurette', 'scanner')", titles["S1E1"], titles["extra"]))
	storetest.ExecRaw(t, world.database.Path(), statements...)
	return files, titles
}

// A file's neighbours in its sequence are the members either side of its title in the ordering its
// person watches its series in (D407 §2): none before the first and none after the last, the specials
// a sequence of their own apart from the episodes (the owner, 2026-11-03), and an extra, which holds
// no place, neither in it nor with any.
func TestNeighboursOf_AreTheMembersEitherSideInTheSequence(t *testing.T) {
	t.Parallel()
	world := newShelf(t)
	files, titles := seriesWithAnExtra(t, world)
	name := map[int64]string{}
	for each, title := range titles {
		name[title] = each
	}
	seen := func(neighbours Neighbours) string {
		previous, next := "none", "none"
		if neighbours.Previous != nil {
			previous = fmt.Sprintf("%s file %d", name[neighbours.Previous.TitleID], neighbours.Previous.FileID)
		}
		if neighbours.Next != nil {
			next = fmt.Sprintf("%s file %d", name[neighbours.Next.TitleID], neighbours.Next.FileID)
		}
		return previous + " < > " + next
	}
	at := func(each string) string { return fmt.Sprintf("%s file %d", each, files[each]) }
	for around, want := range map[string]string{
		"S1E1":      "none < > " + at("S1E2"),
		"S1E2":      at("S1E1") + " < > " + at("S2E1"),
		"S2E1":      at("S1E2") + " < > none",
		"special":   "none < > " + at("special 2"),
		"special 2": at("special") + " < > none",
		"extra":     "none < > none",
	} {
		neighbours, err := world.browser.NeighboursOf(t.Context(), auth.EveryLibrary(), 1,
			Opened{FileID: files[around]})
		if err != nil {
			t.Fatal(err)
		}
		if got := seen(neighbours); got != want {
			t.Errorf("around %s: %s; want %s", around, got, want)
		}
		next, hasNext, err := world.browser.UpNext(t.Context(), auth.EveryLibrary(), 1, Opened{FileID: files[around]})
		if err != nil {
			t.Fatal(err)
		}
		if hasNext != (neighbours.Next != nil) || hasNext && next.TitleID != neighbours.Next.TitleID {
			t.Errorf("after %s, Up next (%v, title %d) and the neighbours' next %+v differ", around, hasNext,
				next.TitleID, neighbours.Next)
		}
	}
}

// One file's bytes filed both as a film and as an episode (the same copy in a films library and a
// series library) play as the title they were opened from (B0 question 20): opened from the series,
// the episode has its neighbours, so its Up next plays and a Watch Together group plays on; opened
// as the film, or as no title in particular, the film it is filed as first, which has none.
func TestNeighboursOf_AFileFiledAsAFilmAndAnEpisodePlaysAsTheTitleItWasOpenedFrom(t *testing.T) {
	t.Parallel()
	world := newShelf(t)
	shows := librarytest.LibraryOn(t, world.database, world.nodeID,
		librarytest.Library{Name: "Shows", Kind: "series", MetadataSource: "nfo", Root: world.rootPath})
	files, titles := seriesWithAnExtraIn(t, world, shows)
	film := librarytest.Title(t, world.database, librarytest.TitleSpec{Library: 1, Kind: "movie"})
	filmVersion := librarytest.Version(t, world.database, librarytest.VersionSpec{TitleID: film})
	both := files["S1E1"]
	// The series library files the episode, and the films library over the same folder the same
	// bytes as the film.
	storetest.ExecRaw(t, world.database.Path(), fmt.Sprintf("INSERT INTO file_filing (file_id, library_kind, "+
		"filing, version_id) VALUES (%d, 'movie', 'listed', %d)", both, filmVersion))
	for _, each := range []struct {
		openedAs, wantTitle, wantNext int64
	}{
		{titles["S1E1"], titles["S1E1"], titles["S1E2"]},
		{film, film, 0},
		{0, film, 0},
		{titles["S2E1"], film, 0}, // a title the file is not filed as is no title it plays as
	} {
		opened := Opened{FileID: both, TitleID: each.openedAs}
		work, err := world.browser.WorkOfFile(t.Context(), auth.EveryLibrary(), opened)
		if err != nil {
			t.Fatal(err)
		}
		neighbours, err := world.browser.NeighboursOf(t.Context(), auth.EveryLibrary(), 1, opened)
		if err != nil {
			t.Fatal(err)
		}
		var next int64
		if neighbours.Next != nil {
			next = neighbours.Next.TitleID
		}
		if work.TitleID != each.wantTitle || next != each.wantNext {
			t.Errorf("opened as title %d: plays as title %d, with title %d next; want title %d, with %d next",
				each.openedAs, work.TitleID, next, each.wantTitle, each.wantNext)
		}
	}
}
