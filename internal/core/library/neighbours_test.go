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
	seriesID := librarytest.Item(t, world.database, librarytest.ItemSpec{Kind: "collection", LibraryID: 1})
	statements := []string{
		fmt.Sprintf("INSERT INTO collection (id, kind, created_by) VALUES (%d, 'series', "+
			"'scanner')", seriesID),
		fmt.Sprintf("INSERT INTO ordering (id, collection_id, name) VALUES (%d, %d, 'aired')", seriesID, seriesID),
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
		title := librarytest.Title(t, world.database, librarytest.TitleSpec{Library: 1, Kind: each.kind})
		version := librarytest.Version(t, world.database, librarytest.VersionSpec{TitleID: title})
		files[each.name] = librarytest.File(t, world.database, librarytest.FileSpec{
			LibraryID: 1, NodeID: world.nodeID, Path: fmt.Sprintf("%s/%s.mkv", world.rootPath, each.name),
			VersionID: version, Filing: each.filing,
		})
		titles[each.name] = title
		if each.kind == "episode" {
			statements = append(statements, fmt.Sprintf("INSERT INTO position (ordering_id, title_id, group_number, "+
				"number_in_group, sort_order) VALUES (%d, %d, %d, %d, %d)", seriesID, title, each.season, each.number,
				each.sortAt))
		}
	}
	statements = append(statements, fmt.Sprintf("INSERT INTO extra_relation (owner_title, extra_title, extra_type, "+
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
		neighbours, err := world.browser.NeighboursOf(t.Context(), auth.EveryLibrary(), 1, files[around])
		if err != nil {
			t.Fatal(err)
		}
		if got := seen(neighbours); got != want {
			t.Errorf("around %s: %s; want %s", around, got, want)
		}
		next, hasNext, err := world.browser.UpNext(t.Context(), auth.EveryLibrary(), 1, files[around])
		if err != nil {
			t.Fatal(err)
		}
		if hasNext != (neighbours.Next != nil) || hasNext && next.TitleID != neighbours.Next.TitleID {
			t.Errorf("after %s, Up next (%v, title %d) and the neighbours' next %+v differ", around, hasNext,
				next.TitleID, neighbours.Next)
		}
	}
}
