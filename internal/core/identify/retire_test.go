// SPDX-License-Identifier: AGPL-3.0-or-later

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
// of kind and source over another root of the node, which the test library is from then on.
func (lib *testLibrary) deleteAndStartAnother(kind, source string) {
	lib.t.Helper()
	admin := library.NewAdmin(lib.database, lib.clock, nil, nil, library.Retirement{Work: noWork{}})
	if _, err := admin.Delete(lib.t.Context(), lib.libraryID); err != nil {
		lib.t.Fatal(err)
	}
	const kids = "/media/kids"
	err := lib.database.Write(lib.t.Context(), func(queries *sqlcgen.Queries) error {
		kidsID, err := queries.InsertLibrary(lib.t.Context(), sqlcgen.InsertLibraryParams{
			Name: "Kids", Kind: kind, MetadataSource: source, ChangeDetection: "manual", CreatedAt: 1,
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

// isListed is whether a library lists a title: covers a live place of one of its files (D363 §4).
func (lib *testLibrary) isListed(titleID int64) bool {
	lib.t.Helper()
	isReached, err := lib.database.Read().IsItemReached(lib.t.Context(), sqlcgen.IsItemReachedParams{
		ItemID: titleID, IsEveryLibrary: 1, LibraryIdsJSON: "[]",
	})
	if err != nil {
		lib.t.Fatal(err)
	}
	return isReached == 1
}

// isLive is whether an item is live, not a tombstone.
func (lib *testLibrary) isLive(itemID int64) bool {
	lib.t.Helper()
	_, err := store.OrNotFound(lib.database.Read().GetWorkForMatch(lib.t.Context(), itemID))
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		lib.t.Fatal(err)
	}
	return err == nil
}

// S24, D231 §7, D208 §2: a deleted library's film that reappears in another library, named by
// the provider id its NFO gives, is its title still, with its history. Deleting the library deleted no
// item (D363 §10): the title stayed, listed by no library, until one covers its file again.
func TestPass_ADeletedLibrarysFilmThatReappearsElsewhereBringsItsTitleBack(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movie", "nfo")
	const body = `<movie><title>The Kid</title><year>2014</year><uniqueid type="tmdb">480600</uniqueid></movie>`
	const film, nfo = "The Kid (2014)/The Kid (2014).mkv", "The Kid (2014)/The Kid (2014).nfo"
	lib.scan(videoRecord(film), nfoRecord(nfo, body))
	the kid := lib.titlesByPath()[film].WorkID
	lib.deleteAndStartAnother("movie", "nfo")
	if !lib.isLive(the kid) {
		t.Fatal("the deleted library's title is a tombstone; deleting a library deletes no item (D363 §10)")
	}
	lib.scan(videoRecord(film), nfoRecord(nfo, body))
	if got := lib.titlesByPath()[film].WorkID; got != the kid || !lib.isLive(the kid) {
		t.Errorf("The Kid in Kids is title %d (live %v), want %d brought back", got, lib.isLive(got), the kid)
	}
}

// S24, D231 §7, D208 §2: a deleted library's film that reappears in another library and is
// matched there to the same work brings its title back, with its history.
func TestMatch_ADeletedLibrarysFilmMatchedAgainElsewhereBringsItsTitleBack(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movie", "tmdb")
	lib.provider.Searches[identifytest.SearchKey("Metropolis", 1995)] = []identify.Candidate{lib.provider.Add(heat)}
	const film = "Metropolis (1995).mkv"
	lib.scan(videoRecord(film))
	lib.matchFileTitle(film)
	metropolisTitle := lib.titlesByPath()[film].WorkID
	lib.deleteAndStartAnother("movie", "tmdb")
	lib.scan(videoRecord(film))
	if newTitle := lib.titlesByPath()[film].WorkID; newTitle == metropolisTitle {
		t.Fatal("the film was filed under its old title before any match; the test proves nothing")
	}
	lib.matchFileTitle(film)
	if got := lib.titlesByPath()[film].WorkID; got != metropolisTitle || !lib.isLive(metropolisTitle) {
		t.Errorf("Metropolis in Kids is title %d (live %v), want %d brought back by the match",
			got, lib.isLive(got), metropolisTitle)
	}
}

// seriesOfFile is the series a file's episode is placed in.
func (lib *testLibrary) seriesOfFile(rel string) int64 {
	lib.t.Helper()
	place, err := lib.database.Read().GetEpisodePlace(lib.t.Context(), lib.titlesByPath()[rel].WorkID)
	if err != nil {
		lib.t.Fatalf("%s's place: %v", rel, err)
	}
	return place.SequenceID
}

// S24, D231 §7, D208 §2: a deleted library's series whose episodes reappear in another library,
// named by the provider id its tvshow.nfo gives, comes back with its history; so does each
// episode whose file reappears, and an episode whose file does not is kept, listed by no library
// (D363 §10).
func TestPass_ADeletedLibrarysSeriesThatReappearsElsewhereComesBack(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "series", "nfo")
	const show = `<tvshow><title>Puddlejump</title><year>2018</year><uniqueid type="tmdb">82728</uniqueid></tvshow>`
	const first, second = "Puddlejump/Season 01/Puddlejump - S01E01.mkv", "Puddlejump/Season 01/Puddlejump - S01E02.mkv"
	lib.scan(nfoRecord("Puddlejump/tvshow.nfo", show), videoRecord(first), videoRecord(second))
	puddlejump := lib.onlySeries().ID
	magicXylophone, hospital := lib.titlesByPath()[first].WorkID, lib.titlesByPath()[second].WorkID
	lib.deleteAndStartAnother("series", "nfo")
	lib.scan(nfoRecord("Puddlejump/tvshow.nfo", show), videoRecord(first))
	if got := lib.onlySeries().ID; got != puddlejump {
		t.Errorf("Puddlejump in Kids is series %d, want %d brought back", got, puddlejump)
	}
	if got := lib.titlesByPath()[first].WorkID; got != magicXylophone || !lib.isLive(magicXylophone) {
		t.Errorf("S01E01 in Kids is title %d (live %v), want %d brought back", got, lib.isLive(got), magicXylophone)
	}
	if !lib.isLive(hospital) || lib.isListed(hospital) {
		t.Errorf("S01E02, whose file did not reappear, is live %v and listed %v; want it kept and listed by no "+
			"library", lib.isLive(hospital), lib.isListed(hospital))
	}
}

// S24, D231 §7, D208 §2: a deleted library's series whose episodes reappear in another library and
// is matched there to the same show comes back with its history: an episode it had takes its file
// back, an episode it did not have joins it, and one whose file reappears later is listed then, kept
// until it does (D363 §10).
func TestMatch_ADeletedLibrarysSeriesMatchedAgainElsewhereComesBack(t *testing.T) {
	t.Parallel()
	lib := orbitoramaLibrary(t)
	lib.scanAll(orbitoramaFiles...)
	outcome, err := lib.matcher.Match(t.Context(), lib.onlySeries().ID)
	if err != nil || outcome != identify.OutcomeMatched {
		t.Fatalf("Match(series) = %s, %v", outcome, err)
	}
	orbitoramaSeries := lib.onlySeries().ID
	clone, later := lib.titlesByPath()[orbitoramaFiles[0]].WorkID, lib.titlesByPath()[orbitoramaFiles[1]].WorkID
	lib.deleteAndStartAnother("series", "tmdb")
	const added = "Orbitorama/Season 02/Orbitorama - S02E12.mkv"
	lib.scanAll(orbitoramaFiles[0], added)
	newSeries := lib.onlySeries().ID
	if newSeries == orbitoramaSeries {
		t.Fatal("the episodes were filed under the old series before any match; the test proves nothing")
	}
	if outcome, err = lib.matcher.Match(t.Context(), newSeries); err != nil || outcome != identify.OutcomeMatched {
		t.Fatalf("Match(series) = %s, %v", outcome, err)
	}
	if got := lib.onlySeries().ID; got != orbitoramaSeries {
		t.Errorf("Orbitorama in Kids is series %d, want %d brought back by the match", got, orbitoramaSeries)
	}
	if got := lib.titlesByPath()[orbitoramaFiles[0]].WorkID; got != clone || !lib.isLive(clone) {
		t.Errorf("S02E10 in Kids is title %d (live %v), want %d brought back", got, lib.isLive(got), clone)
	}
	if got := lib.seriesOfFile(added); got != orbitoramaSeries {
		t.Errorf("S02E12, new in Kids, is in series %d, want %d", got, orbitoramaSeries)
	}
	if !lib.isLive(later) || lib.isListed(later) {
		t.Errorf("S02E11, whose file has not reappeared, is live %v and listed %v; want it kept and listed by "+
			"no library", lib.isLive(later), lib.isListed(later))
	}
	lib.scanAll(orbitoramaFiles[0], orbitoramaFiles[1], added)
	if got := lib.titlesByPath()[orbitoramaFiles[1]].WorkID; got != later || !lib.isLive(later) {
		t.Errorf("S02E11, reappearing later, is title %d (live %v), want %d brought back", got, lib.isLive(got), later)
	}
}
