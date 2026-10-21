// SPDX-License-Identifier: AGPL-3.0-or-later

package interest

import (
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// The episodes a look at a series raises start at its person's place (the owner, 2026-10-21): the
// episode partway through, the latest watched of several; else the one after the latest finished;
// else the first. None when the last is finished.
func TestEpisodesAhead_StartAtThePersonsPlaceInTheSeries(t *testing.T) {
	t.Parallel()
	ids := func(episodes []episode) []int64 {
		var titles []int64
		for _, each := range episodes {
			titles = append(titles, each.titleID)
		}
		return titles
	}
	series := func(changes map[int64]episode) []episode {
		var all []episode
		for title := int64(1); title <= 6; title++ {
			each := changes[title]
			each.titleID = title
			all = append(all, each)
		}
		return all
	}
	for _, each := range []struct {
		name string
		at   map[int64]episode
		want []int64
	}{
		{"nothing watched: the first", nil, []int64{1, 2, 3}},
		{"two finished: after the latest finished", map[int64]episode{
			1: {isFinished: true, watchedAt: 10}, 2: {isFinished: true, watchedAt: 20},
		}, []int64{3, 4, 5}},
		{"finished out of order: after the latest in time", map[int64]episode{
			4: {isFinished: true, watchedAt: 10}, 2: {isFinished: true, watchedAt: 20},
		}, []int64{3, 4, 5}},
		{"partway: it, though a later one is finished", map[int64]episode{
			2: {isPartway: true, watchedAt: 5}, 4: {isFinished: true, watchedAt: 30},
		}, []int64{2, 3, 4}},
		{"two partway: the latest watched", map[int64]episode{
			2: {isPartway: true, watchedAt: 50}, 5: {isPartway: true, watchedAt: 10},
		}, []int64{2, 3, 4}},
		{"near the end: what is left", map[int64]episode{5: {isFinished: true, watchedAt: 1}}, []int64{6}},
		{"the last finished: none", map[int64]episode{6: {isFinished: true, watchedAt: 1}}, nil},
	} {
		if got := ids(episodesAhead(series(each.at), 3)); !slices.Equal(got, each.want) {
			t.Errorf("%s: episodes %v; want %v", each.name, got, each.want)
		}
	}
}

// A series' files, in aired order, make its episodes, each partway or finished by its person's
// position against the in-progress and finished settings, and watched when they last watched it.
func TestEpisodesOf_ReadThePersonsPositionsAgainstTheThresholds(t *testing.T) {
	t.Parallel()
	thresholds, err := library.ReadResumeThresholds(t.Context(),
		settingstest.Seeded(t, storetest.OpenMigrated(t)))
	if err != nil {
		t.Fatal(err)
	}
	position := func(ms int64) *int64 { return &ms }
	rows := []sqlcgen.ListSeriesEpisodeFilesWithPositionsRow{
		{TitleID: 1, FileID: 10, DurationMs: 1000, PositionMs: position(950), PositionReadAt: position(7)},
		{TitleID: 2, FileID: 20, DurationMs: 1000, PositionMs: position(500), PositionReadAt: position(9)},
		{TitleID: 2, FileID: 21, DurationMs: 1000},
		{TitleID: 3, FileID: 30, DurationMs: 1000, PositionMs: position(10), PositionReadAt: position(3)},
	}
	got := episodesOf(rows, thresholds)
	want := []episode{
		{titleID: 1, fileIDs: []int64{10}, isFinished: true, watchedAt: 7},
		{titleID: 2, fileIDs: []int64{20, 21}, isPartway: true, watchedAt: 9},
		{titleID: 3, fileIDs: []int64{30}, watchedAt: 3},
	}
	if !slices.EqualFunc(got, want, func(first, second episode) bool {
		return first.titleID == second.titleID && slices.Equal(first.fileIDs, second.fileIDs) &&
			first.isPartway == second.isPartway && first.isFinished == second.isFinished &&
			first.watchedAt == second.watchedAt
	}) {
		t.Errorf("episodes %+v; want %+v", got, want)
	}
}
