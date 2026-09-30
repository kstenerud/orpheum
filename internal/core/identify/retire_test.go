// SPDX-License-Identifier: GPL-3.0-or-later

package identify_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/core/identify/identifytest"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// noWork is a job queue with nothing to cancel.
type noWork struct{}

func (noWork) CancelRetired(context.Context, jobs.RetiredWork) (int, error) { return 0, nil }

// deleteAndStartAnother deletes the library as an admin does (D231 §7), and makes another, Kids,
// over another root of the node, which the test library is from then on.
func (lib *testLibrary) deleteAndStartAnother(source string) {
	lib.t.Helper()
	admin := library.NewAdmin(lib.database, lib.clock, nil, library.Retirement{Work: noWork{}, Titles: lib.pass})
	if _, err := admin.Delete(lib.t.Context(), lib.libraryID); err != nil {
		lib.t.Fatal(err)
	}
	const kids = "/media/kids"
	err := lib.database.Write(lib.t.Context(), func(queries *sqlcgen.Queries) error {
		kidsID, err := queries.InsertLibrary(lib.t.Context(), sqlcgen.InsertLibraryParams{
			Name: "Kids", Kind: "movies", MetadataSource: source, ChangeDetection: "manual", CreatedAt: 1,
		})
		if err != nil {
			return fmt.Errorf("library: %w", err)
		}
		lib.libraryID, lib.rootPath = kidsID, kids
		return queries.InsertLibraryRoot(lib.t.Context(), sqlcgen.InsertLibraryRootParams{
			LibraryID: kidsID, NodeID: lib.nodeID, Path: kids,
		})
	})
	if err != nil {
		lib.t.Fatal(err)
	}
}

// isLive is whether an item is live, not a tombstone.
func (lib *testLibrary) isLive(itemID int64) bool {
	lib.t.Helper()
	_, err := store.OrNotFound(lib.database.Read().GetTitleForMatch(lib.t.Context(), itemID))
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		lib.t.Fatal(err)
	}
	return err == nil
}

// S24, D231 §7, D208 §2: a deleted library's film that reappears in another library, named by
// the provider id its NFO gives, brings its title back, with its history.
func TestPass_ADeletedLibrarysFilmThatReappearsElsewhereBringsItsTitleBack(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movies", "nfo")
	const body = `<movie><title>The Kid</title><year>2014</year><uniqueid type="tmdb">480600</uniqueid></movie>`
	const film, nfo = "The Kid (2014)/The Kid (2014).mkv", "The Kid (2014)/The Kid (2014).nfo"
	lib.scan(videoRecord(film), nfoRecord(nfo, body))
	the kid := lib.titlesByPath()[film].TitleID
	lib.deleteAndStartAnother("nfo")
	if lib.isLive(the kid) {
		t.Fatal("the deleted library's title is live; it should be a tombstone")
	}
	lib.scan(videoRecord(film), nfoRecord(nfo, body))
	if got := lib.titlesByPath()[film].TitleID; got != the kid || !lib.isLive(the kid) {
		t.Errorf("The Kid in Kids is title %d (live %v), want %d brought back", got, lib.isLive(got), the kid)
	}
}

// S24, D231 §7, D208 §2: a deleted library's film that reappears in another library and is
// matched there to the same work brings its title back, with its history.
func TestMatch_ADeletedLibrarysFilmMatchedAgainElsewhereBringsItsTitleBack(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movies", "tmdb")
	lib.provider.Searches[identifytest.SearchKey("Metropolis", 1995)] = []identify.Candidate{lib.provider.Add(heat)}
	const film = "Metropolis (1995).mkv"
	lib.scan(videoRecord(film))
	lib.matchFileTitle(film)
	metropolisTitle := lib.titlesByPath()[film].TitleID
	lib.deleteAndStartAnother("tmdb")
	lib.scan(videoRecord(film))
	if newTitle := lib.titlesByPath()[film].TitleID; newTitle == metropolisTitle {
		t.Fatal("the film was filed under its old title before any match; the test proves nothing")
	}
	lib.matchFileTitle(film)
	if got := lib.titlesByPath()[film].TitleID; got != metropolisTitle || !lib.isLive(metropolisTitle) {
		t.Errorf("Metropolis in Kids is title %d (live %v), want %d brought back by the match",
			got, lib.isLive(got), metropolisTitle)
	}
}
