// SPDX-License-Identifier: AGPL-3.0-or-later

package playback

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// WatchedState is a title's watched mark as it stood for a person (D370): when it became watched, nil
// while it is not, and when they last marked it by hand, nil for never. Both nil is no mark at all.
type WatchedState struct {
	TitleID   int64
	WatchedAt *int64
	MarkedAt  *int64
}

// ErrBadWatchedState is an undo that cannot be true: a time to come.
var ErrBadWatchedState = errors.New("a watched mark cannot be put back as it is given")

// MarkWatched marks titleIDs watched, or unwatched, by hand for userID (D370 §4), and returns each
// one's mark as it stood before, which RestoreWatched puts back: an undo, not a question. Marked
// watched, a title is watched from now unless it already was; marked unwatched, it is as never
// watched; either way the positions in its files watched before now are no longer where it resumes.
// The caller says which titles the person may mark (library.TitleToMark, library.EpisodesToMark).
func (service *Service) MarkWatched(
	ctx context.Context, userID int64, titleIDs []int64, isWatched bool,
) ([]WatchedState, error) {
	encoded, err := json.Marshal(titleIDs)
	if err != nil {
		return nil, fmt.Errorf("encode %d title ids: %w", len(titleIDs), err)
	}
	now := service.clock.Now().UnixMilli()
	var before []WatchedState
	err = service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		rows, readErr := queries.ListTitlesWatched(ctx, sqlcgen.ListTitlesWatchedParams{
			TitleIdsJSON: string(encoded), UserID: userID,
		})
		if readErr != nil {
			return readErr
		}
		before = statesBefore(titleIDs, rows)
		if isWatched {
			return queries.MarkTitlesWatched(ctx, sqlcgen.MarkTitlesWatchedParams{
				UserID: userID, MarkedAt: now, TitleIdsJSON: string(encoded),
			})
		}
		return queries.MarkTitlesUnwatched(ctx, sqlcgen.MarkTitlesUnwatchedParams{
			UserID: userID, MarkedAt: now, TitleIdsJSON: string(encoded),
		})
	})
	if err != nil {
		return nil, fmt.Errorf("mark %d titles watched (%t) for user %d: %w", len(titleIDs), isWatched, userID, err)
	}
	return before, nil
}

// statesBefore are each title's state as rows read it, one with no row none.
func statesBefore(titleIDs []int64, rows []sqlcgen.ListTitlesWatchedRow) []WatchedState {
	byID := map[int64]sqlcgen.ListTitlesWatchedRow{}
	for _, row := range rows {
		byID[row.TitleID] = row
	}
	states := make([]WatchedState, 0, len(titleIDs))
	for _, id := range titleIDs {
		row := byID[id]
		states = append(states, WatchedState{TitleID: id, WatchedAt: row.WatchedAt, MarkedAt: row.MarkedAt})
	}
	return states
}

// RestoreWatched puts userID's marks of titles back as states give them: MarkWatched's undo. A state
// with neither time forgets the title's mark; a time to come is ErrBadWatchedState. It changes only
// the person's own marks, so it asks nothing of the titles but that they are titles one marks.
func (service *Service) RestoreWatched(ctx context.Context, userID int64, states []WatchedState) error {
	now := service.clock.Now().UnixMilli()
	var kept [][3]any
	var forgotten []int64
	for _, state := range states {
		for _, at := range []*int64{state.WatchedAt, state.MarkedAt} {
			if at != nil && *at > now {
				return fmt.Errorf("%w: title %d at %d, after now", ErrBadWatchedState, state.TitleID, *at)
			}
		}
		if state.WatchedAt == nil && state.MarkedAt == nil {
			forgotten = append(forgotten, state.TitleID)
			continue
		}
		kept = append(kept, [3]any{state.TitleID, state.WatchedAt, state.MarkedAt})
	}
	keptJSON, err := json.Marshal(kept)
	if err != nil {
		return fmt.Errorf("encode %d marks: %w", len(kept), err)
	}
	forgottenJSON, err := json.Marshal(forgotten)
	if err != nil {
		return fmt.Errorf("encode %d title ids: %w", len(forgotten), err)
	}
	err = service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		if len(kept) > 0 {
			restoreErr := queries.RestoreTitlesWatched(ctx, sqlcgen.RestoreTitlesWatchedParams{
				UserID: userID, StatesJSON: string(keptJSON),
			})
			if restoreErr != nil {
				return restoreErr
			}
		}
		if len(forgotten) == 0 {
			return nil
		}
		return queries.DeleteTitlesWatched(ctx, sqlcgen.DeleteTitlesWatchedParams{
			UserID: userID, TitleIdsJSON: string(forgottenJSON),
		})
	})
	if err != nil {
		return fmt.Errorf("put back user %d's marks of %d titles: %w", userID, len(states), err)
	}
	return nil
}

// keepWatchedByPlay makes the titles a played file is filed as watched by its person, at watchedAt,
// when the report played it to its finish: to its end, or past finished_after_percent of a known
// length (D370 §1). A report of unattended watching makes nothing watched; nor does a position in a
// file of unknown length, which only its end does.
func (service *Service) keepWatchedByPlay(
	ctx context.Context, queries *sqlcgen.Queries, userID, fileID, watchedAt int64, report Report,
) error {
	for _, span := range report.Spans {
		if span.IsUnattended {
			return nil
		}
	}
	isFinished := false
	for _, event := range report.Events {
		isFinished = isFinished || event.Kind == EventCompleted
	}
	if !isFinished {
		durationMs, err := store.OrNotFound(queries.GetFileDuration(ctx, fileID))
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("read file %d's length: %w", fileID, err)
		}
		thresholds, err := library.ReadResumeThresholds(ctx, service.settings)
		if err != nil {
			return err
		}
		isFinished = thresholds.IsFinished(report.PositionMs, durationMs)
	}
	if !isFinished {
		return nil
	}
	err := queries.MarkWatchedByPlay(ctx, sqlcgen.MarkWatchedByPlayParams{
		UserID: userID, WatchedAt: watchedAt, FileID: fileID,
	})
	if err != nil {
		return fmt.Errorf("mark file %d's titles watched for user %d: %w", fileID, userID, err)
	}
	return nil
}
