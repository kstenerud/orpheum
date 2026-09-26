// SPDX-License-Identifier: GPL-3.0-or-later

package library

import (
	"slices"
	"testing"
	"time"

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

// name writes an item's text and its sort key in every locale the tests use, as the pass does.
func name(t *testing.T, queries *sqlcgen.Queries, itemID int64, listing, text string) {
	t.Helper()
	err := queries.UpsertOriginalText(t.Context(), sqlcgen.UpsertOriginalTextParams{
		ItemID: itemID, Lang: "und", Name: text, SearchAlternates: new(textindex.Fold(text)),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, locale := range []string{"en", "sv"} {
		sorter, err := textindex.NewSorter(locale)
		if err != nil {
			t.Fatal(err)
		}
		err = queries.InsertSortKey(t.Context(), sqlcgen.InsertSortKeyParams{
			ItemID: itemID, Locale: locale, Listing: listing, Key: sorter.Key(text),
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

// film files the scanned file at path under a new film called text.
func (world *shelf) film(text, path string) int64 {
	world.t.Helper()
	fileID := world.live()[root+"/"+path].ID
	world.clock.Advance(time.Minute)
	var titleID int64
	err := world.database.Write(world.t.Context(), func(queries *sqlcgen.Queries) error {
		var err error
		titleID, err = queries.InsertItem(world.t.Context(), sqlcgen.InsertItemParams{
			Kind: "title", LibraryID: new(int64(1)), CreatedAt: world.clock.Now().UnixMilli(),
		})
		if err != nil {
			return err
		}
		film := sqlcgen.InsertTitleParams{ID: titleID, Kind: "movie", Identification: "identified"}
		err = queries.InsertTitle(world.t.Context(), film)
		if err != nil {
			return err
		}
		name(world.t, queries, titleID, ListFilms, text)
		if fileID == 0 {
			return nil // a film whose files are gone
		}
		version, err := queries.InsertVersion(world.t.Context(),
			sqlcgen.InsertVersionParams{TitleID: titleID, CreatedAt: 1})
		if err != nil {
			return err
		}
		return queries.SetFileVersion(world.t.Context(), sqlcgen.SetFileVersionParams{VersionID: &version, ID: fileID})
	})
	if err != nil {
		world.t.Fatal(err)
	}
	return titleID
}

func names(page Page) []string {
	var list []string
	for _, entry := range page.Entries {
		list = append(list, entry.Name)
	}
	return list
}

func TestBrowse_FilmsInTheLocalesOrderAPageAtATime(t *testing.T) {
	t.Parallel()
	world := newShelf(t)
	world.scan([]nodeproto.Record{observed("z.mkv", "z"), observed("o.mkv", "o"), observed("a.mkv", "a")}, true)
	world.film("Destiny", "z.mkv")
	world.film("Ödet", "o.mkv")
	world.film("Fièvre", "a.mkv")
	world.film("Gone Film", "gone.mkv")
	first, err := world.browser.List(t.Context(), ListQuery{Kind: ListFilms, Sort: SortName, Limit: 2})
	if err != nil || !slices.Equal(names(first), []string{"Fièvre", "Ödet"}) || first.NextCursor == "" {
		t.Fatalf("first page = %+v, %v", first, err)
	}
	next := ListQuery{Kind: ListFilms, Sort: SortName, Cursor: first.NextCursor, Limit: 2}
	rest, err := world.browser.List(t.Context(), next)
	if err != nil || !slices.Equal(names(rest), []string{"Destiny"}) || rest.NextCursor != "" {
		t.Errorf("second page = %+v, %v; the film with no live file is not listed", rest, err)
	}
	settingstest.Set(t, world.config, settings.SortLocale, `"sv"`)
	swedish, err := world.browser.List(t.Context(), ListQuery{Kind: ListFilms, Sort: SortName, Limit: 10})
	if err != nil || !slices.Equal(names(swedish), []string{"Fièvre", "Destiny", "Ödet"}) {
		t.Errorf("in Swedish = %v, %v; Ö comes after Z", names(swedish), err)
	}
	if _, err := world.browser.List(t.Context(), ListQuery{Kind: ListFilms, Cursor: "!", Limit: 1}); err == nil {
		t.Error("a cursor this server did not give was accepted")
	}
}

func TestBrowse_FilmsNewestFirst(t *testing.T) {
	t.Parallel()
	world := newShelf(t)
	world.scan([]nodeproto.Record{observed("a.mkv", "a"), observed("b.mkv", "b"), observed("c.mkv", "c")}, true)
	world.film("First", "a.mkv")
	world.film("Second", "b.mkv")
	world.film("Third", "c.mkv")
	first, err := world.browser.List(t.Context(), ListQuery{Kind: ListFilms, Sort: SortAdded, Limit: 2})
	if err != nil || !slices.Equal(names(first), []string{"Third", "Second"}) {
		t.Fatalf("first page = %v, %v", names(first), err)
	}
	next := ListQuery{Kind: ListFilms, Sort: SortAdded, Cursor: first.NextCursor, Limit: 2}
	rest, err := world.browser.List(t.Context(), next)
	if err != nil || !slices.Equal(names(rest), []string{"First"}) {
		t.Errorf("second page = %v, %v", names(rest), err)
	}
}

func TestBrowse_ATitlesDetailHasItsVersionsFilesAndStreams(t *testing.T) {
	t.Parallel()
	world := newShelf(t)
	world.scan([]nodeproto.Record{observed("metropolis.mkv", "heat")}, true)
	heat := world.film("Metropolis", "metropolis.mkv")
	detail, err := world.browser.Title(t.Context(), heat)
	if err != nil || detail.Name == nil || *detail.Name != "Metropolis" || len(detail.Versions) != 1 {
		t.Fatalf("Title = %+v, %v", detail, err)
	}
	file := detail.Versions[0].Files[0]
	if file.Path != root+"/metropolis.mkv" || len(file.Streams) != 2 || detail.Episode != nil {
		t.Errorf("file %+v; want the video and audio streams, and no episode place", file)
	}
	if _, err := world.browser.Title(t.Context(), 999); err == nil {
		t.Error("the detail of no title was found")
	}
}

func TestSearch_EveryRouteFindsWhatItShould(t *testing.T) {
	t.Parallel()
	world := newShelf(t)
	world.scan([]nodeproto.Record{
		observed("b.mkv", "b"), observed("s.mkv", "s"), observed("u.mkv", "u"), observed("h.mkv", "h"),
	}, true)
	world.film("Sølvi: Lumenfold Live", "b.mkv")
	world.film("十字路", "s.mkv")
	world.film("Up", "u.mkv")
	world.film("Metropolis", "h.mkv")
	for query, want := range map[string]string{
		"solvi":      "Sølvi: Lumenfold Live", // the word index, through the alternates
		"biophi":     "Sølvi: Lumenfold Live", // a prefix
		"solvi live": "Sølvi: Lumenfold Live", // two words
		"十字":       "十字路",              // the trigram index
		"千尋":         "十字路",              // too short for it: the folded substring
		"up":         "Up",
	} {
		hits, err := world.browser.Search(t.Context(), query, 10)
		if err != nil || len(hits) == 0 || hits[0].Name != want {
			t.Errorf("Search(%q) = %+v, %v; want %q first", query, hits, err, want)
		}
	}
	if hits, err := world.browser.Search(t.Context(), `heat" OR "x`, 10); err != nil {
		t.Errorf("a query with FTS5 syntax in it = %+v, %v; want it read as words", hits, err)
	}
}
