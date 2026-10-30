// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"cmp"
	"context"

	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// The first byte of a list by release date's key: whether it is dated. An undated title or series
// comes after every dated one whichever way the list runs, and those by id (D366 §3).
const (
	keyDated   = 0
	keyUndated = 1
)

// releasedKey is a list by release date's key: dated, and its day (store's released_on, YYYYMMDD),
// or undated.
func releasedKey(releasedOn *int64) []byte {
	if releasedOn == nil {
		return append([]byte{keyUndated}, make([]byte, 8)...)
	}
	return append([]byte{keyDated}, timeKey(*releasedOn)...)
}

// isUndated is whether a list by release date's key is of a title or series with no date.
func isUndated(key []byte) bool {
	return len(key) > 0 && key[0] == keyUndated
}

// compareDating orders two entries of a list by release date by whether they are dated, the
// undated last, and two undated by id; 0 for two dated, which their days order.
func compareDating(first, second keyedEntry) int {
	isFirstUndated, isSecondUndated := isUndated(first.key), isUndated(second.key)
	switch {
	case isFirstUndated != isSecondUndated && isFirstUndated:
		return 1
	case isFirstUndated != isSecondUndated:
		return -1
	case isFirstUndated:
		return cmp.Compare(first.entry.ID, second.entry.ID)
	}
	return 0
}

// releasedReads are a page by release date's reads: its dated rows from a place, newest or oldest
// first, then those with no date, by id, after one. Each takes the films' parameters, which a series'
// query takes as its own.
type releasedReads struct {
	dated   func(sqlcgen.ListFilmsByReleasedParams) ([]keyedEntry, error)
	undated func(sqlcgen.ListFilmsUndatedParams) ([]keyedEntry, error)
}

// listFilmsOrSeriesByReleased is films by their release date or series by their first air date,
// newest or oldest first, those with none after them (D366 §1, §3).
func (browser *Browser) listFilmsOrSeriesByReleased(
	ctx context.Context, kind string, query ListQuery, from listPosition,
) ([]keyedEntry, error) {
	reads := browser.filmsReleased(ctx, query.isReversed())
	if kind == ListSeries {
		reads = browser.seriesReleased(ctx, query.isReversed())
	}
	isEveryLibrary, libraryIDsJSON := query.Reach.QueryArgs()
	itemIDsJSON := query.matched.idsJSON
	var keyed []keyedEntry
	afterUndatedID := from.id
	if !isUndated(from.key) {
		day, _ := keyTimes(from.key[1:])
		dated, err := reads.dated(sqlcgen.ListFilmsByReleasedParams{
			BeforeReleasedOn: day, BeforeID: from.id, MaxRows: query.Limit,
			IsEveryLibrary: isEveryLibrary, LibraryIdsJSON: libraryIDsJSON,
			ItemIdsJSON: itemIDsJSON,
		})
		if err != nil || int64(len(dated)) >= query.Limit {
			return dated, err
		}
		keyed, afterUndatedID = dated, 0
	}
	undated, err := reads.undated(sqlcgen.ListFilmsUndatedParams{
		AfterID: afterUndatedID, MaxRows: query.Limit - int64(len(keyed)),
		IsEveryLibrary: isEveryLibrary, LibraryIdsJSON: libraryIDsJSON,
		ItemIdsJSON: itemIDsJSON,
	})
	return append(keyed, undated...), err
}

// oldestReleasedFirst is a page oldest released first from where its newest-first twin's would
// start.
func oldestReleasedFirst(forward sqlcgen.ListFilmsByReleasedParams) sqlcgen.ListFilmsByReleasedAscendingParams {
	return sqlcgen.ListFilmsByReleasedAscendingParams{
		AfterReleasedOn: forward.BeforeReleasedOn, AfterID: forward.BeforeID, MaxRows: forward.MaxRows,
		IsEveryLibrary: forward.IsEveryLibrary, LibraryIdsJSON: forward.LibraryIdsJSON,
		ItemIdsJSON: forward.ItemIdsJSON,
	}
}

// filmsReleased are films' reads by release date.
func (browser *Browser) filmsReleased(ctx context.Context, isReversed bool) releasedReads {
	queries := browser.database.Read()
	entryOf := func(row sqlcgen.ListFilmsByReleasedRow) keyedEntry {
		return keyedEntry{key: releasedKey(row.ReleasedOn), entry: Entry{
			ID: row.ItemID, Kind: ListFilms, Name: ptr.ValueOrZero(row.Name), Year: ptr.ValueOrZero(row.Year),
			IdentificationState: row.IdentificationState, AddedAt: row.AddedAt, IsGuessed: row.IsGuessed,
		}}
	}
	list := eitherWay[sqlcgen.ListFilmsByReleasedParams, sqlcgen.ListFilmsByReleasedAscendingParams,
		sqlcgen.ListFilmsByReleasedRow, sqlcgen.ListFilmsByReleasedAscendingRow]{
		forward: queries.ListFilmsByReleased, reversed: queries.ListFilmsByReleasedAscending,
		paramsOf: oldestReleasedFirst,
		rowOf: func(row sqlcgen.ListFilmsByReleasedAscendingRow) sqlcgen.ListFilmsByReleasedRow {
			return sqlcgen.ListFilmsByReleasedRow(row)
		},
	}
	return releasedReads{
		dated: func(params sqlcgen.ListFilmsByReleasedParams) ([]keyedEntry, error) {
			rows, err := list.read(ctx, isReversed, params)
			return mapped(rows, entryOf), err
		},
		undated: func(params sqlcgen.ListFilmsUndatedParams) ([]keyedEntry, error) {
			rows, err := queries.ListFilmsUndated(ctx, params)
			return mapped(rows, func(row sqlcgen.ListFilmsUndatedRow) keyedEntry {
				return entryOf(sqlcgen.ListFilmsByReleasedRow(row))
			}), err
		},
	}
}

// seriesReleased are series' reads by first air date.
func (browser *Browser) seriesReleased(ctx context.Context, isReversed bool) releasedReads {
	queries := browser.database.Read()
	entryOf := func(row sqlcgen.ListSeriesByReleasedRow) keyedEntry {
		return keyedEntry{key: releasedKey(row.ReleasedOn), entry: Entry{
			ID: row.ItemID, Kind: ListSeries, Name: ptr.ValueOrZero(row.Name), Year: ptr.ValueOrZero(row.Year),
			IdentificationState: ptr.ValueOrZero(row.IdentificationState), AddedAt: row.AddedAt,
			IsGuessed: row.IsGuessed,
		}}
	}
	list := eitherWay[sqlcgen.ListSeriesByReleasedParams, sqlcgen.ListSeriesByReleasedAscendingParams,
		sqlcgen.ListSeriesByReleasedRow, sqlcgen.ListSeriesByReleasedAscendingRow]{
		forward: queries.ListSeriesByReleased, reversed: queries.ListSeriesByReleasedAscending,
		paramsOf: func(forward sqlcgen.ListSeriesByReleasedParams) sqlcgen.ListSeriesByReleasedAscendingParams {
			return sqlcgen.ListSeriesByReleasedAscendingParams(
				oldestReleasedFirst(sqlcgen.ListFilmsByReleasedParams(forward)))
		},
		rowOf: func(row sqlcgen.ListSeriesByReleasedAscendingRow) sqlcgen.ListSeriesByReleasedRow {
			return sqlcgen.ListSeriesByReleasedRow(row)
		},
	}
	return releasedReads{
		dated: func(params sqlcgen.ListFilmsByReleasedParams) ([]keyedEntry, error) {
			rows, err := list.read(ctx, isReversed, sqlcgen.ListSeriesByReleasedParams(params))
			return mapped(rows, entryOf), err
		},
		undated: func(params sqlcgen.ListFilmsUndatedParams) ([]keyedEntry, error) {
			rows, err := queries.ListSeriesUndated(ctx, sqlcgen.ListSeriesUndatedParams(params))
			return mapped(rows, func(row sqlcgen.ListSeriesUndatedRow) keyedEntry {
				return entryOf(sqlcgen.ListSeriesByReleasedRow(row))
			}), err
		},
	}
}

// mapped is each of rows made an entry.
func mapped[Row any](rows []Row, entryOf func(Row) keyedEntry) []keyedEntry {
	keyed := make([]keyedEntry, 0, len(rows))
	for _, row := range rows {
		keyed = append(keyed, entryOf(row))
	}
	return keyed
}
