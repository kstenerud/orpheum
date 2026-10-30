// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"context"
	"fmt"

	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// listKind is up to a page of one kind's entries from a place in a list's order, each with its key.
func (browser *Browser) listKind(
	ctx context.Context, kind string, query ListQuery, after listPosition,
) ([]keyedEntry, error) {
	switch {
	case kind == ListOther:
		return browser.listOther(ctx, query, after)
	case query.Sort == SortName:
		return browser.listFilmsOrSeriesByName(ctx, kind, query, after)
	case query.Sort == SortReleased:
		return browser.listFilmsOrSeriesByReleased(ctx, kind, query, after)
	}
	return browser.listFilmsOrSeriesByAdded(ctx, kind, query, after)
}

// eitherWay is one list's two queries, its rows read one way and the other (D366 §2): forward in
// the sort's own default direction, reversed the other, with the reversed query's parameters made
// from the forward's and its rows read as the forward's.
type eitherWay[ForwardParams, ReversedParams, ForwardRow, ReversedRow any] struct {
	forward  func(context.Context, ForwardParams) ([]ForwardRow, error)
	reversed func(context.Context, ReversedParams) ([]ReversedRow, error)
	paramsOf func(ForwardParams) ReversedParams
	rowOf    func(ReversedRow) ForwardRow
}

// read is the list's rows from params' place, reversed or not.
func (list eitherWay[ForwardParams, ReversedParams, ForwardRow, ReversedRow]) read(
	ctx context.Context, isReversed bool, params ForwardParams,
) ([]ForwardRow, error) {
	if !isReversed {
		return list.forward(ctx, params)
	}
	rows, err := list.reversed(ctx, list.paramsOf(params))
	read := make([]ForwardRow, 0, len(rows))
	for _, row := range rows {
		read = append(read, list.rowOf(row))
	}
	return read, err
}

// listFilmsOrSeriesByName is films or series in the sort_locale's order (D47), A to Z or Z to A.
func (browser *Browser) listFilmsOrSeriesByName(
	ctx context.Context, kind string, query ListQuery, after listPosition,
) ([]keyedEntry, error) {
	sorter, err := browser.sorter(ctx)
	if err != nil {
		return nil, err
	}
	queries := browser.database.Read()
	isEveryLibrary, libraryIDsJSON := query.Reach.QueryArgs()
	itemIDsJSON := query.matched.idsJSON
	params := sqlcgen.ListFilmsByNameParams{
		Locale: sorter.Locale(), AfterKey: after.key, AfterID: after.id, MaxRows: query.Limit,
		IsEveryLibrary: isEveryLibrary, LibraryIdsJSON: libraryIDsJSON,
		ItemIdsJSON: itemIDsJSON,
	}
	var keyed []keyedEntry
	if kind == ListSeries {
		rows, listErr := eitherWay[sqlcgen.ListSeriesByNameParams, sqlcgen.ListSeriesByNameDescendingParams,
			sqlcgen.ListSeriesByNameRow, sqlcgen.ListSeriesByNameDescendingRow]{
			forward: queries.ListSeriesByName, reversed: queries.ListSeriesByNameDescending,
			paramsOf: func(forward sqlcgen.ListSeriesByNameParams) sqlcgen.ListSeriesByNameDescendingParams {
				return sqlcgen.ListSeriesByNameDescendingParams(forward)
			},
			rowOf: func(row sqlcgen.ListSeriesByNameDescendingRow) sqlcgen.ListSeriesByNameRow {
				return sqlcgen.ListSeriesByNameRow(row)
			},
		}.read(ctx, query.isReversed(), sqlcgen.ListSeriesByNameParams(params))
		for _, row := range rows {
			keyed = append(keyed, keyedEntry{key: row.CollationKey, entry: Entry{
				ID: row.ItemID, Kind: ListSeries, Name: ptr.ValueOrZero(row.Name), Year: ptr.ValueOrZero(row.Year),
				IdentificationState: ptr.ValueOrZero(row.IdentificationState),
				AddedAt:             row.AddedAt, IsGuessed: row.IsGuessed,
			}})
		}
		return keyed, listErr
	}
	rows, err := eitherWay[sqlcgen.ListFilmsByNameParams, sqlcgen.ListFilmsByNameDescendingParams,
		sqlcgen.ListFilmsByNameRow, sqlcgen.ListFilmsByNameDescendingRow]{
		forward: queries.ListFilmsByName, reversed: queries.ListFilmsByNameDescending,
		paramsOf: func(forward sqlcgen.ListFilmsByNameParams) sqlcgen.ListFilmsByNameDescendingParams {
			return sqlcgen.ListFilmsByNameDescendingParams(forward)
		},
		rowOf: func(row sqlcgen.ListFilmsByNameDescendingRow) sqlcgen.ListFilmsByNameRow {
			return sqlcgen.ListFilmsByNameRow(row)
		},
	}.read(ctx, query.isReversed(), params)
	for _, row := range rows {
		keyed = append(keyed, keyedEntry{key: row.CollationKey, entry: Entry{
			ID: row.ItemID, Kind: ListFilms, Name: ptr.ValueOrZero(row.Name), Year: ptr.ValueOrZero(row.Year),
			IdentificationState: row.IdentificationState, AddedAt: row.AddedAt, IsGuessed: row.IsGuessed,
		}})
	}
	return keyed, err
}

// listFilmsOrSeriesByAdded is films or series newest first, or oldest first: by the scan that found
// it, then by its file's own time, then by id (the owner, 2026-10-25; step 0058's found_at). A film
// is as new as the scan that first found it; a series as its newest episode.
func (browser *Browser) listFilmsOrSeriesByAdded(
	ctx context.Context, kind string, query ListQuery, from listPosition,
) ([]keyedEntry, error) {
	foundAt, modifiedAt := keyTimes(from.key)
	queries := browser.database.Read()
	isEveryLibrary, libraryIDsJSON := query.Reach.QueryArgs()
	itemIDsJSON := query.matched.idsJSON
	params := sqlcgen.ListFilmsByAddedParams{
		BeforeFoundAt: foundAt, BeforeModifiedAt: modifiedAt, BeforeID: from.id, MaxRows: query.Limit,
		IsEveryLibrary: isEveryLibrary, LibraryIdsJSON: libraryIDsJSON,
		ItemIdsJSON: itemIDsJSON,
	}
	var keyed []keyedEntry
	if kind == ListSeries {
		rows, err := eitherWay[sqlcgen.ListSeriesByAddedParams, sqlcgen.ListSeriesByAddedAscendingParams,
			sqlcgen.ListSeriesByAddedRow, sqlcgen.ListSeriesByAddedAscendingRow]{
			forward: queries.ListSeriesByAdded, reversed: queries.ListSeriesByAddedAscending,
			paramsOf: func(forward sqlcgen.ListSeriesByAddedParams) sqlcgen.ListSeriesByAddedAscendingParams {
				return sqlcgen.ListSeriesByAddedAscendingParams(
					oldestAddedFirst(sqlcgen.ListFilmsByAddedParams(forward)))
			},
			rowOf: func(row sqlcgen.ListSeriesByAddedAscendingRow) sqlcgen.ListSeriesByAddedRow {
				return sqlcgen.ListSeriesByAddedRow(row)
			},
		}.read(ctx, query.isReversed(), sqlcgen.ListSeriesByAddedParams(params))
		for _, row := range rows {
			keyed = append(keyed, keyedEntry{key: timeKey(row.FoundAt, row.FoundFileModifiedAt), entry: Entry{
				ID: row.ItemID, Kind: ListSeries, Name: ptr.ValueOrZero(row.Name), Year: ptr.ValueOrZero(row.Year),
				IdentificationState: ptr.ValueOrZero(row.IdentificationState),
				AddedAt:             row.AddedAt, IsGuessed: row.IsGuessed,
			}})
		}
		return keyed, err
	}
	rows, err := eitherWay[sqlcgen.ListFilmsByAddedParams, sqlcgen.ListFilmsByAddedAscendingParams,
		sqlcgen.ListFilmsByAddedRow, sqlcgen.ListFilmsByAddedAscendingRow]{
		forward: queries.ListFilmsByAdded, reversed: queries.ListFilmsByAddedAscending,
		paramsOf: oldestAddedFirst,
		rowOf: func(row sqlcgen.ListFilmsByAddedAscendingRow) sqlcgen.ListFilmsByAddedRow {
			return sqlcgen.ListFilmsByAddedRow(row)
		},
	}.read(ctx, query.isReversed(), params)
	for _, row := range rows {
		keyed = append(keyed, keyedEntry{key: timeKey(row.FoundAt, row.FoundFileModifiedAt), entry: Entry{
			ID: row.ItemID, Kind: ListFilms, Name: ptr.ValueOrZero(row.Name), Year: ptr.ValueOrZero(row.Year),
			IdentificationState: row.IdentificationState, AddedAt: row.AddedAt, IsGuessed: row.IsGuessed,
		}})
	}
	return keyed, err
}

// oldestAddedFirst is a page oldest added first from where its newest-first twin's would start; a
// series' and an other video's query take the films' as their own.
func oldestAddedFirst(forward sqlcgen.ListFilmsByAddedParams) sqlcgen.ListFilmsByAddedAscendingParams {
	return sqlcgen.ListFilmsByAddedAscendingParams{
		AfterFoundAt: forward.BeforeFoundAt, AfterModifiedAt: forward.BeforeModifiedAt, AfterID: forward.BeforeID,
		MaxRows: forward.MaxRows, IsEveryLibrary: forward.IsEveryLibrary, LibraryIdsJSON: forward.LibraryIdsJSON,
		ItemIdsJSON: forward.ItemIdsJSON,
	}
}

// LiveLibraryIDs are the libraries there are, for a reach of every library less those a person
// left out (auth.Reach's Without).
func (browser *Browser) LiveLibraryIDs(ctx context.Context) ([]int64, error) {
	ids, err := browser.database.Read().ListLiveLibraryIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("list the libraries: %w", err)
	}
	return ids, nil
}
