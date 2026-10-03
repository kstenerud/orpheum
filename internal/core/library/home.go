// SPDX-License-Identifier: GPL-3.0-or-later

package library

import (
	"cmp"
	"context"
	"fmt"
	"path"
	"slices"

	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// recentPositionsReadPerWork is how many of a person's most recent positions Continue Watching is
// chosen from, for each work it may show (continue_watching_max_works): enough for its rows once
// finished and barely started files are passed over.
const recentPositionsReadPerWork = 10

// InProgress is one row of Continue Watching: a work, and the file of it the person was most
// recently partway through (play-state.md).
type InProgress struct {
	Entry
	FileID     int64
	PositionMs int64
	// DurationMs is the file's length, or 0 when its probe did not know it.
	DurationMs int64
	// FileName names the file when more than one of the work's files is partway through, so the
	// row says which it resumes; empty otherwise.
	FileName string
	// Episode is an episode's series and place; nil for a film.
	Episode *sqlcgen.GetEpisodePlaceRow
}

// ContinueWatching is what the person stopped in the middle of (play-state.md): each work once,
// most recent watching first, at its most recent position that is past resume_min_percent and not
// past finished_after_percent of its file's length. A file of unknown length counts at any
// position. A work hidden since its last watching is left out (D188); extras never appear (D162).
func (browser *Browser) ContinueWatching(ctx context.Context, userID int64) ([]InProgress, error) {
	thresholds, err := ReadResumeThresholds(ctx, browser.settings)
	if err != nil {
		return nil, err
	}
	maxWorks, err := browser.settings.Int(ctx, settings.ContinueWatchingMaxWorks)
	if err != nil {
		return nil, fmt.Errorf("read continue_watching_max_works: %w", err)
	}
	rows, err := browser.database.Read().ListRecentPlayPositions(ctx, sqlcgen.ListRecentPlayPositionsParams{
		UserID: userID, MaxRows: maxWorks * recentPositionsReadPerWork,
	})
	if err != nil {
		return nil, fmt.Errorf("read the recent positions: %w", err)
	}
	var inProgress []InProgress
	rowIndexByTitle := map[int64]int{} // a title's row in inProgress
	partwayFileCountByTitle := map[int64]int{}
	latestWatchedAt := map[int64]int64{} // a title's most recent watching, in any file
	for _, row := range rows {
		if _, seen := latestWatchedAt[row.TitleID]; !seen {
			latestWatchedAt[row.TitleID] = row.PositionReadAt
		}
		if row.HiddenAt != nil && *row.HiddenAt >= latestWatchedAt[row.TitleID] {
			continue
		}
		if !thresholds.IsPartwayThrough(row.PositionMs, row.DurationMs) {
			continue
		}
		partwayFileCountByTitle[row.TitleID]++
		if _, placed := rowIndexByTitle[row.TitleID]; placed || int64(len(inProgress)) == maxWorks {
			continue
		}
		rowIndexByTitle[row.TitleID] = len(inProgress)
		inProgress = append(inProgress, InProgress{
			Entry: Entry{
				ID: row.TitleID, Name: ptr.ValueOrZero(row.Name), Year: ptr.ValueOrZero(row.Year),
				LibraryID: ptr.ValueOrZero(row.LibraryID),
			},
			FileID: row.FileID, PositionMs: row.PositionMs, DurationMs: row.DurationMs, FileName: path.Base(row.Path),
		})
		if row.Kind == TitleKindEpisode {
			if inProgress[len(inProgress)-1].Episode, err = browser.episodePlace(ctx, row.TitleID); err != nil {
				return nil, err
			}
		}
	}
	// A row is ordered by its work's latest watching, in any of its files: finishing one copy is
	// activity, though the row resumes another.
	slices.SortStableFunc(inProgress, func(first, second InProgress) int {
		return cmp.Compare(latestWatchedAt[second.ID], latestWatchedAt[first.ID])
	})
	return browser.withPostersAndFileNames(ctx, inProgress, partwayFileCountByTitle)
}

// withPostersAndFileNames gives each row its poster, and drops the file's name where the work has only
// one file partway through.
func (browser *Browser) withPostersAndFileNames(
	ctx context.Context, rows []InProgress, partwayFileCountByTitle map[int64]int,
) ([]InProgress, error) {
	entries := make([]Entry, 0, len(rows))
	for index := range rows {
		if partwayFileCountByTitle[rows[index].ID] < 2 {
			rows[index].FileName = ""
		}
		entries = append(entries, rows[index].Entry)
	}
	posterPage, err := browser.finishPage(ctx, ListFilms, Page{Entries: entries})
	if err != nil {
		return nil, err
	}
	for index := range rows {
		rows[index].Entry = posterPage.Entries[index]
	}
	if rows == nil {
		rows = []InProgress{}
	}
	return rows, nil
}

// ResumeThresholds are resume_min_percent and finished_after_percent: where in a file a position
// counts as partway through it (play-state.md). Continue Watching and a playback's starting point
// both read them here.
type ResumeThresholds struct {
	resumeMinPercent, finishedAfterPercent int64
}

// IntSettings reads an integer setting: settings.Service.
type IntSettings interface {
	Int(ctx context.Context, key settings.Key) (int64, error)
}

// ReadResumeThresholds reads the two thresholds from the settings.
func ReadResumeThresholds(ctx context.Context, config IntSettings) (ResumeThresholds, error) {
	minimum, err := config.Int(ctx, settings.ResumeMinPercent)
	if err != nil {
		return ResumeThresholds{}, fmt.Errorf("read resume_min_percent: %w", err)
	}
	finished, err := config.Int(ctx, settings.FinishedAfterPercent)
	if err != nil {
		return ResumeThresholds{}, fmt.Errorf("read finished_after_percent: %w", err)
	}
	return ResumeThresholds{resumeMinPercent: minimum, finishedAfterPercent: finished}, nil
}

// IsPartwayThrough is whether a position is partway through a file: past the first threshold and not
// past the second, compared as products so no rounding moves the edges. A file of unknown length
// is partway through at any position but its start: a missing fact never becomes "finished".
func (thresholds ResumeThresholds) IsPartwayThrough(positionMs, durationMs int64) bool {
	if durationMs <= 0 {
		return positionMs > 0
	}
	return positionMs*100 >= thresholds.resumeMinPercent*durationMs &&
		positionMs*100 <= thresholds.finishedAfterPercent*durationMs
}
