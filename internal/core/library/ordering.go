// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/preferences"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// ViewingOrdering is the ordering userID watches seriesID in, browses it in and is offered its Next
// Up in (D159 §1, D210 §3): their choice, kept as a preference at the series' work scope, when it
// names an ordering the series still has; else the series' files' order (D210 §5), which is aired
// until an admin can name another (episode-orderings.md §7 Q3). It is the one answer to "which
// ordering" for everything a person sees of a series; identification never asks it.
func ViewingOrdering(ctx context.Context, queries *sqlcgen.Queries, userID, seriesID int64) (string, error) {
	chosen, err := store.OrNotFound(queries.GetChosenOrdering(ctx, sqlcgen.GetChosenOrderingParams{
		UserID: userID, CollectionID: seriesID, Axis: preferences.AxisEpisodeOrdering,
	}))
	switch {
	case errors.Is(err, store.ErrNotFound):
		return OrderingAired, nil
	case err != nil:
		return "", fmt.Errorf("read the ordering user %d watches series %d in: %w", userID, seriesID, err)
	}
	return chosen, nil
}

// EpisodePlace is an episode's series and aired place, and its place in the ordering its person
// watches the series in, which is what labels it wherever they see it (episode-orderings.md §7 Q8).
type EpisodePlace struct {
	sqlcgen.GetEpisodePlaceRow
	// Ordering is the ordering the person watches the series in.
	Ordering string
	// InOrdering is its place in that ordering when it is not aired; nil in aired, whose place is the
	// row's, and when the ordering leaves it out.
	InOrdering *OrderingPlace
}

// OrderingPlace is a member's place in a named ordering other than aired: its group and number in
// it, the group's name where the ordering names it, and whether the ordering is a single run, whose
// one group is no heading (an absolute order).
type OrderingPlace struct {
	Group, NumberInGroup int64
	GroupName            string
	IsOnlyGroup          bool
}

// episodePlace is an episode's series and place as userID sees it, or nil for a title in none.
func (browser *Browser) episodePlace(ctx context.Context, userID, titleID int64) (*EpisodePlace, error) {
	queries := browser.database.Read()
	aired, err := store.OrNotFound(queries.GetEpisodePlace(ctx, titleID))
	switch {
	case errors.Is(err, store.ErrNotFound):
		return nil, nil //nolint:nilnil // no place is an answer, not an error
	case err != nil:
		return nil, fmt.Errorf("read title %d's place: %w", titleID, err)
	}
	ordering, err := ViewingOrdering(ctx, queries, userID, aired.CollectionID)
	if err != nil {
		return nil, err
	}
	place := &EpisodePlace{GetEpisodePlaceRow: aired, Ordering: ordering}
	if ordering == OrderingAired {
		return place, nil
	}
	inOrdering, err := store.OrNotFound(queries.GetPlaceInOrdering(ctx, sqlcgen.GetPlaceInOrderingParams{
		CollectionID: aired.CollectionID, OrderingName: ordering, TitleID: titleID,
	}))
	switch {
	case errors.Is(err, store.ErrNotFound):
		return place, nil // the ordering leaves it out
	case err != nil:
		return nil, fmt.Errorf("read title %d's place in the %s ordering: %w", titleID, ordering, err)
	}
	place.InOrdering = &OrderingPlace{
		Group: ptr.ValueOrZero(inOrdering.GroupNumber), NumberInGroup: ptr.ValueOrZero(inOrdering.NumberInGroup),
		GroupName: inOrdering.GroupName, IsOnlyGroup: inOrdering.GroupCount == 1,
	}
	return place, nil
}

// Ordering is one of a series' orderings, as its page offers it (episode-orderings.md §4): its name,
// the key a person's choice keeps; its kind and its provider's label for it, "" for aired; how many
// episodes its provider lists in it, 0 when it did not say; and how many of the series' episodes it
// holds.
type Ordering struct {
	Name, Kind, Label string
	ListedCount       int64
	EpisodeCount      int64
}

// withOrderings gives a series' detail every ordering it has, the names its ordering gives its
// groups, and how many of its episodes that ordering leaves out.
func (browser *Browser) withOrderings(ctx context.Context, detail *SeriesDetail) error {
	queries := browser.database.Read()
	rows, err := queries.ListSeriesOrderings(ctx, detail.ID)
	if err != nil {
		return fmt.Errorf("list series %d's orderings: %w", detail.ID, err)
	}
	for _, row := range rows {
		detail.Orderings = append(detail.Orderings, Ordering{
			Name: row.Name, Kind: ptr.ValueOrZero(row.Kind), Label: ptr.ValueOrZero(row.Label),
			ListedCount: ptr.ValueOrZero(row.ListedCount), EpisodeCount: row.EpisodeCount,
		})
	}
	names, err := groupNames(ctx, queries, detail.ID, detail.Ordering)
	if err != nil {
		return err
	}
	for index := range detail.Seasons {
		detail.Seasons[index].Name = names[detail.Seasons[index].Number]
	}
	if detail.Ordering == OrderingAired {
		return nil
	}
	detail.NotInOrderingCount, err = queries.CountEpisodesNotInOrdering(ctx, sqlcgen.CountEpisodesNotInOrderingParams{
		CollectionID: detail.ID, OrderingName: detail.Ordering,
	})
	if err != nil {
		return fmt.Errorf("count series %d's episodes its %s ordering leaves out: %w", detail.ID, detail.Ordering, err)
	}
	return nil
}

// groupNames are the names a series' ordering gives its groups, by number: each in the first
// language it has one in.
func groupNames(
	ctx context.Context, queries *sqlcgen.Queries, seriesID int64, ordering string,
) (map[int64]string, error) {
	rows, err := queries.ListOrderingGroupNames(ctx, sqlcgen.ListOrderingGroupNamesParams{
		CollectionID: seriesID, OrderingName: ordering,
	})
	if err != nil {
		return nil, fmt.Errorf("read the group names of series %d's %s ordering: %w", seriesID, ordering, err)
	}
	names := map[int64]string{}
	for _, row := range rows {
		if _, isNamed := names[row.GroupNumber]; !isNamed {
			names[row.GroupNumber] = row.Name
		}
	}
	return names, nil
}

// HasOrdering is whether a series reach reaches has an ordering of that name; a series it does not
// reach is ErrNoItem, as one that does not exist (D243 §4).
func (browser *Browser) HasOrdering(ctx context.Context, reach auth.Reach, seriesID int64, name string) (bool, error) {
	if _, err := browser.reachedSeries(ctx, reach, seriesID); err != nil {
		return false, err
	}
	rows, err := browser.database.Read().ListSeriesOrderings(ctx, seriesID)
	if err != nil {
		return false, fmt.Errorf("list series %d's orderings: %w", seriesID, err)
	}
	return slices.ContainsFunc(rows, func(row sqlcgen.ListSeriesOrderingsRow) bool { return row.Name == name }), nil
}
