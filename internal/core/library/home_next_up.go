// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"cmp"
	"context"
	"fmt"
	"slices"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/settings"
)

// NextUpRow is Home's Next Up (play-state.md "Continue Watching and Next Up", plans/b1.md §A): for
// each series the person finished an episode of and is partway through none of, by the thresholds
// Continue Watching reads, so that a series is never in both rows, its Next Up in the ordering they
// watch it in (MembersAhead's first, as its page offers it), each row the episode and the file its
// Play plays, from its start. Most recent watching in the series first, at most
// continueWatchingMaxWorks of them, of what reach reaches; with no age limit, and series only (a
// franchise's films wait for collections, D160).
//
// A hide attaches to the episode the row offers, as Continue Watching's attaches to its work: the one
// per-person item_hidden of D188, written by PUT /items/{id}/hidden. It lifts at the series' next
// activity, not the episode's alone: the row is left out while its episode was hidden at or after the
// person's latest watching of any episode of the series. Finishing the episode, or any other, moves
// that, and a row offering a different episode is not hidden.
//
// It reads the candidate series in one statement, their orderings in one and their members in one,
// whatever their number; then each row's place and pictures as Continue Watching reads its own.
func (browser *Browser) NextUpRow(ctx context.Context, userID int64, reach auth.Reach) ([]InProgress, error) {
	maxWorks, err := browser.settings.Int(ctx, settings.ContinueWatchingMaxWorks)
	if err != nil {
		return nil, fmt.Errorf("read continueWatchingMaxWorks: %w", err)
	}
	queries := browser.database.Read()
	seriesIDs, err := queries.ListSeriesWithEpisodesAhead(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list the series user %d has episodes ahead in: %w", userID, err)
	}
	if len(seriesIDs) == 0 {
		return []InProgress{}, nil
	}
	orderings, err := ViewingOrderings(ctx, queries, userID, seriesIDs)
	if err != nil {
		return nil, err
	}
	membersBySeries, err := readMembersOf(ctx, queries, browser.settings, Viewer{UserID: userID, Reach: reach},
		seriesIDs, orderings)
	if err != nil {
		return nil, err
	}
	type offer struct {
		next            Member
		latestWatchedAt int64
	}
	var offers []offer
	for _, id := range seriesIDs {
		if next, latestWatchedAt, isOffered := nextUpOnHome(membersBySeries[id]); isOffered {
			offers = append(offers, offer{next: next, latestWatchedAt: latestWatchedAt})
		}
	}
	slices.SortStableFunc(offers, func(first, second offer) int {
		return cmp.Compare(second.latestWatchedAt, first.latestWatchedAt)
	})
	rows := make([]InProgress, 0, min(int64(len(offers)), maxWorks))
	for _, each := range offers[:min(int64(len(offers)), maxWorks)] {
		row := InProgress{
			Entry:  Entry{ID: each.next.TitleID, Kind: TitleKindEpisode, Name: each.next.Name},
			FileID: each.next.FileToPlay(),
		}
		if row.Episode, err = browser.EpisodePlaceOf(ctx, userID, row.ID); err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	return browser.withPostersAndFileNames(ctx, reach, userID, rows, nil)
}

// nextUpOnHome is a series' row of Home's Next Up, from its members as its person stands in them:
// its Next Up, and their latest watching of any of its members; false when they are partway through
// one (Continue Watching's), finished none, finished its last, or hid the one it would offer since
// they last watched any.
func nextUpOnHome(members []Member) (Member, int64, bool) {
	latestWatchedAt, hasFinished := int64(0), false
	for _, each := range members {
		if each.IsPartway {
			return Member{}, 0, false
		}
		hasFinished = hasFinished || each.IsFinished
		latestWatchedAt = max(latestWatchedAt, each.WatchedAt)
	}
	ahead := MembersAhead(members, 1)
	if !hasFinished || len(ahead) == 0 {
		return Member{}, 0, false
	}
	if hiddenAt := ahead[0].HiddenAt; hiddenAt > 0 && hiddenAt >= latestWatchedAt {
		return Member{}, 0, false
	}
	return ahead[0], latestWatchedAt, true
}
