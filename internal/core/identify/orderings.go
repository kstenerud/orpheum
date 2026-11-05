// SPDX-License-Identifier: AGPL-3.0-or-later

package identify

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/kstenerud/orpheum/internal/core/preferences"
	"github.com/kstenerud/orpheum/internal/platform/bounded"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// A series' orderings beside aired are its provider's episode groups (episode-orderings.md §2.1,
// §3): the list of them is stored as one part of the series' response, from its own record where
// that carries it, and each group's contents as another, fetched only once a person chose it
// (D170). Each group is an ordering named "<provider>:<group id>", and a pass of the series' library
// keeps placing its episodes in it, from what is stored, as they come and go.

// groupListPart is where a series' list of episode groups is stored in provider_response.
const groupListPart = "episode_groups"

// groupPartPrefix begins the part a group's contents are stored at: groupPart.
const groupPartPrefix = "episode_group/"

// groupPart is where one episode group's contents are stored in provider_response (§3.4).
func groupPart(groupID string) string { return groupPartPrefix + groupID }

// groupOfPart is the group a stored part names, and whether it names one.
func groupOfPart(part string) (string, bool) {
	groupID, isGroup := strings.CutPrefix(part, groupPartPrefix)
	return groupID, isGroup && groupID != ""
}

// groupOrderingName is the name of the ordering an episode group is, the key a person's choice of it
// keeps (§3.1): never its label, which is display text.
func groupOrderingName(provider, groupID string) string { return provider + ":" + groupID }

// groupNameLanguage is the language a group's names are kept in: the provider's answer's, which is
// asked for in none, as a series' own names are kept.
const groupNameLanguage = "und"

// deriveOrderings keeps a series' orderings from its provider as what is stored says: one for each
// group its stored list names, and, for each group fetched, its episodes placed as the group places
// them and its parts' names. It compares before it writes, so a series as stored writes nothing
// (D303, DL9): run it through store.WriteUnlessUnchanged. A group the list no longer names is kept
// (D213 §4); an episode a group leaves out has no place in it (D96).
func deriveOrderings(ctx context.Context, queries *sqlcgen.Queries, provider Provider, seriesID int64) error {
	if err := deriveListedSeasons(ctx, queries, provider, seriesID); err != nil {
		return err
	}
	if err := deriveGroupList(ctx, queries, provider, seriesID); err != nil {
		return err
	}
	orderings, err := queries.ListProviderOrderingsOfSeries(ctx, sqlcgen.ListProviderOrderingsOfSeriesParams{
		SequenceID: seriesID, Provider: new(provider.Name()),
	})
	if err != nil {
		return fmt.Errorf("list series %d's %s orderings: %w", seriesID, provider.Name(), err)
	}
	var titlesByEpisode map[string][]int64
	for _, ordering := range orderings {
		parts, isFetched, err := storedGroup(ctx, queries, provider, seriesID, ptr.ValueOrZero(ordering.SourceID))
		if err != nil || !isFetched {
			if err != nil {
				return err
			}
			continue
		}
		if titlesByEpisode == nil {
			if titlesByEpisode, err = seriesTitlesByEpisode(ctx, queries, provider, seriesID); err != nil {
				return err
			}
		}
		if err := placeInGroup(ctx, queries, ordering.ID, parts, titlesByEpisode); err != nil {
			return fmt.Errorf("place series %d's episodes in its %s ordering: %w", seriesID, ordering.Name, err)
		}
		if err := nameGroups(ctx, queries, ordering.ID, parts); err != nil {
			return fmt.Errorf("name the groups of series %d's %s ordering: %w", seriesID, ordering.Name, err)
		}
	}
	return nil
}

// deriveListedSeasons makes a group in a series' aired order for each season its own stored response
// lists, the specials among them, whether any episode of it is found or not, so that a season's
// poster has its group to name: identify alone makes groups, and artwork only looks them up (D426
// section 5).
func deriveListedSeasons(ctx context.Context, queries *sqlcgen.Queries, provider Provider, seriesID int64) error {
	ownGzip, err := store.OrNotFound(queries.GetProviderResponse(ctx, sqlcgen.GetProviderResponseParams{
		ItemID: seriesID, Provider: provider.Name(), Part: "",
	}))
	if errors.Is(err, store.ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read series %d's own response: %w", seriesID, err)
	}
	seasons := seasonsWithSpecials(provider, ownGzip)
	if len(seasons) == 0 {
		return nil
	}
	ordering, err := airedOrderingOf(ctx, queries, seriesID)
	if err != nil {
		return err
	}
	for _, season := range seasons {
		if err := makeGroup(ctx, queries, ordering, int64(season)); err != nil {
			return err
		}
	}
	return nil
}

// makeGroup makes an ordering's group of a number, a season or a disc, when it has none yet (D426
// section 5): what names a group by its number, a position or a name, finds it made.
func makeGroup(ctx context.Context, queries *sqlcgen.Queries, orderingID, number int64) error {
	if err := queries.InsertGroup(ctx, sqlcgen.InsertGroupParams{OrderingID: orderingID, Number: number}); err != nil {
		return fmt.Errorf("make group %d of ordering %d: %w", number, orderingID, err)
	}
	return nil
}

// deriveGroupList keeps an ordering for each group a series' stored list names.
func deriveGroupList(ctx context.Context, queries *sqlcgen.Queries, provider Provider, seriesID int64) error {
	body, isStored, err := storedPart(ctx, queries, provider, seriesID, groupListPart)
	if err != nil || !isStored || len(body) == 0 {
		return err
	}
	groups, err := provider.ParseEpisodeGroups(body)
	if err != nil {
		return fmt.Errorf("read series %d's list of episode groups: %w", seriesID, err)
	}
	existing, err := queries.ListProviderOrderingsOfSeries(ctx, sqlcgen.ListProviderOrderingsOfSeriesParams{
		SequenceID: seriesID, Provider: new(provider.Name()),
	})
	if err != nil {
		return fmt.Errorf("list series %d's %s orderings: %w", seriesID, provider.Name(), err)
	}
	byName := map[string]sqlcgen.ListProviderOrderingsOfSeriesRow{}
	for _, ordering := range existing {
		byName[ordering.Name] = ordering
	}
	for _, group := range groups {
		listed := sqlcgen.UpsertProviderOrderingParams{
			SequenceID: seriesID, Name: groupOrderingName(provider.Name(), group.ID), Provider: new(provider.Name()),
			Kind: ptr.NilIfZero(group.Kind), SourceID: new(group.ID), Label: ptr.NilIfZero(group.Name),
			ListedCount: ptr.NilIfZero(int64(group.EpisodeCount)),
		}
		if was, isKept := byName[listed.Name]; isKept && ptr.ValueOrZero(was.Kind) == ptr.ValueOrZero(listed.Kind) &&
			ptr.ValueOrZero(was.SourceID) == group.ID && ptr.ValueOrZero(was.Label) == ptr.ValueOrZero(listed.Label) &&
			ptr.ValueOrZero(was.ListedCount) == ptr.ValueOrZero(listed.ListedCount) {
			continue
		}
		if err := queries.UpsertProviderOrdering(ctx, listed); err != nil {
			return fmt.Errorf("keep series %d's %s ordering: %w", seriesID, listed.Name, err)
		}
	}
	return nil
}

// storedPart is one stored part of a series' response, ungzipped, and whether it is stored.
func storedPart(
	ctx context.Context, queries *sqlcgen.Queries, provider Provider, seriesID int64, part string,
) ([]byte, bool, error) {
	bodyGzip, err := store.OrNotFound(queries.GetProviderResponse(ctx, sqlcgen.GetProviderResponseParams{
		ItemID: seriesID, Provider: provider.Name(), Part: part,
	}))
	if errors.Is(err, store.ErrNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("read series %d's %q: %w", seriesID, part, err)
	}
	body, err := bounded.Gunzip(bodyGzip, maxStoredResponseBytes)
	if err != nil {
		return nil, false, fmt.Errorf("series %d's %q: %w", seriesID, part, err)
	}
	return body, true, nil
}

// storedGroup is a series' stored episode group's parts, and whether it is fetched; one its provider
// did not have is stored empty, and has none.
func storedGroup(
	ctx context.Context, queries *sqlcgen.Queries, provider Provider, seriesID int64, groupID string,
) ([]EpisodeGroupPart, bool, error) {
	body, isStored, err := storedPart(ctx, queries, provider, seriesID, groupPart(groupID))
	if err != nil || !isStored {
		return nil, false, err
	}
	if len(body) == 0 {
		return nil, true, nil
	}
	parts, err := provider.ParseEpisodeGroup(body)
	if err != nil {
		return nil, false, fmt.Errorf("read series %d's episode group %s: %w", seriesID, groupID, err)
	}
	return parts, true, nil
}

// seriesTitlesByEpisode are a series' live episodes by their id at the provider, which a group names
// its episodes by (§3.2); two copies of one episode are both placed.
func seriesTitlesByEpisode(
	ctx context.Context, queries *sqlcgen.Queries, provider Provider, seriesID int64,
) (map[string][]int64, error) {
	rows, err := queries.ListSeriesEpisodeSourceIDs(ctx, sqlcgen.ListSeriesEpisodeSourceIDsParams{
		SequenceID: seriesID, Provider: new(provider.Name()),
	})
	if err != nil {
		return nil, fmt.Errorf("list series %d's episodes at %s: %w", seriesID, provider.Name(), err)
	}
	titles := map[string][]int64{}
	for _, row := range rows {
		titles[row.FollowedSourceID] = append(titles[row.FollowedSourceID], row.WorkID)
	}
	return titles, nil
}

// placeInGroup places each episode a group lists in its ordering, as Kodi and Jellyfin read a group
// (§2.1, §3.2): its part's order is its group, its own order plus one its number, and the two make its
// sort order as aired's season and number do; an episode the group no longer lists loses its place.
func placeInGroup(
	ctx context.Context, queries *sqlcgen.Queries, orderingID int64, parts []EpisodeGroupPart,
	titlesByEpisode map[string][]int64,
) error {
	wanted := map[int64]sqlcgen.UpsertPositionParams{}
	for _, part := range parts {
		for _, entry := range part.Episodes {
			group, number := int64(part.Order), int64(entry.Order+1)
			for _, titleID := range titlesByEpisode[entry.EpisodeID] {
				wanted[titleID] = sqlcgen.UpsertPositionParams{
					OrderingID: orderingID, WorkID: titleID, GroupNumber: &group, NumberInGroup: &number,
					SortOrder: float64(group)*10_000 + float64(number),
				}
			}
		}
	}
	placed, err := queries.ListOrderingPositions(ctx, orderingID)
	if err != nil {
		return fmt.Errorf("list ordering %d's places: %w", orderingID, err)
	}
	for _, was := range placed {
		place, isWanted := wanted[was.WorkID]
		switch {
		case !isWanted:
			unplaced := sqlcgen.DeletePositionParams{OrderingID: orderingID, WorkID: was.WorkID}
			err = queries.DeletePosition(ctx, unplaced)
		case ptr.ValueOrZero(was.GroupNumber) == *place.GroupNumber &&
			ptr.ValueOrZero(was.NumberInGroup) == *place.NumberInGroup && was.SortOrder == place.SortOrder:
		default:
			err = placeTitle(ctx, queries, place)
		}
		if err != nil {
			return fmt.Errorf("place title %d: %w", was.WorkID, err)
		}
		delete(wanted, was.WorkID)
	}
	for titleID, place := range wanted {
		if err := placeTitle(ctx, queries, place); err != nil {
			return fmt.Errorf("place title %d: %w", titleID, err)
		}
	}
	return nil
}

// placeTitle places a title in an ordering, making its group first: a position names its group's row
// (D426 section 5), which UpsertPosition finds by its number.
func placeTitle(ctx context.Context, queries *sqlcgen.Queries, place sqlcgen.UpsertPositionParams) error {
	if place.GroupNumber != nil {
		if err := makeGroup(ctx, queries, place.OrderingID, *place.GroupNumber); err != nil {
			return err
		}
	}
	if err := queries.UpsertPosition(ctx, place); err != nil {
		return fmt.Errorf("place it in ordering %d: %w", place.OrderingID, err)
	}
	return nil
}

// nameGroups keeps each part's name as its group's in the ordering, and drops the name of a group it
// no longer has.
func nameGroups(ctx context.Context, queries *sqlcgen.Queries, orderingID int64, parts []EpisodeGroupPart) error {
	wanted := map[int64]string{}
	for _, part := range parts {
		if part.Name != "" {
			wanted[int64(part.Order)] = part.Name
		}
	}
	named, err := queries.ListOrderingGroupTexts(ctx, orderingID)
	if err != nil {
		return fmt.Errorf("list ordering %d's group names: %w", orderingID, err)
	}
	for _, was := range named {
		if was.Language != groupNameLanguage {
			continue
		}
		name, isWanted := wanted[was.GroupNumber]
		switch {
		case !isWanted:
			err = queries.DeleteGroupText(ctx, sqlcgen.DeleteGroupTextParams{
				OrderingID: orderingID, GroupNumber: was.GroupNumber, Language: groupNameLanguage,
			})
		case ptr.ValueOrZero(was.Name) != name:
			err = queries.UpsertGroupText(ctx, sqlcgen.UpsertGroupTextParams{ // named, so its group is made
				OrderingID: orderingID, GroupNumber: was.GroupNumber, Language: groupNameLanguage, Name: &name,
			})
		}
		if err != nil {
			return fmt.Errorf("name group %d: %w", was.GroupNumber, err)
		}
		delete(wanted, was.GroupNumber)
	}
	for group, name := range wanted {
		err := makeGroup(ctx, queries, orderingID, group)
		if err == nil {
			err = queries.UpsertGroupText(ctx, sqlcgen.UpsertGroupTextParams{
				OrderingID: orderingID, GroupNumber: group, Language: groupNameLanguage, Name: &name,
			})
		}
		if err != nil {
			return fmt.Errorf("name group %d: %w", group, err)
		}
	}
	return nil
}

// keepOrderingsDerived derives a series' orderings from what is stored (deriveOrderings), taking the
// writer only when something changed.
func keepOrderingsDerived(ctx context.Context, database *store.Database, provider Provider, seriesID int64) error {
	_, err := database.WriteUnlessUnchanged(ctx, func(queries *sqlcgen.Queries) error {
		return deriveOrderings(ctx, queries, provider, seriesID)
	})
	if err != nil {
		return fmt.Errorf("derive series %d's orderings: %w", seriesID, err)
	}
	return nil
}

// deriveLibraryOrderings keeps the orderings of each of a library's series with a group fetched as
// what is stored says, its episodes placed as they come and go: a pass's last step. A library with
// none runs one statement.
func (pass *Pass) deriveLibraryOrderings(ctx context.Context, libraryID int64) error {
	seriesIDs, err := pass.database.Read().ListLibrarySeriesWithFetchedGroups(ctx, &libraryID)
	if err != nil {
		return fmt.Errorf("list library %d's series with episode groups: %w", libraryID, err)
	}
	for _, seriesID := range seriesIDs {
		for _, provider := range pass.providers {
			if err := keepOrderingsDerived(ctx, pass.database, provider, seriesID); err != nil {
				return err
			}
		}
	}
	return nil
}

// fetchOwedOrderings fetches what a matched series' orderings are owed (D170, D335 §1): its list of
// episode groups, taken from its own stored record where that carries it, no request made, else
// fetched; and each group a person chose that is not fetched yet. It stores each and derives the
// series' orderings, and says whether it fetched anything.
func (matcher *Matcher) fetchOwedOrderings(
	ctx context.Context, provider Provider, series sqlcgen.GetSeriesForMatchRow,
) (bool, error) {
	queries := matcher.database.Read()
	sourceID := ptr.ValueOrZero(series.FollowedSourceID)
	fetched := false
	if _, isStored, err := storedPart(ctx, queries, provider, series.ID, groupListPart); err != nil {
		return false, err
	} else if !isStored {
		list, err := matcher.groupList(ctx, provider, series.ID, sourceID)
		if err != nil {
			return false, err
		}
		if err := matcher.storePartBody(ctx, provider, series.ID, groupListPart, list); err != nil {
			return false, err
		}
		fetched = true
		// The orderings the list names are what a person's choice can name.
		if err := keepOrderingsDerived(ctx, matcher.database, provider, series.ID); err != nil {
			return false, err
		}
	}
	chosen, err := queries.ListChosenGroupsToFetch(ctx, sqlcgen.ListChosenGroupsToFetchParams{
		Axis: preferences.AxisEpisodeOrdering, SequenceID: series.ID, Provider: new(provider.Name()),
	})
	if err != nil {
		return false, fmt.Errorf("list the episode groups chosen in series %d: %w", series.ID, err)
	}
	for _, groupID := range chosen {
		body, err := provider.FetchEpisodeGroup(ctx, groupID)
		if errors.Is(err, ErrNoMatch) {
			body, err = []byte{}, nil // the provider has no such group: stored empty, so it is not asked again
		}
		if err != nil {
			return false, fmt.Errorf("fetch series %d's episode group %s: %w", series.ID, groupID, err)
		}
		if err := matcher.storePartBody(ctx, provider, series.ID, groupPart(groupID), body); err != nil {
			return false, err
		}
		fetched = true
	}
	if !fetched {
		return false, nil
	}
	return true, keepOrderingsDerived(ctx, matcher.database, provider, series.ID)
}

// groupList is a series' list of episode groups: the one its stored record carries, with no request
// (§2.1), else its provider's answer; an empty one where the provider has none.
func (matcher *Matcher) groupList(
	ctx context.Context, provider Provider, seriesID int64, sourceID string,
) ([]byte, error) {
	record, isStored, err := storedPart(ctx, matcher.database.Read(), provider, seriesID, "")
	if err != nil {
		return nil, err
	}
	if isStored {
		if list, isCarried := provider.EpisodeGroupsIn(record); isCarried {
			return list, nil
		}
	}
	list, err := provider.FetchEpisodeGroups(ctx, sourceID)
	if errors.Is(err, ErrNoMatch) {
		return []byte{}, nil // stored empty, so it is not asked again
	}
	if err != nil {
		return nil, fmt.Errorf("fetch series %d's list of episode groups: %w", seriesID, err)
	}
	return list, nil
}

// storePartBody stores one fetched part of a series' response, as fetched now.
func (matcher *Matcher) storePartBody(
	ctx context.Context, provider Provider, seriesID int64, part string, body []byte,
) error {
	bodyGzip, err := bounded.Gzip(body)
	if err != nil {
		return fmt.Errorf("keep the provider's answer: %w", err)
	}
	err = matcher.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		return queries.UpsertProviderResponse(ctx, sqlcgen.UpsertProviderResponseParams{
			ItemID: seriesID, Provider: provider.Name(), Part: part,
			FetchedAt: matcher.clock.Now().UnixMilli(), BodyGzip: bodyGzip,
		})
	})
	if err != nil {
		return fmt.Errorf("store series %d's %q: %w", seriesID, part, err)
	}
	return nil
}
