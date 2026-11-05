// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"context"

	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// otherEntry is an other video as a list shows it.
func otherEntry(id int64, name *string, state string, addedAt int64, facts *OtherFacts) Entry {
	return Entry{
		ID: id, Kind: ListOther, Name: ptr.ValueOrZero(name), IdentificationState: state, AddedAt: addedAt,
		Other: facts,
	}
}

// listOther is other videos in one of their four orders (D349 §4), either way (D366 §2).
func (browser *Browser) listOther(ctx context.Context, query ListQuery, from listPosition) ([]keyedEntry, error) {
	switch query.Sort {
	case SortName:
		return browser.listOtherByName(ctx, query, from)
	case SortFileName:
		return browser.listOtherByFileName(ctx, query, from)
	case SortRecorded:
		return browser.listOtherByRecorded(ctx, query, from)
	}
	return browser.listOtherByAdded(ctx, query, from)
}

func (browser *Browser) listOtherByName(
	ctx context.Context, query ListQuery, after listPosition,
) ([]keyedEntry, error) {
	sorter, err := browser.sorter(ctx)
	if err != nil {
		return nil, err
	}
	queries := browser.database.Read()
	isEveryLibrary, libraryIDsJSON := query.Reach.QueryArgs()
	itemIDsJSON := query.matched.idsJSON
	rows, err := eitherWay[sqlcgen.ListOtherByNameParams, sqlcgen.ListOtherByNameDescendingParams,
		sqlcgen.ListOtherByNameRow, sqlcgen.ListOtherByNameDescendingRow]{
		forward: queries.ListOtherByName, reversed: queries.ListOtherByNameDescending,
		paramsOf: func(forward sqlcgen.ListOtherByNameParams) sqlcgen.ListOtherByNameDescendingParams {
			return sqlcgen.ListOtherByNameDescendingParams(forward)
		},
		rowOf: func(row sqlcgen.ListOtherByNameDescendingRow) sqlcgen.ListOtherByNameRow {
			return sqlcgen.ListOtherByNameRow(row)
		},
	}.read(ctx, query.isReversed(), sqlcgen.ListOtherByNameParams{
		Locale: sorter.Locale(), AfterKey: after.key, AfterID: after.id, MaxRows: query.Limit,
		IsEveryLibrary: isEveryLibrary, LibraryIdsJSON: libraryIDsJSON,
		ItemIdsJSON: itemIDsJSON,
	})
	keyed := make([]keyedEntry, 0, len(rows))
	for _, row := range rows {
		facts := otherFactsOf(row.RecordedAt, row.RecordedFrom, row.FileName)
		keyed = append(keyed, keyedEntry{key: row.CollationKey, entry: otherEntry(
			row.ItemID, row.Name, ptr.ValueOrZero(row.IdentificationState), row.AddedAt, facts)})
	}
	return keyed, err
}

func (browser *Browser) listOtherByFileName(
	ctx context.Context, query ListQuery, after listPosition,
) ([]keyedEntry, error) {
	queries := browser.database.Read()
	isEveryLibrary, libraryIDsJSON := query.Reach.QueryArgs()
	itemIDsJSON := query.matched.idsJSON
	rows, err := eitherWay[sqlcgen.ListOtherByFileNameParams, sqlcgen.ListOtherByFileNameDescendingParams,
		sqlcgen.ListOtherByFileNameRow, sqlcgen.ListOtherByFileNameDescendingRow]{
		forward: queries.ListOtherByFileName, reversed: queries.ListOtherByFileNameDescending,
		paramsOf: func(forward sqlcgen.ListOtherByFileNameParams) sqlcgen.ListOtherByFileNameDescendingParams {
			return sqlcgen.ListOtherByFileNameDescendingParams(forward)
		},
		rowOf: func(row sqlcgen.ListOtherByFileNameDescendingRow) sqlcgen.ListOtherByFileNameRow {
			return sqlcgen.ListOtherByFileNameRow(row)
		},
	}.read(ctx, query.isReversed(), sqlcgen.ListOtherByFileNameParams{
		AfterKey: after.key, AfterID: after.id, MaxRows: query.Limit,
		IsEveryLibrary: isEveryLibrary, LibraryIdsJSON: libraryIDsJSON,
		ItemIdsJSON: itemIDsJSON,
	})
	keyed := make([]keyedEntry, 0, len(rows))
	for _, row := range rows {
		facts := &OtherFacts{RecordedAtMs: row.RecordedAt, RecordedFrom: row.RecordedFrom, FileName: row.FileName}
		keyed = append(keyed, keyedEntry{key: row.FileNameKey, entry: otherEntry(
			row.ItemID, row.Name, ptr.ValueOrZero(row.IdentificationState), row.AddedAt, facts)})
	}
	return keyed, err
}

func (browser *Browser) listOtherByRecorded(
	ctx context.Context, query ListQuery, from listPosition,
) ([]keyedEntry, error) {
	recordedAt, _ := keyTimes(from.key)
	queries := browser.database.Read()
	isEveryLibrary, libraryIDsJSON := query.Reach.QueryArgs()
	itemIDsJSON := query.matched.idsJSON
	rows, err := eitherWay[sqlcgen.ListOtherByRecordedParams, sqlcgen.ListOtherByRecordedAscendingParams,
		sqlcgen.ListOtherByRecordedRow, sqlcgen.ListOtherByRecordedAscendingRow]{
		forward: queries.ListOtherByRecorded, reversed: queries.ListOtherByRecordedAscending,
		paramsOf: func(forward sqlcgen.ListOtherByRecordedParams) sqlcgen.ListOtherByRecordedAscendingParams {
			return sqlcgen.ListOtherByRecordedAscendingParams{
				AfterRecordedAt: forward.BeforeRecordedAt, AfterID: forward.BeforeID, MaxRows: forward.MaxRows,
				IsEveryLibrary: forward.IsEveryLibrary, LibraryIdsJSON: forward.LibraryIdsJSON,
				ItemIdsJSON: forward.ItemIdsJSON,
			}
		},
		rowOf: func(row sqlcgen.ListOtherByRecordedAscendingRow) sqlcgen.ListOtherByRecordedRow {
			return sqlcgen.ListOtherByRecordedRow(row)
		},
	}.read(ctx, query.isReversed(), sqlcgen.ListOtherByRecordedParams{
		BeforeRecordedAt: recordedAt, BeforeID: from.id, MaxRows: query.Limit,
		IsEveryLibrary: isEveryLibrary, LibraryIdsJSON: libraryIDsJSON,
		ItemIdsJSON: itemIDsJSON,
	})
	keyed := make([]keyedEntry, 0, len(rows))
	for _, row := range rows {
		facts := &OtherFacts{RecordedAtMs: row.RecordedAt, RecordedFrom: row.RecordedFrom, FileName: row.FileName}
		keyed = append(keyed, keyedEntry{key: timeKey(row.RecordedAt), entry: otherEntry(
			row.ItemID, row.Name, ptr.ValueOrZero(row.IdentificationState), row.AddedAt, facts)})
	}
	return keyed, err
}

func (browser *Browser) listOtherByAdded(
	ctx context.Context, query ListQuery, from listPosition,
) ([]keyedEntry, error) {
	foundAt, modifiedAt := keyTimes(from.key)
	queries := browser.database.Read()
	isEveryLibrary, libraryIDsJSON := query.Reach.QueryArgs()
	itemIDsJSON := query.matched.idsJSON
	rows, err := eitherWay[sqlcgen.ListOtherByAddedParams, sqlcgen.ListOtherByAddedAscendingParams,
		sqlcgen.ListOtherByAddedRow, sqlcgen.ListOtherByAddedAscendingRow]{
		forward: queries.ListOtherByAdded, reversed: queries.ListOtherByAddedAscending,
		paramsOf: func(forward sqlcgen.ListOtherByAddedParams) sqlcgen.ListOtherByAddedAscendingParams {
			return sqlcgen.ListOtherByAddedAscendingParams(oldestAddedFirst(sqlcgen.ListFilmsByAddedParams(forward)))
		},
		rowOf: func(row sqlcgen.ListOtherByAddedAscendingRow) sqlcgen.ListOtherByAddedRow {
			return sqlcgen.ListOtherByAddedRow(row)
		},
	}.read(ctx, query.isReversed(), sqlcgen.ListOtherByAddedParams{
		BeforeFoundAt: foundAt, BeforeModifiedAt: modifiedAt, BeforeID: from.id, MaxRows: query.Limit,
		IsEveryLibrary: isEveryLibrary, LibraryIdsJSON: libraryIDsJSON,
		ItemIdsJSON: itemIDsJSON,
	})
	keyed := make([]keyedEntry, 0, len(rows))
	for _, row := range rows {
		facts := otherFactsOf(row.RecordedAt, row.RecordedFrom, row.FileName)
		keyed = append(keyed, keyedEntry{key: timeKey(row.FoundAt, row.FoundFileModifiedAt), entry: otherEntry(
			row.ItemID, row.Name, ptr.ValueOrZero(row.IdentificationState), row.AddedAt, facts)})
	}
	return keyed, err
}
