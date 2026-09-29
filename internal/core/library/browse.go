// SPDX-License-Identifier: GPL-3.0-or-later

package library

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"

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
	LibraryID           int64
	AddedAt             int64
	PosterHash          string // its poster's hash; "" when it has none
}

// Page is one page of a list, and the cursor of the next; "" on the last.
type Page struct {
	Entries    []Entry
	NextCursor string
}

// ListQuery asks for one page of a list.
type ListQuery struct {
	Kind   string // ListFilms or ListSeries
	Sort   string // SortName or SortAdded
	Cursor string
	Limit  int64
}

// List is one page of films or series, only those with a live file.
func (browser *Browser) List(ctx context.Context, query ListQuery) (Page, error) {
	if query.Sort == SortAdded {
		return browser.listByAdded(ctx, query)
	}
	locale, err := browser.sortLocale(ctx)
	if err != nil {
		return Page{}, err
	}
	afterKey, afterID, err := decodeCursor(query.Cursor)
	if err != nil {
		return Page{}, err
	}
	var entries []Entry
	var keys [][]byte
	queries := browser.database.Read()
	if query.Kind == ListSeries {
		rows, listErr := queries.ListSeriesByName(ctx, sqlcgen.ListSeriesByNameParams{
			Locale: locale, AfterKey: afterKey, AfterID: afterID, MaxRows: query.Limit,
		})
		err = listErr
		for _, row := range rows {
			entries = append(entries, Entry{
				ID: row.ItemID, Name: ptr.ValueOrZero(row.Name), Year: ptr.ValueOrZero(row.Year),
				IdentificationState: ptr.ValueOrZero(row.IdentificationState),
				LibraryID:           ptr.ValueOrZero(row.LibraryID), AddedAt: row.AddedAt,
			})
			keys = append(keys, row.CollationKey)
		}
	} else {
		rows, listErr := queries.ListFilmsByName(ctx, sqlcgen.ListFilmsByNameParams{
			Locale: locale, AfterKey: afterKey, AfterID: afterID, MaxRows: query.Limit,
		})
		err = listErr
		for _, row := range rows {
			entries = append(entries, Entry{
				ID: row.ItemID, Name: ptr.ValueOrZero(row.Name), Year: ptr.ValueOrZero(row.Year),
				IdentificationState: row.IdentificationState, LibraryID: ptr.ValueOrZero(row.LibraryID),
				AddedAt: row.AddedAt,
			})
			keys = append(keys, row.CollationKey)
		}
	}
	if err != nil {
		return Page{}, fmt.Errorf("list %s by name: %w", query.Kind, err)
	}
	return browser.withPosters(ctx,
		closePage(entries, query.Limit, func(last int) string { return encodeCursor(keys[last], entries[last].ID) }))
}

func (browser *Browser) listByAdded(ctx context.Context, query ListQuery) (Page, error) {
	addedAtKey, id, err := decodeCursor(query.Cursor)
	if err != nil {
		return Page{}, err
	}
	beforeAddedAt, beforeID := int64(1)<<62, int64(1)<<62 // the first page starts before everything
	if query.Cursor != "" {
		beforeAddedAt, beforeID = int64(binary.BigEndian.Uint64(addedAtKey)), id //nolint:gosec // a time we encoded
	}
	var entries []Entry
	queries := browser.database.Read()
	if query.Kind == ListSeries {
		rows, listErr := queries.ListSeriesByAdded(ctx, sqlcgen.ListSeriesByAddedParams{
			BeforeAt: beforeAddedAt, BeforeID: beforeID, MaxRows: query.Limit,
		})
		err = listErr
		for _, row := range rows {
			entries = append(entries, Entry{
				ID: row.ItemID, Name: ptr.ValueOrZero(row.Name), Year: ptr.ValueOrZero(row.Year),
				IdentificationState: ptr.ValueOrZero(row.IdentificationState),
				LibraryID:           ptr.ValueOrZero(row.LibraryID), AddedAt: row.AddedAt,
			})
		}
	} else {
		rows, listErr := queries.ListFilmsByAdded(ctx, sqlcgen.ListFilmsByAddedParams{
			BeforeAt: beforeAddedAt, BeforeID: beforeID, MaxRows: query.Limit,
		})
		err = listErr
		for _, row := range rows {
			entries = append(entries, Entry{
				ID: row.ItemID, Name: ptr.ValueOrZero(row.Name), Year: ptr.ValueOrZero(row.Year),
				IdentificationState: row.IdentificationState, LibraryID: ptr.ValueOrZero(row.LibraryID),
				AddedAt: row.AddedAt,
			})
		}
	}
	if err != nil {
		return Page{}, fmt.Errorf("list %s by date added: %w", query.Kind, err)
	}
	return browser.withPosters(ctx, closePage(entries, query.Limit, func(last int) string {
		added := binary.BigEndian.AppendUint64(nil, uint64(entries[last].AddedAt)) //nolint:gosec // a time
		return encodeCursor(added, entries[last].ID)
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

func (browser *Browser) sortLocale(ctx context.Context) (string, error) {
	locale, err := browser.settings.String(ctx, settings.SortLocale)
	if err != nil {
		return "", fmt.Errorf("read sort_locale: %w", err)
	}
	sorter, err := textindex.NewSorter(locale)
	if err != nil {
		return "", err
	}
	return sorter.Locale(), nil
}

// Search finds titles and series by name, through the index that can answer the query (D48).
func (browser *Browser) Search(ctx context.Context, query string, limit int64) ([]store.SearchHit, error) {
	plan := textindex.PlanQuery(query)
	if plan.IndexQuery == "" {
		return []store.SearchHit{}, nil
	}
	hits, err := browser.database.Search(ctx, store.SearchRoute(plan.Route), plan.IndexQuery, limit)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}
	return hits, nil
}
