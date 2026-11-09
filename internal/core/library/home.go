// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"cmp"
	"context"
	"fmt"
	"path"
	"slices"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// recentPositionsReadPerWork is how many of a person's most recent positions Continue Watching is
// chosen from, for each work it may show (continueWatchingMaxWorks): enough for its rows once
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
	// Episode is an episode's series and place, as the person sees it; nil for a film.
	Episode *EpisodePlace
}

// ContinueWatching is what the person stopped in the middle of (play-state.md): each work once,
// most recent watching first, at its most recent position that is past inProgressAfterPercent and not
// past finishedAfterPercent of its file's length. A file of unknown length counts at any
// position. A work hidden since its last watching is left out (D188), and a position watched before
// its work was last marked by hand does not count (D370 §4); extras never appear (D162). A work its
// person watched before, rewatched, says so (D370 §1).
func (browser *Browser) ContinueWatching(ctx context.Context, userID int64, reach auth.Reach) ([]InProgress, error) {
	thresholds, err := ReadResumeThresholds(ctx, browser.settings)
	if err != nil {
		return nil, err
	}
	maxWorks, err := browser.settings.Int(ctx, settings.ContinueWatchingMaxWorks)
	if err != nil {
		return nil, fmt.Errorf("read continueWatchingMaxWorks: %w", err)
	}
	isEveryLibrary, libraryIDsJSON := reach.QueryArgs()
	rows, err := browser.database.Read().ListRecentPlayPositions(ctx, sqlcgen.ListRecentPlayPositionsParams{
		UserID: userID, MaxRows: maxWorks * recentPositionsReadPerWork,
		IsEveryLibrary: isEveryLibrary, LibraryIdsJSON: libraryIDsJSON,
	})
	if err != nil {
		return nil, fmt.Errorf("read the recent positions: %w", err)
	}
	var inProgress []InProgress
	rowIndexByTitle := map[int64]int{} // a title's row in inProgress
	partwayFileCountByTitle := map[int64]int{}
	latestWatchedAt := map[int64]int64{} // a title's most recent watching, in any file
	for _, row := range rows {
		if IsBeforeMark(row.PositionReadAt, row.MarkedAt) {
			continue // marked by hand since: a fresh start (D370 §4)
		}
		if _, seen := latestWatchedAt[row.WorkID]; !seen {
			latestWatchedAt[row.WorkID] = row.PositionReadAt
		}
		if !thresholds.resumesAt(row.PositionMs, row.DurationMs, row.HiddenAt, latestWatchedAt[row.WorkID]) {
			continue
		}
		partwayFileCountByTitle[row.WorkID]++
		if _, placed := rowIndexByTitle[row.WorkID]; placed || int64(len(inProgress)) == maxWorks {
			continue
		}
		rowIndexByTitle[row.WorkID] = len(inProgress)
		inProgress = append(inProgress, InProgress{
			Entry: Entry{
				ID: row.WorkID, Kind: row.Kind, Name: ptr.ValueOrZero(row.Name), Year: ptr.ValueOrZero(row.Year),
			},
			FileID: row.FileID, PositionMs: row.PositionMs, DurationMs: row.DurationMs, FileName: path.Base(row.Path),
		})
		if row.Kind == TitleKindEpisode {
			last := &inProgress[len(inProgress)-1]
			if last.Episode, err = browser.EpisodePlaceOf(ctx, userID, row.WorkID); err != nil {
				return nil, err
			}
		}
	}
	// A row is ordered by its work's latest watching, in any of its files: finishing one copy is
	// activity, though the row resumes another.
	slices.SortStableFunc(inProgress, func(first, second InProgress) int {
		return cmp.Compare(latestWatchedAt[second.ID], latestWatchedAt[first.ID])
	})
	return browser.withPostersAndFileNames(ctx, reach, userID, inProgress, partwayFileCountByTitle)
}

// withPostersAndFileNames gives each row its poster and whether userID watched it before, and drops the
// file's name where the work has only one file partway through.
func (browser *Browser) withPostersAndFileNames(
	ctx context.Context, reach auth.Reach, userID int64, rows []InProgress, partwayFileCountByTitle map[int64]int,
) ([]InProgress, error) {
	entries := make([]Entry, 0, len(rows))
	ids := make([]int64, 0, len(rows))
	for index := range rows {
		if partwayFileCountByTitle[rows[index].ID] < 2 {
			rows[index].FileName = ""
		}
		ids = append(ids, rows[index].ID)
	}
	watched, err := browser.WatchedTitles(ctx, userID, ids)
	if err != nil {
		return nil, err
	}
	for index := range rows {
		rows[index].IsWatched = watched[rows[index].ID]
		entries = append(entries, rows[index].Entry)
	}
	posterPage, err := browser.finishPage(ctx, reach, Page{Entries: entries})
	if err != nil {
		return nil, err
	}
	for index := range rows {
		rows[index].Entry = posterPage.Entries[index]
	}
	if err := browser.withSeriesPosters(ctx, rows); err != nil {
		return nil, err
	}
	if rows == nil {
		rows = []InProgress{}
	}
	return rows, nil
}

// withSeriesPosters gives each episode's row that has no poster of its own, which an episode never
// has (its picture is a still), its series' poster: a row of Continue Watching is a poster card, and
// an episode's showed the mark of a missing poster (the owner, 2026-11-01).
func (browser *Browser) withSeriesPosters(ctx context.Context, rows []InProgress) error {
	var seriesIDs []int64
	for _, row := range rows {
		if row.PosterSha256 == "" && row.Episode != nil {
			seriesIDs = append(seriesIDs, row.Episode.SequenceID)
		}
	}
	if len(seriesIDs) == 0 {
		return nil
	}
	posters, err := browser.shownArtSourceSha256s(ctx, ArtPoster, seriesIDs)
	if err != nil {
		return err
	}
	for index := range rows {
		if rows[index].PosterSha256 == "" && rows[index].Episode != nil {
			rows[index].PosterSha256 = posters[rows[index].Episode.SequenceID]
		}
	}
	return nil
}

// ResumeThresholds are inProgressAfterPercent and finishedAfterPercent: where in a file a position
// counts as partway through it (play-state.md). Continue Watching and a playback's starting point
// both read them here.
type ResumeThresholds struct {
	inProgressAfterPercent, finishedAfterPercent int64
}

// IntSettings reads an integer setting: settings.Service.
type IntSettings interface {
	Int(ctx context.Context, key settings.Key) (int64, error)
}

// ReadResumeThresholds reads the two thresholds from the settings.
func ReadResumeThresholds(ctx context.Context, config IntSettings) (ResumeThresholds, error) {
	minimum, err := config.Int(ctx, settings.InProgressAfterPercent)
	if err != nil {
		return ResumeThresholds{}, fmt.Errorf("read inProgressAfterPercent: %w", err)
	}
	finished, err := config.Int(ctx, settings.FinishedAfterPercent)
	if err != nil {
		return ResumeThresholds{}, fmt.Errorf("read finishedAfterPercent: %w", err)
	}
	return ResumeThresholds{inProgressAfterPercent: minimum, finishedAfterPercent: finished}, nil
}

// IsFinished is whether a position is past the file's finishing threshold: a play that reaches it
// makes the file's title watched (D370 §1). A file of unknown length is never finished by its
// position, only by playing to its end.
func (thresholds ResumeThresholds) IsFinished(positionMs, durationMs int64) bool {
	return durationMs > 0 && positionMs*100 > thresholds.finishedAfterPercent*durationMs
}

// resumesAt is whether Continue Watching resumes a work at a position in one of its files: partway
// through it, and the work not hidden since latestWatchedAt, its latest watching in any of its files
// (D188). A list's film shows how far in it is by the same rule.
func (thresholds ResumeThresholds) resumesAt(
	positionMs, durationMs int64, hiddenAt *int64, latestWatchedAt int64,
) bool {
	if hiddenAt != nil && *hiddenAt >= latestWatchedAt {
		return false
	}
	return thresholds.IsPartwayThrough(positionMs, durationMs)
}

// IsPartwayThrough is whether a position is partway through a file: past the first threshold and not
// past the second, compared as products so no rounding moves the edges. A file of unknown length
// is partway through at any position but its start: a missing fact never becomes "finished".
func (thresholds ResumeThresholds) IsPartwayThrough(positionMs, durationMs int64) bool {
	if durationMs <= 0 {
		return positionMs > 0
	}
	return positionMs*100 >= thresholds.inProgressAfterPercent*durationMs &&
		positionMs*100 <= thresholds.finishedAfterPercent*durationMs
}
