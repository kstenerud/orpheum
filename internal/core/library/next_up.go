// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"context"
	"fmt"

	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// OrderingAired is the aired ordering, TMDB's season listing, which every episode of a series is in
// and a person watches it in until they choose another (D159: a collection declares its named
// orderings; the choice is a preference, ViewingOrdering).
const OrderingAired = "aired"

// Member is one member of a collection's ordering, in its place (D160: a sequence is what a
// collection produces, and nothing asks whether its members are episodes or films): its title, its
// group and number in it (a series' season and episode), its name, its live files as its page lists
// them, and where its person is in it.
type Member struct {
	TitleID int64
	// Group is the group it is in, a series' season: 0 for the specials and those with none.
	Group         int64
	NumberInGroup int64
	Name          string
	FileIDs       []int64
	// IsPartway is whether its person is partway through one of its files, and IsFinished whether
	// they finished one; WatchedAt is when they last watched one, 0 for never.
	IsPartway, IsFinished bool
	WatchedAt             int64
	// PartwayFileID is the file they are partway through, the latest watched of several, and
	// PartwayAtMs where in it; 0 when they are partway through none.
	PartwayFileID, PartwayAtMs int64
	partwayWatchedAt           int64
}

// MembersOf are a collection's members, from its files in its ordering, as a person stands in them.
func MembersOf(rows []sqlcgen.ListOrderingMemberFilesWithPositionsRow, thresholds ResumeThresholds) []Member {
	var members []Member
	for _, row := range rows {
		if len(members) == 0 || members[len(members)-1].TitleID != row.TitleID {
			members = append(members, Member{
				TitleID: row.TitleID, Group: row.GroupNumber, NumberInGroup: row.NumberInGroup, Name: row.Name,
				partwayWatchedAt: -1,
			})
		}
		last := &members[len(members)-1]
		last.FileIDs = append(last.FileIDs, row.FileID)
		if row.PositionMs == nil {
			continue
		}
		readAt := int64(0)
		if row.PositionReadAt != nil {
			readAt = *row.PositionReadAt
		}
		if thresholds.IsPartwayThrough(*row.PositionMs, row.DurationMs) {
			last.IsPartway = true
			if readAt > last.partwayWatchedAt {
				last.PartwayFileID, last.PartwayAtMs, last.partwayWatchedAt = row.FileID, *row.PositionMs, readAt
			}
		}
		last.IsFinished = last.IsFinished || thresholds.IsFinished(*row.PositionMs, row.DurationMs)
		last.WatchedAt = max(last.WatchedAt, readAt)
	}
	return members
}

// MembersAhead are the count members from the person's place in the ordering, the first of them its
// Next Up (D160; D324 §3, the owner, 2026-10-21): the one they are partway through, the latest
// watched when several are; else the one after the last they finished, the latest finished; else
// the first. None when the last is finished.
func MembersAhead(members []Member, count int) []Member {
	start, partwayAt, finishedAt := 0, int64(-1), int64(-1)
	for index, each := range members {
		switch {
		case each.IsPartway && each.WatchedAt > partwayAt:
			start, partwayAt = index, each.WatchedAt
		case partwayAt < 0 && each.IsFinished && each.WatchedAt > finishedAt:
			start, finishedAt = index+1, each.WatchedAt
		}
	}
	if start >= len(members) {
		return nil // every member up to the last is finished
	}
	return members[start:min(len(members), start+count)]
}

// MembersOfGroup are the members of one group, a series' season, in their order.
func MembersOfGroup(members []Member, group int64) []Member {
	var inGroup []Member
	for _, each := range members {
		if each.Group == group {
			inGroup = append(inGroup, each)
		}
	}
	return inGroup
}

// MembersAheadInGroup are the count members of one group, a series' season, from the person's place
// in it (the owner, 2026-10-24: interest follows the page, and a season's page is that season's):
// their place among its members alone, as MembersAhead finds it; else, when they finished its last,
// its first.
func MembersAheadInGroup(members []Member, group int64, count int) []Member {
	inGroup := MembersOfGroup(members, group)
	if ahead := MembersAhead(inGroup, count); len(ahead) > 0 {
		return ahead
	}
	return inGroup[:min(len(inGroup), count)]
}

// ReadMembers are collectionID's members in its ordering as userID stands in them, by
// in_progress_after_percent and finished_after_percent.
func ReadMembers(
	ctx context.Context, queries *sqlcgen.Queries, config IntSettings, collectionID, userID int64, ordering string,
) ([]Member, error) {
	thresholds, err := ReadResumeThresholds(ctx, config)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListOrderingMemberFilesWithPositions(ctx, sqlcgen.ListOrderingMemberFilesWithPositionsParams{
		CollectionID: collectionID, UserID: userID, OrderingName: ordering,
	})
	if err != nil {
		return nil, fmt.Errorf("list collection %d's members in its %s ordering: %w", collectionID, ordering, err)
	}
	return MembersOf(rows, thresholds), nil
}

// NextUp is a person's next member of a collection's ordering (D160), what its page offers to play:
// the member, the file its Play plays, the one they are partway through, else its first, and why
// none of its files can be read now, nil when one can (D241 §3), as its own page judges it.
type NextUp struct {
	Member
	FileID int64
	// GroupName is the ordering's name for its group ("Disc 2"), "" where it names none.
	GroupName  string
	Unreadable *Unreadable
}

// NextUp is userID's Next Up in collectionID's ordering, MembersAhead's first; false when there is
// none, every member finished or none with a live file.
func (browser *Browser) NextUp(ctx context.Context, userID, collectionID int64, ordering string) (
	NextUp, bool, error,
) {
	members, err := ReadMembers(ctx, browser.database.Read(), browser.settings, collectionID, userID, ordering)
	if err != nil {
		return NextUp{}, false, err
	}
	ahead := MembersAhead(members, 1)
	if len(ahead) == 0 {
		return NextUp{}, false, nil
	}
	names, err := groupNames(ctx, browser.database.Read(), collectionID, ordering)
	if err != nil {
		return NextUp{}, false, err
	}
	next := NextUp{Member: ahead[0], FileID: ahead[0].FileIDs[0], GroupName: names[ahead[0].Group]}
	if next.IsPartway {
		next.FileID = next.PartwayFileID
	}
	unreadable, err := browser.unreadableItems(ctx, []int64{next.TitleID}, false)
	if err != nil {
		return NextUp{}, false, err
	}
	if why, isUnreadable := unreadable[next.TitleID]; isUnreadable {
		next.Unreadable = &why
	}
	return next, true, nil
}
