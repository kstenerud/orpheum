// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// The members ahead in an ordering, the first its Next Up, start at its person's place (D160; the
// owner, 2026-10-21): the member partway through, the latest watched of several; else the one after
// the latest finished; else the first. None when the last is finished.
func TestMembersAhead_StartAtThePersonsPlaceInTheOrdering(t *testing.T) {
	t.Parallel()
	ids := func(members []Member) []int64 {
		var titles []int64
		for _, each := range members {
			titles = append(titles, each.TitleID)
		}
		return titles
	}
	series := func(changes map[int64]Member) []Member {
		var all []Member
		for title := int64(1); title <= 6; title++ {
			each := changes[title]
			each.TitleID = title
			all = append(all, each)
		}
		return all
	}
	for _, each := range []struct {
		name string
		at   map[int64]Member
		want []int64
	}{
		{"nothing watched: the first", nil, []int64{1, 2, 3}},
		{"two finished: after the latest finished", map[int64]Member{
			1: {IsFinished: true, FinishedAt: 10}, 2: {IsFinished: true, FinishedAt: 20},
		}, []int64{3, 4, 5}},
		{"finished out of order: after the latest in time", map[int64]Member{
			4: {IsFinished: true, FinishedAt: 10}, 2: {IsFinished: true, FinishedAt: 20},
		}, []int64{3, 4, 5}},
		{"a finished one begun again, short of partway: after the latest finished", map[int64]Member{
			1: {IsFinished: true, FinishedAt: 10, WatchedAt: 30}, 2: {IsFinished: true, FinishedAt: 20},
		}, []int64{3, 4, 5}},
		{"several finished at once, as a season marked: after the last of them", map[int64]Member{
			1: {IsFinished: true, FinishedAt: 10}, 2: {IsFinished: true, FinishedAt: 10},
			3: {IsFinished: true, FinishedAt: 10},
		}, []int64{4, 5, 6}},
		{"partway: it, though a later one is finished", map[int64]Member{
			2: {IsPartway: true, WatchedAt: 5}, 4: {IsFinished: true, FinishedAt: 30},
		}, []int64{2, 3, 4}},
		{"two partway: the latest watched", map[int64]Member{
			2: {IsPartway: true, WatchedAt: 50}, 5: {IsPartway: true, WatchedAt: 10},
		}, []int64{2, 3, 4}},
		{"near the end: what is left", map[int64]Member{5: {IsFinished: true, FinishedAt: 1}}, []int64{6}},
		{"the last finished: none", map[int64]Member{6: {IsFinished: true, FinishedAt: 1}}, nil},
	} {
		if got := ids(MembersAhead(series(each.at), 3)); !slices.Equal(got, each.want) {
			t.Errorf("%s: members %v; want %v", each.name, got, each.want)
		}
	}
}

// The members a look at a series' season raises are that group's, from its person's place in it
// alone, the same way; else, when its last is finished, from its first (the owner, 2026-10-24).
// A series' specials are optional: one nobody has started begins at season 1, a finished special
// does not move the person's place, and one they are partway through is still theirs to finish (the
// owner, 2026-11-01: The Freshman said "Play S0E0").
func TestMembersAhead_TheSpecialsAreOptional(t *testing.T) {
	t.Parallel()
	ids := func(members []Member) []int64 {
		var titles []int64
		for _, each := range members {
			titles = append(titles, each.TitleID)
		}
		return titles
	}
	// Titles 1 and 2 are specials (season 0), 3 to 5 season 1, 6 a special after it.
	series := func(changes map[int64]Member) []Member {
		groups := map[int64]int64{1: 0, 2: 0, 3: 1, 4: 1, 5: 1, 6: 0}
		var all []Member
		for title := int64(1); title <= 6; title++ {
			each := changes[title]
			each.TitleID, each.Group = title, groups[title]
			all = append(all, each)
		}
		return all
	}
	for _, each := range []struct {
		name string
		at   map[int64]Member
		want []int64
	}{
		{"nothing watched: season 1's first", nil, []int64{3, 4, 5}},
		{"a special finished: still season 1's first", map[int64]Member{
			1: {IsFinished: true, FinishedAt: 10},
		}, []int64{3, 4, 5}},
		{"partway through a special: that special, then season 1", map[int64]Member{
			2: {IsPartway: true, WatchedAt: 10},
		}, []int64{2, 3, 4}},
		{"season 1 finished and a special after it: none", map[int64]Member{
			3: {IsFinished: true, FinishedAt: 10}, 4: {IsFinished: true, FinishedAt: 20},
			5: {IsFinished: true, FinishedAt: 30},
		}, nil},
	} {
		if got := ids(MembersAhead(series(each.at), 3)); !slices.Equal(got, each.want) {
			t.Errorf("%s: MembersAhead = %v, want %v", each.name, got, each.want)
		}
	}
	onlySpecials := []Member{{TitleID: 1}, {TitleID: 2}}
	if got := ids(MembersAhead(onlySpecials, 2)); !slices.Equal(got, []int64{1, 2}) {
		t.Errorf("an ordering of group 0 alone: MembersAhead = %v, want [1 2]", got)
	}
}

func TestMembersAheadInGroup_StartAtThePersonsPlaceInTheGroup(t *testing.T) {
	t.Parallel()
	ids := func(members []Member) []int64 {
		var titles []int64
		for _, each := range members {
			titles = append(titles, each.TitleID)
		}
		return titles
	}
	// Members 1 and 2 are season 1's, 3 to 6 season 2's.
	series := func(changes map[int64]Member) []Member {
		var all []Member
		for title := int64(1); title <= 6; title++ {
			each := changes[title]
			each.TitleID, each.Group = title, 1+min(1, (title-1)/2)
			all = append(all, each)
		}
		return all
	}
	for _, each := range []struct {
		name  string
		group int64
		at    map[int64]Member
		want  []int64
	}{
		{"nothing watched: the season's first", 2, nil, []int64{3, 4, 5}},
		{"the season before finished: the season's first", 2, map[int64]Member{
			2: {IsFinished: true, FinishedAt: 10},
		}, []int64{3, 4, 5}},
		{"partway in it: that episode", 2, map[int64]Member{4: {IsPartway: true, WatchedAt: 10}}, []int64{4, 5, 6}},
		{"its place, though later partway in another", 2, map[int64]Member{
			3: {IsFinished: true, FinishedAt: 10}, 1: {IsPartway: true, WatchedAt: 20},
		}, []int64{4, 5, 6}},
		{"its last finished: its first again", 2, map[int64]Member{
			6: {IsFinished: true, FinishedAt: 1},
		}, []int64{3, 4, 5}},
		{"a short season: its episodes alone", 1, map[int64]Member{
			2: {IsFinished: true, FinishedAt: 1},
		}, []int64{1, 2}},
		{"a season with no episodes: none", 3, nil, nil},
	} {
		if got := ids(MembersAheadInGroup(series(each.at), each.group, 3)); !slices.Equal(got, each.want) {
			t.Errorf("%s: members %v; want %v", each.name, got, each.want)
		}
	}
}

// A collection's files, in its ordering, make its members, each in its group with its number and
// name, each partway by its person's position against the in-progress and finished settings, finished
// when they watched it, by playing or by hand, whatever their position says now (D370 §5), watched when
// they last watched it, and partway through the file they watched latest of those they are partway
// through, at its position; a position from before their last mark by hand counts for nothing (§4).
func TestMembersOf_ReadThePersonsPositionsAgainstTheThresholds(t *testing.T) {
	t.Parallel()
	thresholds, err := ReadResumeThresholds(t.Context(), settingstest.Seeded(t, storetest.OpenMigrated(t)))
	if err != nil {
		t.Fatal(err)
	}
	position := func(ms int64) *int64 { return &ms }
	rows := []sqlcgen.ListOrderingMemberFilesWithPositionsRow{
		{
			TitleID: 1, GroupNumber: 1, NumberInGroup: 1, Name: "Pilot", FileID: 10, DurationMs: 1000,
			PositionMs: position(950), PositionReadAt: position(7), WatchedAt: position(7),
		},
		{
			TitleID: 2, GroupNumber: 1, NumberInGroup: 2, FileID: 20, DurationMs: 1000, PositionMs: position(500),
			PositionReadAt: position(9),
		},
		{
			TitleID: 2, GroupNumber: 1, NumberInGroup: 2, FileID: 21, DurationMs: 1000, PositionMs: position(400),
			PositionReadAt: position(11),
		},
		{TitleID: 2, GroupNumber: 1, NumberInGroup: 2, FileID: 22, DurationMs: 1000},
		{
			TitleID: 3, GroupNumber: 2, NumberInGroup: 1, FileID: 30, DurationMs: 1000, PositionMs: position(10),
			PositionReadAt: position(3),
		},
		// Past the threshold, but never watched to it: an unattended play, so not finished.
		{
			TitleID: 4, GroupNumber: 2, NumberInGroup: 2, FileID: 40, DurationMs: 1000, PositionMs: position(950),
			PositionReadAt: position(4),
		},
		// Marked watched by hand at 8, partway at 6 before it: finished, and partway through nothing.
		{
			TitleID: 5, GroupNumber: 2, NumberInGroup: 3, FileID: 50, DurationMs: 1000, PositionMs: position(500),
			PositionReadAt: position(6), WatchedAt: position(8), MarkedAt: position(8),
		},
		// Watched at 2, begun again at 12 and a third in: finished, partway in its rewatch.
		{
			TitleID: 6, GroupNumber: 2, NumberInGroup: 4, FileID: 60, DurationMs: 1000, PositionMs: position(300),
			PositionReadAt: position(12), WatchedAt: position(2),
		},
	}
	got := MembersOf(rows, thresholds)
	want := []Member{
		{
			TitleID: 1, Group: 1, NumberInGroup: 1, Name: "Pilot", FileIDs: []int64{10}, IsFinished: true, WatchedAt: 7,
			FinishedAt: 7,
		},
		{
			TitleID: 2, Group: 1, NumberInGroup: 2, FileIDs: []int64{20, 21, 22}, IsPartway: true, WatchedAt: 11,
			PartwayFileID: 21, PartwayAtMs: 400,
		},
		{TitleID: 3, Group: 2, NumberInGroup: 1, FileIDs: []int64{30}, WatchedAt: 3},
		{TitleID: 4, Group: 2, NumberInGroup: 2, FileIDs: []int64{40}, WatchedAt: 4},
		{TitleID: 5, Group: 2, NumberInGroup: 3, FileIDs: []int64{50}, IsFinished: true, WatchedAt: 8, FinishedAt: 8},
		{
			TitleID: 6, Group: 2, NumberInGroup: 4, FileIDs: []int64{60}, IsPartway: true, IsFinished: true,
			WatchedAt: 12, FinishedAt: 2, PartwayFileID: 60, PartwayAtMs: 300,
		},
	}
	if !slices.EqualFunc(got, want, func(first, second Member) bool {
		return first.TitleID == second.TitleID && first.Group == second.Group &&
			first.NumberInGroup == second.NumberInGroup && first.Name == second.Name &&
			slices.Equal(first.FileIDs, second.FileIDs) && first.IsPartway == second.IsPartway &&
			first.IsFinished == second.IsFinished && first.WatchedAt == second.WatchedAt &&
			first.FinishedAt == second.FinishedAt &&
			first.PartwayFileID == second.PartwayFileID && first.PartwayAtMs == second.PartwayAtMs
	}) {
		t.Errorf("members %+v; want %+v", got, want)
	}
}
