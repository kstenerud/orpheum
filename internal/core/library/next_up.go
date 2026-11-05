// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"context"
	"fmt"
	"slices"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
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
	// IsPartway is whether its person is partway through one of its files, since they last marked it
	// by hand, and IsFinished whether they watched it, by playing or by hand (D370); WatchedAt is when
	// they last watched one, 0 for never, and FinishedAt when it became watched or they last marked it
	// so, 0 while it is not: a rewatch begun since does not move it (D370 §5).
	IsPartway, IsFinished bool
	WatchedAt, FinishedAt int64
	// PartwayFileID is the file they are partway through, the latest watched of several, and
	// PartwayAtMs where in it; 0 when they are partway through none.
	PartwayFileID, PartwayAtMs int64
	partwayWatchedAt           int64
}

// MembersOf are a collection's members, from its files in its ordering, as a person stands in them.
func MembersOf(rows []sqlcgen.ListOrderingMemberFilesWithPositionsRow, thresholds ResumeThresholds) []Member {
	var members []Member
	for _, row := range rows {
		if len(members) == 0 || members[len(members)-1].TitleID != row.WorkID {
			members = append(members, Member{
				TitleID: row.WorkID, Group: row.GroupNumber, NumberInGroup: row.NumberInGroup, Name: row.Name,
				partwayWatchedAt: -1,
			})
		}
		last := &members[len(members)-1]
		last.FileIDs = append(last.FileIDs, row.FileID)
		if row.WatchedAt != nil {
			// Watched, by playing or by hand, whatever its positions say now (D370 §5).
			last.IsFinished = true
			last.FinishedAt = max(*row.WatchedAt, ptr.ValueOrZero(row.MarkedAt))
			last.WatchedAt = max(last.WatchedAt, last.FinishedAt)
		}
		if row.PositionMs == nil {
			continue
		}
		readAt := ptr.ValueOrZero(row.PositionReadAt)
		if IsBeforeMark(readAt, row.MarkedAt) {
			continue // marked by hand since: a fresh start (D370 §4)
		}
		if thresholds.IsPartwayThrough(*row.PositionMs, row.DurationMs) {
			last.IsPartway = true
			if readAt > last.partwayWatchedAt {
				last.PartwayFileID, last.PartwayAtMs, last.partwayWatchedAt = row.FileID, *row.PositionMs, readAt
			}
		}
		last.WatchedAt = max(last.WatchedAt, readAt)
	}
	return members
}

// MembersAhead are the count members from the person's place in the ordering, the first of them its
// Next Up (D160; D324 §3, the owner, 2026-10-21): the one they are partway through, the latest
// watched when several are; else the one after the last they finished, the latest finished, the
// later in the ordering of several finished at once, as a season marked by hand is (D370 §4); else
// the first. None when the last is finished. The specials, group 0, are optional where the ordering
// has groups besides them (the owner, 2026-11-01: a series "should always start at season 1,
// leaving the specials optional"): one is Next Up only while its person is partway through it, a
// finished one does not move their place, and the members ahead pass over them.
func MembersAhead(members []Member, count int) []Member {
	isOptional := func(Member) bool { return false }
	if slices.ContainsFunc(members, func(each Member) bool { return each.Group != 0 }) {
		isOptional = func(each Member) bool { return each.Group == 0 }
	}
	start, partwayAt, finishedAt := 0, int64(-1), int64(-1)
	for index, each := range members {
		switch {
		case each.IsPartway && each.WatchedAt > partwayAt:
			start, partwayAt = index, each.WatchedAt
		case partwayAt < 0 && each.IsFinished && each.FinishedAt >= finishedAt && !isOptional(each):
			start, finishedAt = index+1, each.FinishedAt
		}
	}
	var ahead []Member
	for index := start; index < len(members) && len(ahead) < count; index++ {
		if index == start && partwayAt >= 0 || !isOptional(members[index]) {
			ahead = append(ahead, members[index])
		}
	}
	return ahead // none when every member up to the last is finished
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

// Viewer is a person as a read answers them: who they are, and the libraries they reach.
type Viewer struct {
	UserID int64
	Reach  auth.Reach
}

// ReadMembers are collectionID's members in its ordering as viewer stands in them, by
// in_progress_after_percent and finished_after_percent: each with the files of it a library the viewer
// reaches lists, and none it lists no file of (D363 §8).
func ReadMembers(
	ctx context.Context, queries *sqlcgen.Queries, config IntSettings, viewer Viewer, collectionID int64,
	ordering string,
) ([]Member, error) {
	reach, userID := viewer.Reach, viewer.UserID
	thresholds, err := ReadResumeThresholds(ctx, config)
	if err != nil {
		return nil, err
	}
	isEveryLibrary, libraryIDsJSON := reach.QueryArgs()
	rows, err := queries.ListOrderingMemberFilesWithPositions(ctx, sqlcgen.ListOrderingMemberFilesWithPositionsParams{
		SequenceID: collectionID, UserID: userID, OrderingName: ordering,
		IsEveryLibrary: isEveryLibrary, LibraryIdsJSON: libraryIDsJSON,
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

// NextUp is userID's Next Up in collectionID's ordering, MembersAhead's first, of what reach reaches;
// false when there is none, every member finished or none with a live file.
func (browser *Browser) NextUp(ctx context.Context, reach auth.Reach, userID, collectionID int64, ordering string) (
	NextUp, bool, error,
) {
	members, err := ReadMembers(ctx, browser.database.Read(), browser.settings, Viewer{UserID: userID, Reach: reach},
		collectionID, ordering)
	if err != nil {
		return NextUp{}, false, err
	}
	ahead := MembersAhead(members, 1)
	if len(ahead) == 0 {
		return NextUp{}, false, nil
	}
	next, err := browser.nextUpOf(ctx, reach, collectionID, ordering, ahead[0])
	return next, err == nil, err
}

// nextUpOf is member of collectionID's ordering as its page offers to play it: the file its Play
// plays, the one its person is partway through, else its first; its group's name; and why none of
// its files can be read now.
func (browser *Browser) nextUpOf(
	ctx context.Context, reach auth.Reach, collectionID int64, ordering string, member Member,
) (NextUp, error) {
	names, err := groupNames(ctx, browser.database.Read(), collectionID, ordering)
	if err != nil {
		return NextUp{}, err
	}
	next := NextUp{Member: member, FileID: member.FileIDs[0], GroupName: names[member.Group]}
	if next.IsPartway {
		next.FileID = next.PartwayFileID
	}
	unreadable, err := browser.unreadableItems(ctx, reach, []int64{next.TitleID}, false)
	if err != nil {
		return NextUp{}, err
	}
	if why, isUnreadable := unreadable[next.TitleID]; isUnreadable {
		next.Unreadable = &why
	}
	return next, nil
}

// UpNext is what plays after a file as it ends (plans/up-next.md): the member after the file's
// title in the ordering its person watches its series in (D159 §1, D160), as Next Up offers it; the
// ordering, which labels it; whether that ordering is one group, whose label names no group; and
// the member's still, "" for none.
type UpNext struct {
	NextUp
	Ordering    string
	IsOnlyGroup bool
	StillSha256 string
}

// Neighbours are the members either side of a file's title in its sequence (D407 §2): for now the
// ordering its person watches its series in, whose members are its episodes and specials and never
// its extras, which hold no place in an ordering. The specials are a sequence of their own, apart
// from the episodes (the owner, 2026-11-03), where the ordering has groups besides them. Previous
// is the member before it, as Next Up offers it, nil for the first; Next is what plays after it, nil
// for the last.
type Neighbours struct {
	Previous *NextUp
	Next     *UpNext
}

// sequencePlace is a file's title's place in its sequence: the ordering, the sequence's members, the
// title's index among them, and whether the ordering is one group, whose label names no group.
type sequencePlace struct {
	place       *EpisodePlace
	members     []Member
	at          int
	isOnlyGroup bool
}

// sequenceAround are the members of the sequence the member at index at is in: the specials alone
// for a special and the rest for any other, where the ordering has groups besides the specials
// (the owner, 2026-11-03: "Specials should be their own set of videos, separate from the episodes").
func sequenceAround(members []Member, at int) []Member {
	isSpecial := members[at].Group == 0
	return slices.DeleteFunc(slices.Clone(members), func(each Member) bool { return (each.Group == 0) != isSpecial })
}

// sequencePlaceOf is fileID's title's place in the sequence userID watches it in, of what reach
// reaches; false for a file of no series and one its person's ordering leaves out.
func (browser *Browser) sequencePlaceOf(ctx context.Context, reach auth.Reach, userID, fileID int64) (
	sequencePlace, bool, error,
) {
	work, err := browser.WorkOfFile(ctx, reach, fileID)
	if err != nil || work.TitleID == 0 {
		return sequencePlace{}, false, err
	}
	place, err := browser.EpisodePlaceOf(ctx, userID, work.TitleID)
	if err != nil || place == nil {
		return sequencePlace{}, false, err
	}
	members, err := ReadMembers(ctx, browser.database.Read(), browser.settings, Viewer{UserID: userID, Reach: reach},
		place.SequenceID, place.Ordering)
	if err != nil {
		return sequencePlace{}, false, err
	}
	inOrdering := slices.IndexFunc(members, func(each Member) bool { return each.TitleID == work.TitleID })
	if inOrdering < 0 {
		return sequencePlace{}, false, nil
	}
	groups := map[int64]bool{}
	for _, each := range members {
		groups[each.Group] = true
	}
	sequence := sequenceAround(members, inOrdering)
	return sequencePlace{
		place: place, members: sequence, isOnlyGroup: len(groups) == 1,
		at: slices.IndexFunc(sequence, func(each Member) bool { return each.TitleID == work.TitleID }),
	}, true, nil
}

// UpNext is what plays after fileID for userID, of what reach reaches; false for a file of no series,
// one its person's ordering leaves out, and its last. ErrNoFile for a file there is not.
func (browser *Browser) UpNext(ctx context.Context, reach auth.Reach, userID, fileID int64) (UpNext, bool, error) {
	sequence, isInOne, err := browser.sequencePlaceOf(ctx, reach, userID, fileID)
	if err != nil || !isInOne {
		return UpNext{}, false, err
	}
	return browser.upNextIn(ctx, reach, sequence)
}

// NeighboursOf are the members either side of fileID's title in the sequence userID watches it in,
// of what reach reaches: neither for a file of no series and one its person's ordering leaves out.
// ErrNoFile for a file there is not.
func (browser *Browser) NeighboursOf(ctx context.Context, reach auth.Reach, userID, fileID int64) (
	Neighbours, error,
) {
	sequence, isInOne, err := browser.sequencePlaceOf(ctx, reach, userID, fileID)
	if err != nil || !isInOne {
		return Neighbours{}, err
	}
	var neighbours Neighbours
	if sequence.at > 0 {
		previous, previousErr := browser.nextUpOf(ctx, reach, sequence.place.SequenceID, sequence.place.Ordering,
			sequence.members[sequence.at-1])
		if previousErr != nil {
			return Neighbours{}, previousErr
		}
		neighbours.Previous = &previous
	}
	next, hasNext, err := browser.upNextIn(ctx, reach, sequence)
	if err != nil {
		return Neighbours{}, err
	}
	if hasNext {
		neighbours.Next = &next
	}
	return neighbours, nil
}

// upNextIn is the member after sequence's title, as UpNext offers it; false for its last.
func (browser *Browser) upNextIn(ctx context.Context, reach auth.Reach, sequence sequencePlace) (
	UpNext, bool, error,
) {
	place, members, at := sequence.place, sequence.members, sequence.at
	if at+1 >= len(members) {
		return UpNext{}, false, nil
	}
	next, err := browser.nextUpOf(ctx, reach, place.SequenceID, place.Ordering, members[at+1])
	if err != nil {
		return UpNext{}, false, err
	}
	stills, err := browser.shownArtSourceSha256s(ctx, ArtStill, []int64{next.TitleID})
	if err != nil {
		return UpNext{}, false, err
	}
	return UpNext{
		NextUp: next, Ordering: place.Ordering, IsOnlyGroup: sequence.isOnlyGroup, StillSha256: stills[next.TitleID],
	}, true, nil
}
