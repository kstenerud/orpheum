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
	}
	return browser.listFilmsOrSeriesByAdded(ctx, kind, query, after)
}

// listFilmsOrSeriesByName is films or series in the sort_locale's order (D47).
func (browser *Browser) listFilmsOrSeriesByName(
	ctx context.Context, kind string, query ListQuery, after listPosition,
) ([]keyedEntry, error) {
	sorter, err := browser.sorter(ctx)
	if err != nil {
		return nil, err
	}
	queries := browser.database.Read()
	isEveryLibrary, libraryIDsJSON := query.Reach.QueryArgs()
	var keyed []keyedEntry
	if kind == ListSeries {
		rows, listErr := queries.ListSeriesByName(ctx, sqlcgen.ListSeriesByNameParams{
			Locale: sorter.Locale(), AfterKey: after.key, AfterID: after.id, MaxRows: query.Limit,
			IsEveryLibrary: isEveryLibrary, LibraryIdsJSON: libraryIDsJSON,
		})
		for _, row := range rows {
			keyed = append(keyed, keyedEntry{key: row.CollationKey, entry: Entry{
				ID: row.ItemID, Kind: ListSeries, Name: ptr.ValueOrZero(row.Name), Year: ptr.ValueOrZero(row.Year),
				IdentificationState: ptr.ValueOrZero(row.IdentificationState),
				AddedAt:             row.AddedAt, IsGuessed: row.IsGuessed,
			}})
		}
		return keyed, listErr
	}
	rows, err := queries.ListFilmsByName(ctx, sqlcgen.ListFilmsByNameParams{
		Locale: sorter.Locale(), AfterKey: after.key, AfterID: after.id, MaxRows: query.Limit,
		IsEveryLibrary: isEveryLibrary, LibraryIdsJSON: libraryIDsJSON,
	})
	for _, row := range rows {
		keyed = append(keyed, keyedEntry{key: row.CollationKey, entry: Entry{
			ID: row.ItemID, Kind: ListFilms, Name: ptr.ValueOrZero(row.Name), Year: ptr.ValueOrZero(row.Year),
			IdentificationState: row.IdentificationState, AddedAt: row.AddedAt, IsGuessed: row.IsGuessed,
		}})
	}
	return keyed, err
}

// listFilmsOrSeriesByAdded is films or series newest first: by the scan that found it, most recent
// first, then by its file's own time, newest first, then by id (the owner, 2026-10-25; step 0058's
// found_at). A film is as new as the scan that first found it; a series as its newest episode.
func (browser *Browser) listFilmsOrSeriesByAdded(
	ctx context.Context, kind string, query ListQuery, before listPosition,
) ([]keyedEntry, error) {
	foundAt, modifiedAt := keyTimes(before.key)
	queries := browser.database.Read()
	isEveryLibrary, libraryIDsJSON := query.Reach.QueryArgs()
	var keyed []keyedEntry
	if kind == ListSeries {
		rows, err := queries.ListSeriesByAdded(ctx, sqlcgen.ListSeriesByAddedParams{
			BeforeFoundAt: foundAt, BeforeModifiedAt: modifiedAt, BeforeID: before.id, MaxRows: query.Limit,
			IsEveryLibrary: isEveryLibrary, LibraryIdsJSON: libraryIDsJSON,
		})
		for _, row := range rows {
			keyed = append(keyed, keyedEntry{key: timeKey(row.FoundAt, row.FoundFileModifiedAt), entry: Entry{
				ID: row.ItemID, Kind: ListSeries, Name: ptr.ValueOrZero(row.Name), Year: ptr.ValueOrZero(row.Year),
				IdentificationState: ptr.ValueOrZero(row.IdentificationState),
				AddedAt:             row.AddedAt, IsGuessed: row.IsGuessed,
			}})
		}
		return keyed, err
	}
	rows, err := queries.ListFilmsByAdded(ctx, sqlcgen.ListFilmsByAddedParams{
		BeforeFoundAt: foundAt, BeforeModifiedAt: modifiedAt, BeforeID: before.id, MaxRows: query.Limit,
		IsEveryLibrary: isEveryLibrary, LibraryIdsJSON: libraryIDsJSON,
	})
	for _, row := range rows {
		keyed = append(keyed, keyedEntry{key: timeKey(row.FoundAt, row.FoundFileModifiedAt), entry: Entry{
			ID: row.ItemID, Kind: ListFilms, Name: ptr.ValueOrZero(row.Name), Year: ptr.ValueOrZero(row.Year),
			IdentificationState: row.IdentificationState, AddedAt: row.AddedAt, IsGuessed: row.IsGuessed,
		}})
	}
	return keyed, err
}

// otherEntry is an other video as a list shows it.
func otherEntry(id int64, name *string, state string, addedAt int64, facts *OtherFacts) Entry {
	return Entry{
		ID: id, Kind: ListOther, Name: ptr.ValueOrZero(name), IdentificationState: state, AddedAt: addedAt,
		Other: facts,
	}
}

// listOther is other videos in one of their four orders (D349 §4).
func (browser *Browser) listOther(ctx context.Context, query ListQuery, after listPosition) ([]keyedEntry, error) {
	switch query.Sort {
	case SortName:
		return browser.listOtherByName(ctx, query, after)
	case SortFileName:
		return browser.listOtherByFileName(ctx, query, after)
	case SortRecorded:
		return browser.listOtherByRecorded(ctx, query, after)
	}
	return browser.listOtherByAdded(ctx, query, after)
}

func (browser *Browser) listOtherByName(
	ctx context.Context, query ListQuery, after listPosition,
) ([]keyedEntry, error) {
	sorter, err := browser.sorter(ctx)
	if err != nil {
		return nil, err
	}
	isEveryLibrary, libraryIDsJSON := query.Reach.QueryArgs()
	rows, err := browser.database.Read().ListOtherByName(ctx, sqlcgen.ListOtherByNameParams{
		Locale: sorter.Locale(), AfterKey: after.key, AfterID: after.id, MaxRows: query.Limit,
		IsEveryLibrary: isEveryLibrary, LibraryIdsJSON: libraryIDsJSON,
	})
	keyed := make([]keyedEntry, 0, len(rows))
	for _, row := range rows {
		facts := otherFactsOf(row.RecordedAt, row.RecordedFrom, row.FileName)
		keyed = append(keyed, keyedEntry{key: row.CollationKey, entry: otherEntry(
			row.ItemID, row.Name, row.IdentificationState, row.AddedAt, facts)})
	}
	return keyed, err
}

func (browser *Browser) listOtherByFileName(
	ctx context.Context, query ListQuery, after listPosition,
) ([]keyedEntry, error) {
	isEveryLibrary, libraryIDsJSON := query.Reach.QueryArgs()
	rows, err := browser.database.Read().ListOtherByFileName(ctx, sqlcgen.ListOtherByFileNameParams{
		AfterKey: after.key, AfterID: after.id, MaxRows: query.Limit,
		IsEveryLibrary: isEveryLibrary, LibraryIdsJSON: libraryIDsJSON,
	})
	keyed := make([]keyedEntry, 0, len(rows))
	for _, row := range rows {
		facts := &OtherFacts{RecordedAtMs: row.RecordedAt, RecordedFrom: row.RecordedFrom, FileName: row.FileName}
		keyed = append(keyed, keyedEntry{key: row.FileNameKey, entry: otherEntry(
			row.ItemID, row.Name, row.IdentificationState, row.AddedAt, facts)})
	}
	return keyed, err
}

func (browser *Browser) listOtherByRecorded(
	ctx context.Context, query ListQuery, before listPosition,
) ([]keyedEntry, error) {
	recordedAt, _ := keyTimes(before.key)
	isEveryLibrary, libraryIDsJSON := query.Reach.QueryArgs()
	rows, err := browser.database.Read().ListOtherByRecorded(ctx, sqlcgen.ListOtherByRecordedParams{
		BeforeRecordedAt: recordedAt, BeforeID: before.id, MaxRows: query.Limit,
		IsEveryLibrary: isEveryLibrary, LibraryIdsJSON: libraryIDsJSON,
	})
	keyed := make([]keyedEntry, 0, len(rows))
	for _, row := range rows {
		facts := &OtherFacts{RecordedAtMs: row.RecordedAt, RecordedFrom: row.RecordedFrom, FileName: row.FileName}
		keyed = append(keyed, keyedEntry{key: timeKey(row.RecordedAt), entry: otherEntry(
			row.ItemID, row.Name, row.IdentificationState, row.AddedAt, facts)})
	}
	return keyed, err
}

func (browser *Browser) listOtherByAdded(
	ctx context.Context, query ListQuery, before listPosition,
) ([]keyedEntry, error) {
	foundAt, modifiedAt := keyTimes(before.key)
	isEveryLibrary, libraryIDsJSON := query.Reach.QueryArgs()
	rows, err := browser.database.Read().ListOtherByAdded(ctx, sqlcgen.ListOtherByAddedParams{
		BeforeFoundAt: foundAt, BeforeModifiedAt: modifiedAt, BeforeID: before.id, MaxRows: query.Limit,
		IsEveryLibrary: isEveryLibrary, LibraryIdsJSON: libraryIDsJSON,
	})
	keyed := make([]keyedEntry, 0, len(rows))
	for _, row := range rows {
		facts := otherFactsOf(row.RecordedAt, row.RecordedFrom, row.FileName)
		keyed = append(keyed, keyedEntry{key: timeKey(row.FoundAt, row.FoundFileModifiedAt), entry: otherEntry(
			row.ItemID, row.Name, row.IdentificationState, row.AddedAt, facts)})
	}
	return keyed, err
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
