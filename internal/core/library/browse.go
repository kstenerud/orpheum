// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/textindex"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// Refusals a browse can meet.
var (
	// ErrNoItem is a title or series that does not exist or was removed.
	ErrNoItem = errors.New("no such title or series")
	// ErrBadCursor is a cursor this server did not give.
	ErrBadCursor = errors.New("the cursor is not one this server gave")
)

// The lists a person browses, and the orders they come in.
const (
	ListFilms  = "movie"
	ListSeries = "series"
	SortName   = "name"
	SortAdded  = "added"
)

// BrowseSettings is what the browser reads from the settings: the locale lists sort in (D47), and
// the thresholds that choose Continue Watching (play-state.md).
type BrowseSettings interface {
	String(ctx context.Context, key settings.Key) (string, error)
	Int(ctx context.Context, key settings.Key) (int64, error)
}

// Browser is what a person browses and searches: films and series in a locale's order or newest
// first, a title's detail, a series' episodes (cut-a §5, Browse; D47, D48).
type Browser struct {
	database *store.Database
	settings BrowseSettings
	// connections says whether each node is connected, for whether a title's files can be read now
	// (D241 §3); nil judges none unreadable.
	connections NodeConnections
}

// SetNodeConnections names what says whether a node is connected: nodes.Service. Set it before the
// API serves.
func (browser *Browser) SetNodeConnections(connections NodeConnections) {
	browser.connections = connections
}

// NewBrowser returns the browser.
func NewBrowser(database *store.Database, config BrowseSettings) *Browser {
	return &Browser{database: database, settings: config}
}

// Entry is one film or series in a list.
type Entry struct {
	ID                  int64
	Name                string
	Year                int64
	IdentificationState string
	// IsGuessed is whether its match is a guess no person confirmed: its poster says so (D301 §4).
	IsGuessed    bool
	LibraryID    int64
	AddedAt      int64
	PosterSha256 string // its poster's hash; "" when it has none
	// Unreadable is why none of its files can be read now, the first's, or nil when one can, or it
	// has none (D241 §3).
	Unreadable *Unreadable
	// WatchedPercent is how far its person is through a film, as Continue Watching would resume it:
	// 0 when it would not, or its file's length is not known.
	WatchedPercent int64
	// EpisodeCount is how many episodes a series has with a live file, in the ordering its person
	// watches it in, and FinishedEpisodeCount how many of them they finished; 0 for a film.
	EpisodeCount, FinishedEpisodeCount int64
	// found is where it stands newest first, which a page's cursor holds: set by listByAdded alone.
	found foundKey
}

// foundKey is when the scan that found a title or a series' newest episode listed it, and that
// file's own modified time (step 0058).
type foundKey struct{ at, fileModifiedAt int64 }

// Page is one page of a list, and the cursor of the next; "" on the last.
type Page struct {
	Entries    []Entry
	NextCursor string
}

// ListQuery asks for one page of a list, of the libraries Reach reaches (D243 §4).
type ListQuery struct {
	Kind   string // ListFilms or ListSeries
	Sort   string // SortName or SortAdded
	Cursor string
	// From is a name a list by name starts at, its first title sorted at or after it in the
	// sort_locale, in place of its first page: the alphabet rail's letter. Only with no Cursor.
	From  string
	Limit int64
	Reach auth.Reach
	// UserID is the person listing, whose progress each entry shows.
	UserID int64
}

// List is one page of films or series, only those with a live file.
func (browser *Browser) List(ctx context.Context, query ListQuery) (Page, error) {
	var page Page
	var err error
	if query.Sort == SortAdded {
		page, err = browser.listByAdded(ctx, query)
	} else {
		page, err = browser.listByName(ctx, query)
	}
	if err != nil {
		return Page{}, err
	}
	return browser.withProgress(ctx, query.UserID, query.Kind, page)
}

// listByName is one page in the sort_locale's order (D47): after its cursor, or from its From.
func (browser *Browser) listByName(ctx context.Context, query ListQuery) (Page, error) {
	sorter, err := browser.sorter(ctx)
	if err != nil {
		return Page{}, err
	}
	locale := sorter.Locale()
	afterKey, afterID, err := decodeCursor(query.Cursor)
	if err != nil {
		return Page{}, err
	}
	if query.From != "" && query.Cursor == "" {
		// Every title whose key is the name's or after it: no item's id is below zero.
		afterKey, afterID = sorter.Key(query.From), -1
	}
	var entries []Entry
	var keys [][]byte
	queries := browser.database.Read()
	isEveryLibrary, libraryIDsJSON := query.Reach.QueryArgs()
	if query.Kind == ListSeries {
		rows, listErr := queries.ListSeriesByName(ctx, sqlcgen.ListSeriesByNameParams{
			Locale: locale, AfterKey: afterKey, AfterID: afterID, MaxRows: query.Limit,
			IsEveryLibrary: isEveryLibrary, LibraryIdsJSON: libraryIDsJSON,
		})
		err = listErr
		for _, row := range rows {
			entries = append(entries, Entry{
				ID: row.ItemID, Name: ptr.ValueOrZero(row.Name), Year: ptr.ValueOrZero(row.Year),
				IdentificationState: ptr.ValueOrZero(row.IdentificationState),
				LibraryID:           ptr.ValueOrZero(row.LibraryID), AddedAt: row.AddedAt, IsGuessed: row.IsGuessed,
			})
			keys = append(keys, row.CollationKey)
		}
	} else {
		rows, listErr := queries.ListFilmsByName(ctx, sqlcgen.ListFilmsByNameParams{
			Locale: locale, AfterKey: afterKey, AfterID: afterID, MaxRows: query.Limit,
			IsEveryLibrary: isEveryLibrary, LibraryIdsJSON: libraryIDsJSON,
		})
		err = listErr
		for _, row := range rows {
			entries = append(entries, Entry{
				ID: row.ItemID, Name: ptr.ValueOrZero(row.Name), Year: ptr.ValueOrZero(row.Year),
				IdentificationState: row.IdentificationState, LibraryID: ptr.ValueOrZero(row.LibraryID),
				AddedAt: row.AddedAt, IsGuessed: row.IsGuessed,
			})
			keys = append(keys, row.CollationKey)
		}
	}
	if err != nil {
		return Page{}, fmt.Errorf("list %s by name: %w", query.Kind, err)
	}
	return browser.finishPage(ctx, query.Kind,
		closePage(entries, query.Limit, func(last int) string { return encodeCursor(keys[last], entries[last].ID) }))
}

// listByAdded is one page newest first: by the scan that found it, most recent first, then by its
// file's own time, newest first, then by id (the owner, 2026-10-25; step 0058's found_at). A film is
// as new as the scan that first found it; a series as its newest episode.
func (browser *Browser) listByAdded(ctx context.Context, query ListQuery) (Page, error) {
	cursorKey, id, err := decodeCursor(query.Cursor)
	if err != nil {
		return Page{}, err
	}
	beforeFoundAt, beforeModifiedAt, beforeID := int64(1)<<62, int64(1)<<62, int64(1)<<62 // before everything
	if query.Cursor != "" {
		if len(cursorKey) != 16 {
			return Page{}, ErrBadCursor
		}
		beforeFoundAt = int64(binary.BigEndian.Uint64(cursorKey[:8]))    //nolint:gosec // a time we encoded
		beforeModifiedAt = int64(binary.BigEndian.Uint64(cursorKey[8:])) //nolint:gosec // a time we encoded
		beforeID = id
	}
	var entries []Entry
	queries := browser.database.Read()
	isEveryLibrary, libraryIDsJSON := query.Reach.QueryArgs()
	if query.Kind == ListSeries {
		rows, listErr := queries.ListSeriesByAdded(ctx, sqlcgen.ListSeriesByAddedParams{
			BeforeFoundAt: beforeFoundAt, BeforeModifiedAt: beforeModifiedAt, BeforeID: beforeID, MaxRows: query.Limit,
			IsEveryLibrary: isEveryLibrary, LibraryIdsJSON: libraryIDsJSON,
		})
		err = listErr
		for _, row := range rows {
			entries = append(entries, Entry{
				ID: row.ItemID, Name: ptr.ValueOrZero(row.Name), Year: ptr.ValueOrZero(row.Year),
				IdentificationState: ptr.ValueOrZero(row.IdentificationState),
				LibraryID:           ptr.ValueOrZero(row.LibraryID), AddedAt: row.AddedAt, IsGuessed: row.IsGuessed,
				found: foundKey{at: row.FoundAt, fileModifiedAt: row.FoundFileModifiedAt},
			})
		}
	} else {
		rows, listErr := queries.ListFilmsByAdded(ctx, sqlcgen.ListFilmsByAddedParams{
			BeforeFoundAt: beforeFoundAt, BeforeModifiedAt: beforeModifiedAt, BeforeID: beforeID, MaxRows: query.Limit,
			IsEveryLibrary: isEveryLibrary, LibraryIdsJSON: libraryIDsJSON,
		})
		err = listErr
		for _, row := range rows {
			entries = append(entries, Entry{
				ID: row.ItemID, Name: ptr.ValueOrZero(row.Name), Year: ptr.ValueOrZero(row.Year),
				IdentificationState: row.IdentificationState, LibraryID: ptr.ValueOrZero(row.LibraryID),
				AddedAt: row.AddedAt, IsGuessed: row.IsGuessed,
				found: foundKey{at: row.FoundAt, fileModifiedAt: row.FoundFileModifiedAt},
			})
		}
	}
	if err != nil {
		return Page{}, fmt.Errorf("list %s by date added: %w", query.Kind, err)
	}
	return browser.finishPage(ctx, query.Kind, closePage(entries, query.Limit, func(last int) string {
		found := entries[last].found
		key := binary.BigEndian.AppendUint64(nil, uint64(found.at))            //nolint:gosec // a time
		key = binary.BigEndian.AppendUint64(key, uint64(found.fileModifiedAt)) //nolint:gosec // a time
		return encodeCursor(key, entries[last].ID)
	}))
}

// closePage closes a list: a full page has a next cursor, made from its last entry.
func closePage(entries []Entry, limit int64, nextCursorAt func(last int) string) Page {
	closed := Page{Entries: entries}
	if closed.Entries == nil {
		closed.Entries = []Entry{}
	}
	if int64(len(entries)) == limit && len(entries) > 0 {
		closed.NextCursor = nextCursorAt(len(entries) - 1)
	}
	return closed
}

// A cursor is the last entry's sort key and id, base64url: opaque to clients.
func encodeCursor(key []byte, id int64) string {
	raw := binary.BigEndian.AppendUint64(append([]byte(nil), key...), uint64(id)) //nolint:gosec // an id
	return base64.RawURLEncoding.EncodeToString(raw)
}

func decodeCursor(cursor string) ([]byte, int64, error) {
	if cursor == "" {
		return []byte{}, 0, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil || len(raw) < 8 {
		return nil, 0, ErrBadCursor
	}
	split := len(raw) - 8
	return raw[:split], int64(binary.BigEndian.Uint64(raw[split:])), nil //nolint:gosec // an id we encoded
}

// sorter is the sorter of the sort_locale lists sort in (D47).
func (browser *Browser) sorter(ctx context.Context) (*textindex.Sorter, error) {
	locale, err := browser.settings.String(ctx, settings.SortLocale)
	if err != nil {
		return nil, fmt.Errorf("read sort_locale: %w", err)
	}
	return textindex.NewSorter(locale)
}

// Search finds titles and series by name, through the index that can answer the query (D48), of
// the libraries reach reaches (D243 §4).
func (browser *Browser) Search(
	ctx context.Context, reach auth.Reach, query string, limit int64,
) ([]store.SearchHit, error) {
	plan := textindex.PlanQuery(query)
	if plan.IndexQuery == "" {
		return []store.SearchHit{}, nil
	}
	isEveryLibrary, libraryIDsJSON := reach.QueryArgs()
	hits, err := browser.database.Search(ctx, store.SearchQuery{
		Route: store.SearchRoute(plan.Route), IndexQuery: plan.IndexQuery, Limit: limit,
		IsEveryLibrary: isEveryLibrary, LibraryIDsJSON: libraryIDsJSON,
	})
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}
	return hits, nil
}
