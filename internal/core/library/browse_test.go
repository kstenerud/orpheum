// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"slices"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/core/textindex"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// shelf is a harness whose files are filed under titles, as the identification pass would.
type shelf struct {
	*harness
	browser *Browser
	config  *settings.Service
}

func newShelf(t *testing.T) *shelf {
	t.Helper()
	world := newHarness(t)
	config := settingstest.Seeded(t, world.database)
	return &shelf{harness: world, browser: NewBrowser(world.database, config), config: config}
}

// nameItem writes an item's text and its sort key in every locale the tests use, as the pass does.
func nameItem(t *testing.T, queries *sqlcgen.Queries, itemID int64, listing, text string) {
	t.Helper()
	err := queries.UpsertOriginalText(t.Context(), sqlcgen.UpsertOriginalTextParams{
		ItemID: itemID, Language: "und", Name: text, SearchAlternates: new(textindex.Fold(text)),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, locale := range []string{"en", "sv"} {
		sorter, err := textindex.NewSorter(locale)
		if err != nil {
			t.Fatal(err)
		}
		err = queries.UpsertSortKey(t.Context(), sqlcgen.UpsertSortKeyParams{
			ItemID: itemID, Locale: locale, Listing: listing, CollationKey: sorter.Key(text),
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

// fileAsFilm files the scanned file at path under a new film called text.
func (world *shelf) fileAsFilm(text, path string) int64 {
	world.t.Helper()
	fileID := world.liveFilesByPath()[root+"/"+path].ID
	world.clock.Advance(time.Minute)
	var titleID int64
	err := world.database.Write(world.t.Context(), func(queries *sqlcgen.Queries) error {
		var err error
		titleID, err = queries.InsertItem(world.t.Context(), sqlcgen.InsertItemParams{
			Kind: "title", LibraryID: new(int64(1)), AddedAt: world.clock.Now().UnixMilli(),
		})
		if err != nil {
			return err
		}
		film := sqlcgen.InsertTitleParams{ID: titleID, Kind: "movie", IdentificationState: "identified"}
		err = queries.InsertTitle(world.t.Context(), film)
		if err != nil {
			return err
		}
		nameItem(world.t, queries, titleID, ListFilms, text)
		version, err := queries.InsertVersion(world.t.Context(),
			sqlcgen.InsertVersionParams{TitleID: titleID, CreatedAt: 1})
		if err != nil || fileID == 0 {
			return err // with no file, a film whose files are gone: its version is all that is left
		}
		return PutInVersion(world.t.Context(), queries, fileID, LibraryKindMovie, version)
	})
	if err != nil {
		world.t.Fatal(err)
	}
	return titleID
}

func entryNames(page Page) []string {
	var list []string
	for _, entry := range page.Entries {
		list = append(list, entry.Name)
	}
	return list
}

func TestBrowse_FilmsInTheLocalesOrderAPageAtATime(t *testing.T) {
	t.Parallel()
	world := newShelf(t)
	world.scan([]nodeproto.ScanRecord{
		observedRecord("z.mkv", "z"), observedRecord("o.mkv", "o"), observedRecord("a.mkv", "a"),
	}, true)
	world.fileAsFilm("Destiny", "z.mkv")
	world.fileAsFilm("Ödet", "o.mkv")
	world.fileAsFilm("Fièvre", "a.mkv")
	world.fileAsFilm("Gone Film", "gone.mkv")
	first, err := world.browser.List(t.Context(), ListQuery{
		Reach: auth.EveryLibrary(), Kind: ListFilms, Sort: SortName, Limit: 2,
	})
	if err != nil || !slices.Equal(entryNames(first), []string{"Fièvre", "Ödet"}) || first.NextCursor == "" {
		t.Fatalf("first page = %+v, %v", first, err)
	}
	next := ListQuery{Reach: auth.EveryLibrary(), Kind: ListFilms, Sort: SortName, Cursor: first.NextCursor, Limit: 2}
	rest, err := world.browser.List(t.Context(), next)
	if err != nil || !slices.Equal(entryNames(rest), []string{"Destiny"}) || rest.NextCursor != "" {
		t.Errorf("second page = %+v, %v; the film with no live file is not listed", rest, err)
	}
	settingstest.Set(t, world.config, settings.SortLocale, `"sv"`)
	swedish, err := world.browser.List(t.Context(), ListQuery{
		Reach: auth.EveryLibrary(), Kind: ListFilms, Sort: SortName, Limit: 10,
	})
	if err != nil || !slices.Equal(entryNames(swedish), []string{"Fièvre", "Destiny", "Ödet"}) {
		t.Errorf("in Swedish = %v, %v; Ö comes after Z", entryNames(swedish), err)
	}
	if _, err := world.browser.List(t.Context(), ListQuery{
		Reach: auth.EveryLibrary(), Kind: ListFilms, Cursor: "!", Limit: 1,
	}); err == nil {
		t.Error("a cursor this server did not give was accepted")
	}
}

// A list by name from a name starts at the first title sorted at or after it, in the sort_locale,
// a letter of the alphabet rail's at the first title it begins (the owner, 2026-10-25); its next
// cursor goes on from there, and a name after every title is an empty page.
func TestBrowse_FilmsFromANameStartAtTheFirstSortedAtOrAfterIt(t *testing.T) {
	t.Parallel()
	world := newShelf(t)
	world.scan([]nodeproto.ScanRecord{
		observedRecord("1.mkv", "1"), observedRecord("a.mkv", "a"), observedRecord("m.mkv", "m"),
		observedRecord("m2.mkv", "m2"), observedRecord("o.mkv", "o"), observedRecord("z.mkv", "z"),
	}, true)
	world.fileAsFilm("12 Patient Sailors", "1.mkv")
	world.fileAsFilm("Fièvre", "a.mkv")
	world.fileAsFilm("M", "m.mkv")
	world.fileAsFilm("memento", "m2.mkv")
	world.fileAsFilm("Ödet", "o.mkv")
	world.fileAsFilm("Destiny", "z.mkv")
	from := func(name string, limit int64) Page {
		t.Helper()
		page, err := world.browser.List(t.Context(), ListQuery{
			Reach: auth.EveryLibrary(), Kind: ListFilms, Sort: SortName, From: name, Limit: limit,
		})
		if err != nil {
			t.Fatal(err)
		}
		return page
	}
	// A title named the letter itself is at it; one in lower case beside it; accents fold in English.
	if page := from("M", 2); !slices.Equal(entryNames(page), []string{"M", "memento"}) || page.NextCursor == "" {
		t.Errorf("from M = %v", entryNames(page))
	} else {
		rest, err := world.browser.List(t.Context(), ListQuery{
			Reach: auth.EveryLibrary(), Kind: ListFilms, Sort: SortName, Cursor: page.NextCursor, Limit: 10,
		})
		if err != nil || !slices.Equal(entryNames(rest), []string{"Ödet", "Destiny"}) {
			t.Errorf("after from M = %v, %v", entryNames(rest), err)
		}
	}
	if page := from("N", 1); !slices.Equal(entryNames(page), []string{"Ödet"}) {
		t.Errorf("from N, which no title begins = %v; want the next title, Ödet", entryNames(page))
	}
	if page := from("A", 1); !slices.Equal(entryNames(page), []string{"Fièvre"}) {
		t.Errorf("from A = %v; want Fièvre, after the title that begins with a digit", entryNames(page))
	}
	if page := from("Zz", 5); len(page.Entries) != 0 || page.NextCursor != "" {
		t.Errorf("from past the last = %v", entryNames(page))
	}
	settingstest.Set(t, world.config, settings.SortLocale, `"sv"`)
	if page := from("P", 5); !slices.Equal(entryNames(page), []string{"Destiny", "Ödet"}) {
		t.Errorf("from P in Swedish = %v; want Destiny, then Ödet after Z", entryNames(page))
	}
}

func TestBrowse_FilmsNewestFirst(t *testing.T) {
	t.Parallel()
	world := newShelf(t)
	world.scan([]nodeproto.ScanRecord{
		observedRecord("a.mkv", "a"), observedRecord("b.mkv", "b"), observedRecord("c.mkv", "c"),
	}, true)
	world.fileAsFilm("First", "a.mkv")
	world.fileAsFilm("Second", "b.mkv")
	world.fileAsFilm("Third", "c.mkv")
	first, err := world.browser.List(t.Context(), ListQuery{
		Reach: auth.EveryLibrary(), Kind: ListFilms, Sort: SortAdded, Limit: 2,
	})
	if err != nil || !slices.Equal(entryNames(first), []string{"Third", "Second"}) {
		t.Fatalf("first page = %v, %v", entryNames(first), err)
	}
	next := ListQuery{Reach: auth.EveryLibrary(), Kind: ListFilms, Sort: SortAdded, Cursor: first.NextCursor, Limit: 2}
	rest, err := world.browser.List(t.Context(), next)
	if err != nil || !slices.Equal(entryNames(rest), []string{"First"}) {
		t.Errorf("second page = %v, %v", entryNames(rest), err)
	}
}

func TestBrowse_ATitlesDetailHasItsVersionsFilesAndStreams(t *testing.T) {
	t.Parallel()
	world := newShelf(t)
	world.scan([]nodeproto.ScanRecord{observedRecord("metropolis.mkv", "heat")}, true)
	heat := world.fileAsFilm("Metropolis", "metropolis.mkv")
	detail, err := world.browser.Title(t.Context(), auth.EveryLibrary(), 0, heat)
	if err != nil || detail.Name == nil || *detail.Name != "Metropolis" || len(detail.Versions) != 1 {
		t.Fatalf("Title = %+v, %v", detail, err)
	}
	file := detail.Versions[0].Files[0]
	if file.Path != root+"/metropolis.mkv" || len(file.Streams) != 2 || detail.Episode != nil {
		t.Errorf("file %+v; want the video and audio streams, and no episode place", file)
	}
	if _, err := world.browser.Title(t.Context(), auth.EveryLibrary(), 0, 999); err == nil {
		t.Error("the detail of no title was found")
	}
}

func TestSearch_EveryRouteFindsWhatItShould(t *testing.T) {
	t.Parallel()
	world := newShelf(t)
	world.scan([]nodeproto.ScanRecord{
		observedRecord("b.mkv", "b"), observedRecord("s.mkv", "s"),
		observedRecord("u.mkv", "u"), observedRecord("h.mkv", "h"),
	}, true)
	world.fileAsFilm("Sølvi: Lumenfold Live", "b.mkv")
	world.fileAsFilm("十字路", "s.mkv")
	world.fileAsFilm("Up", "u.mkv")
	world.fileAsFilm("Metropolis", "h.mkv")
	for query, want := range map[string]string{
		"solvi":      "Sølvi: Lumenfold Live", // the word index, through the alternates
		"biophi":     "Sølvi: Lumenfold Live", // a prefix
		"solvi live": "Sølvi: Lumenfold Live", // two words
		"十字":       "十字路",              // the trigram index
		"千尋":         "十字路",              // too short for it: the folded substring
		"up":         "Up",
	} {
		hits, err := world.browser.Search(t.Context(), auth.EveryLibrary(), query, 10)
		if err != nil || len(hits) == 0 || hits[0].Name != want {
			t.Errorf("Search(%q) = %+v, %v; want %q first", query, hits, err, want)
		}
	}
	if hits, err := world.browser.Search(t.Context(), auth.EveryLibrary(), `heat" OR "x`, 10); err != nil {
		t.Errorf("a query with FTS5 syntax in it = %+v, %v; want it read as words", hits, err)
	}
}

func TestSearch_DoesNotFindAFilmWhoseFilesAreAllGone(t *testing.T) {
	t.Parallel()
	world := newShelf(t)
	world.scan([]nodeproto.ScanRecord{observedRecord("h.mkv", "h"), observedRecord("u.mkv", "u")}, true)
	world.fileAsFilm("Metropolis", "h.mkv")
	world.fileAsFilm("Up", "u.mkv")
	if hits, err := world.browser.Search(t.Context(), auth.EveryLibrary(), "heat", 10); err != nil || len(hits) != 1 {
		t.Fatalf("Search(heat) with its file = %+v, %v; want Metropolis", hits, err)
	}
	// A complete scan without h.mkv removes its file; the film is kept, with its history, unlisted.
	world.scan([]nodeproto.ScanRecord{observedRecord("u.mkv", "u")}, true)
	if hits, err := world.browser.Search(t.Context(), auth.EveryLibrary(), "heat", 10); err != nil || len(hits) != 0 {
		t.Errorf("Search(heat) with its file gone = %+v, %v; want nothing, as browse lists nothing", hits, err)
	}
}

func TestBrowse_ATitlesExtrasAreOnlyThoseWithALiveFile(t *testing.T) {
	t.Parallel()
	world := newShelf(t)
	world.scan([]nodeproto.ScanRecord{
		observedRecord("metropolis.mkv", "heat"), observedRecord("trailer.mkv", "trailer"),
		observedRecord("interview.mkv", "interview"),
	}, true)
	heat := world.fileAsFilm("Metropolis", "metropolis.mkv")
	trailer := world.fileAsFilm("Metropolis trailer", "trailer.mkv")
	interview := world.fileAsFilm("Metropolis interview", "interview.mkv")
	err := world.database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
		for _, extra := range []int64{trailer, interview} {
			relation := sqlcgen.UpsertScannerExtraRelationParams{
				OwnerTitle: heat, ExtraTitle: extra, ExtraType: "trailer",
			}
			if err := queries.UpsertScannerExtraRelation(t.Context(), relation); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// A complete scan without the interview removes its file; its title is kept, unlisted.
	world.scan([]nodeproto.ScanRecord{
		observedRecord("metropolis.mkv", "heat"), observedRecord("trailer.mkv", "trailer"),
	}, true)
	detail, err := world.browser.Title(t.Context(), auth.EveryLibrary(), 0, heat)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Extras) != 1 || detail.Extras[0].ExtraTitle != trailer {
		t.Errorf("extras = %+v; want only the trailer, whose file is live", detail.Extras)
	}
}
