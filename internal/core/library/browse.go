// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"bytes"
	"cmp"
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"slices"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/textindex"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store"
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
	// ListOther is an other library's videos, neither films nor series (D349 §3).
	ListOther = "other"
	SortName  = "name"
	SortAdded = "added"
	// SortRecorded is newest recorded first, and SortFileName by the files' names: other videos'
	// alone (D349 §4).
	SortRecorded = "recorded"
	SortFileName = "fileName"
	// SortReleased is newest released first: a film's release date, a series' first air date, those
	// with none last; films' and series' alone (D366 §1).
	SortReleased = "released"
)

// The ways a list runs (D366 §2): a sort's own default, by name A to Z and by date newest first,
// or either way asked for.
const (
	OrderDefault    = ""
	OrderAscending  = "ascending"
	OrderDescending = "descending"
)

// ErrNoSuchSort is an order the list does not come in: by recorded date or file name, but for
// other videos; by release date, but for films and series.
var ErrNoSuchSort = errors.New("the list does not come in that order")

// BrowseSettings is what the browser reads from the settings: the locale lists sort in (D47), and
// the thresholds that choose Continue Watching (play-state.md).
type BrowseSettings interface {
	String(ctx context.Context, key settings.Key) (string, error)
	Int(ctx context.Context, key settings.Key) (int64, error)
}

// Browser is what a person browses and searches: films, series and other videos in a locale's order
// or newest first, a title's detail, a series' episodes (cut-a §5, Browse; D47, D48, D349).
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

// Entry is one film, series or other video in a list.
type Entry struct {
	ID int64
	// Kind is what it is: ListFilms, ListSeries or ListOther.
	Kind                string
	Name                string
	Year                int64
	IdentificationState string
	// IsGuessed is whether its match is a guess no person confirmed: its poster says so (D301 §4).
	IsGuessed    bool
	AddedAt      int64
	PosterSha256 string // its poster's hash; "" when it has none
	// LandscapeSha256 is an other video's 16:9 picture, its still or else its backdrop, a frame still
	// of its video only when it has neither; "" when it has none (D349 §3, D362 §2).
	LandscapeSha256 string
	// NoPosterStillSha256 is, for a film or series with no poster, the 16:9 picture shown in the top of
	// its poster's place: its backdrop, else a film's frame still or a series' first episode's still;
	// "" when it has a poster or none of them (D362 §2).
	NoPosterStillSha256 string
	// Other is an other video's date and file name; nil for a film or a series.
	Other *OtherFacts
	// Unreadable is why none of its files can be read now, the first's, or nil when one can, or it
	// has none (D241 §3).
	Unreadable *Unreadable
	// WatchedPercent is how far its person is through a film, as Continue Watching would resume it:
	// 0 when it would not, or its file's length is not known.
	WatchedPercent int64
	// EpisodeCount is how many episodes a series has with a live file, in the ordering its person
	// watches it in, and FinishedEpisodeCount how many of them they watched (D370 §5); 0 for a film.
	EpisodeCount, FinishedEpisodeCount int64
	// IsWatched is whether its person watched it: a film or other video by playing or by hand, a series
	// once every episode it counts is (D370 §1, §3).
	IsWatched bool
	// Letter is the alphabet rail's letter it is under, in a list by name or by file name: # or A to
	// Z, by its sort key, its article and its sort title as the list's order reads them (D402); ""
	// in a list in any other order.
	Letter string
}

// OtherFacts are an other video's: when it was recorded, where that came from (tag, name, file or
// person), and its file's name (D349 §4).
type OtherFacts struct {
	RecordedAtMs int64
	RecordedFrom string
	FileName     string
}

// otherFactsOf is an other video's facts as a query reads them, or nil when the pass gave it none.
func otherFactsOf(recordedAt *int64, recordedFrom, fileName *string) *OtherFacts {
	if recordedAt == nil {
		return nil
	}
	return &OtherFacts{
		RecordedAtMs: *recordedAt, RecordedFrom: ptr.ValueOrZero(recordedFrom), FileName: ptr.ValueOrZero(fileName),
	}
}

// Page is one page of a list, and the cursor of the next; "" on the last.
type Page struct {
	Entries    []Entry
	NextCursor string
	// Total is how many entries the whole list holds, counted on every page but one asked from a name
	// (From), so a client sizes the list before its other pages come and sees, page by page, where its
	// end now is (the owner, 2026-11-11); 0 on a page from a name.
	Total int64
}

// ListQuery asks for one page of a list, of the libraries Reach reaches (D243 §4).
type ListQuery struct {
	// Kinds are the kinds it lists, merged in its order: ListFilms, ListSeries or ListOther, each
	// named, never "every kind", so a kind added later is in no list that does not name it (D432 §1).
	Kinds []string
	// Sort is SortName or SortAdded; for ListOther SortRecorded or SortFileName too, for ListFilms
	// and ListSeries SortReleased.
	Sort string
	// Order is the way it runs: OrderAscending, OrderDescending, or OrderDefault for the sort's own
	// (isDescending).
	Order  string
	Cursor string
	// From is a name a list by name or by file name starts at, its first title sorted at or after it
	// in the sortLocale, in place of its first page: the alphabet rail's letter. Only with no Cursor.
	From  string
	Limit int64
	// Reach is the libraries listed: the caller's, narrowed to one library or less those they left
	// out (auth.Reach's Narrowed and Without).
	Reach auth.Reach
	// UserID is the person listing, whose progress each entry shows.
	UserID int64
	// Query, when not "", lists only the titles whose names match it, as search matches them (D48):
	// a film, other video or series by its own name, never by an episode's or an extra's.
	Query string
	// GenreID, PersonID and Decade, when not zero, list only the titles in that genre, crediting
	// that person, or first out in that decade (its first year, 1980), and IsUnwatched only those
	// UserID has not watched, a series while one of its episodes with a file is not: the list's
	// filters, combined with each other and with Query (search-by-more-2026-11-04.md §4).
	GenreID     int64
	PersonID    int64
	Decade      int64
	IsUnwatched bool
	// matched is the items Query matched, which List and Letters find before they read the list.
	matched matchedItems
}

// isDescending is whether the list runs Z to A or newest first: as asked, or by its sort's own
// default, A to Z by name or file name and newest first by date (D366 §2).
func (query ListQuery) isDescending() bool {
	if query.Order == OrderDefault {
		return !isByName(query.Sort)
	}
	return query.Order == OrderDescending
}

// isReversed is whether the list runs the other way from its sort's own: Z to A, or oldest first.
func (query ListQuery) isReversed() bool {
	return query.isDescending() == isByName(query.Sort)
}

// isByName is whether a sort is by a name, the title's or its file's: the alphabet rail's.
func isByName(sort string) bool {
	return sort == SortName || sort == SortFileName
}

// ListKinds are every kind a list can name today: films, series and other videos (D432 §1).
func ListKinds() []string {
	return []string{ListFilms, ListSeries, ListOther}
}

// isSortOfKinds is whether a list of kinds comes in sort's order: every one of them does. A list
// names one kind at least.
func isSortOfKinds(kinds []string, sort string) bool {
	return len(kinds) > 0 && !slices.ContainsFunc(kinds, func(kind string) bool { return !isSortOfKind(kind, sort) })
}

// distinctKinds are kinds each once, in a fixed order, so a kind named twice lists its entries once.
func distinctKinds(kinds []string) []string {
	distinct := slices.Clone(kinds)
	slices.Sort(distinct)
	return slices.Compact(distinct)
}

// isSortOfKind is whether a list of kind comes in sort's order.
func isSortOfKind(kind, sort string) bool {
	switch sort {
	case SortRecorded, SortFileName:
		return kind == ListOther
	case SortReleased:
		return kind == ListFilms || kind == ListSeries
	}
	return true
}

// List is one page of films, series or other videos, only those with a live file.
func (browser *Browser) List(ctx context.Context, query ListQuery) (Page, error) {
	if !isSortOfKinds(query.Kinds, query.Sort) {
		return Page{}, fmt.Errorf("%v by %s: %w", query.Kinds, query.Sort, ErrNoSuchSort)
	}
	query, isNoneMatched, err := browser.withMatched(ctx, query)
	if err != nil || isNoneMatched {
		return Page{Entries: []Entry{}}, err
	}
	keyed, err := browser.listKeyed(ctx, query)
	if err == nil && isByName(query.Sort) {
		err = browser.withLetters(ctx, keyed)
	}
	if err != nil {
		return Page{}, err
	}
	entries := make([]Entry, 0, len(keyed))
	for _, each := range keyed {
		entries = append(entries, each.entry)
	}
	page := closePage(entries, query.Limit, func(last int) string {
		return EncodeCursor(keyed[last].key, entries[last].ID)
	})
	page, err = browser.finishPage(ctx, query.Reach, page)
	if err != nil {
		return Page{}, err
	}
	if query.From == "" {
		if page.Total, err = browser.countListed(ctx, query); err != nil {
			return Page{}, err
		}
	}
	return browser.withProgress(ctx, query.Reach, query.UserID, page)
}

// keyedEntry is an entry with its place in its list's order: the key a cursor holds.
type keyedEntry struct {
	entry Entry
	key   []byte
}

// listPosition is where a page starts: after a key and an id, in its list's order.
type listPosition struct {
	key []byte
	id  int64
}

// listKeyed is one page's entries with their keys: of one kind, or of every kind merged in the
// list's order, each kind read from the same place.
func (browser *Browser) listKeyed(ctx context.Context, query ListQuery) ([]keyedEntry, error) {
	after, err := browser.startOf(ctx, query)
	if err != nil {
		return nil, err
	}
	var keyed []keyedEntry
	for _, kind := range distinctKinds(query.Kinds) {
		ofKind, listErr := browser.listKind(ctx, kind, query, after)
		if listErr != nil {
			return nil, fmt.Errorf("list %s by %s: %w", kind, query.Sort, listErr)
		}
		keyed = append(keyed, ofKind...)
	}
	isDescending := query.isDescending()
	slices.SortFunc(keyed, func(first, second keyedEntry) int {
		if query.Sort == SortReleased {
			if byDating := compareDating(first, second); byDating != 0 {
				return byDating
			}
		}
		order := cmp.Or(bytes.Compare(first.key, second.key), cmp.Compare(first.entry.ID, second.entry.ID))
		if isDescending {
			return -order
		}
		return order
	})
	if int64(len(keyed)) > query.Limit {
		keyed = keyed[:query.Limit]
	}
	return keyed, nil
}

// startOf is where a list's page starts: after its cursor, at its From, or at its start.
func (browser *Browser) startOf(ctx context.Context, query ListQuery) (listPosition, error) {
	key, id, err := DecodeCursor(query.Cursor)
	if err != nil {
		return listPosition{}, err
	}
	isNamed := isByName(query.Sort)
	switch {
	case query.Cursor != "" && !isNamed && len(key) != keyLength(query.Sort):
		return listPosition{}, ErrBadCursor
	case query.Cursor != "":
		return listPosition{key: key, id: id}, nil
	case query.From != "" && isNamed:
		sorter, sorterErr := browser.sorter(ctx)
		if sorterErr != nil {
			return listPosition{}, sorterErr
		}
		// A to Z, every title whose key is the name's or after it: no item's id is below zero. Z to
		// A, every title whose key is before it: the name is where the list's titles end (D366 §4).
		return listPosition{key: sorter.Key(query.From), id: -1}, nil
	case isNamed && query.isDescending():
		return listPosition{key: afterEveryName, id: math.MaxInt64}, nil
	case isNamed:
		return listPosition{key: []byte{}, id: 0}, nil
	}
	return dateListStart(query.Sort, query.isDescending()), nil
}

// afterEveryName is a key after every name's, where a list Z to A starts: no collation key's first
// weight begins with 0xFF.
var afterEveryName = []byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF} //nolint:gochecknoglobals // a constant

// dateListStart is where a list by date starts: before everything, newest first or oldest first.
func dateListStart(sort string, isDescending bool) listPosition {
	edge := int64(math.MinInt64)
	if isDescending {
		edge = math.MaxInt64
	}
	if sort == SortReleased {
		return listPosition{key: releasedKey(&edge), id: edge}
	}
	times := make([]int64, timesInKey(sort))
	for index := range times {
		times[index] = edge
	}
	return listPosition{key: timeKey(times...), id: edge}
}

// timesInKey is how many times a list by date's key holds: a found time and its file's time, a
// recorded time, or a release day.
func timesInKey(sort string) int {
	if sort == SortAdded {
		return 2
	}
	return 1
}

// keyLength is how long a list by date's key is: its times, eight bytes each, and by release date
// a byte before them that says whether it is dated.
func keyLength(sort string) int {
	if sort == SortReleased {
		return 1 + 8
	}
	return 8 * timesInKey(sort)
}

// timeKey is a list by date's key: its times, each eight bytes, big-endian, with the sign bit
// flipped so the bytes compare as the times do, a time before 1970 among them.
func timeKey(times ...int64) []byte {
	key := make([]byte, 0, 8*len(times))
	for _, at := range times {
		key = binary.BigEndian.AppendUint64(key, uint64(at)^signBit) //nolint:gosec // a time, every bit kept
	}
	return key
}

// signBit is an int64's sign bit, flipped in a time's key.
const signBit = uint64(1) << 63

// keyTimes are a list by date's key's times.
func keyTimes(key []byte) (first, second int64) {
	first = int64(binary.BigEndian.Uint64(key[:8]) ^ signBit) //nolint:gosec // a time we encoded
	if len(key) >= 16 {
		second = int64(binary.BigEndian.Uint64(key[8:16]) ^ signBit) //nolint:gosec // a time we encoded
	}
	return first, second
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

// EncodeCursor is a cursor: the last entry's sort key and id, base64url, opaque to clients. A list
// that pages by a key and an id makes its cursor here and reads it with DecodeCursor.
func EncodeCursor(key []byte, id int64) string {
	raw := binary.BigEndian.AppendUint64(append([]byte(nil), key...), uint64(id)) //nolint:gosec // an id
	return base64.RawURLEncoding.EncodeToString(raw)
}

// DecodeCursor is a cursor's key and id: empty for the first page's, and ErrBadCursor for one this
// server did not give.
func DecodeCursor(cursor string) ([]byte, int64, error) {
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

// Int64Key is a number as a cursor's key: eight bytes, big-endian.
func Int64Key(value int64) []byte {
	return binary.BigEndian.AppendUint64(nil, uint64(value)) //nolint:gosec // read back by KeyInt64
}

// KeyInt64 reads Int64Key's number back; false for a key that is not one.
func KeyInt64(key []byte) (int64, bool) {
	if len(key) != 8 {
		return 0, false
	}
	return int64(binary.BigEndian.Uint64(key)), true //nolint:gosec // written by Int64Key
}

// float64Key is a number as a cursor's key: its eight bytes, big-endian, read back exactly.
func float64Key(value float64) []byte {
	return binary.BigEndian.AppendUint64(nil, math.Float64bits(value))
}

// keyFloat64 reads float64Key's number back; false for a key that is not one.
func keyFloat64(key []byte) (float64, bool) {
	if len(key) != 8 {
		return 0, false
	}
	value := math.Float64frombits(binary.BigEndian.Uint64(key))
	return value, !math.IsNaN(value)
}

// sorter is the sorter of the sortLocale lists sort in (D47).
func (browser *Browser) sorter(ctx context.Context) (*textindex.Sorter, error) {
	locale, err := browser.settings.String(ctx, settings.SortLocale)
	if err != nil {
		return nil, fmt.Errorf("read sortLocale: %w", err)
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
