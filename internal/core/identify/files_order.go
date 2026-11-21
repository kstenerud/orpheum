// SPDX-License-Identifier: AGPL-3.0-or-later

package identify

import (
	"context"
	"fmt"

	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// filesOrder is the ordering a series' files are numbered in, as a pass reads it once (D210 §1;
// episode-orderings.md §3.4): none set, aired; set but its group not fetched yet, aired until it is
// (fetching the group is queued when it is set); else its entries by the group and number a file
// gives.
type filesOrder struct {
	name     string
	provider string
	entries  map[[2]int]Episode // by part order and order plus one: the group's own numbering
	isRead   bool               // the group is fetched and read: a number it lacks is not aired's
}

// filesOrderOf is a series' files' order, read once per pass: the library's series set to one are
// listed once, and each of their groups read once.
func (filer *libraryFiler) filesOrderOf(ctx context.Context, seriesID int64) (filesOrder, error) {
	if filer.filesOrders == nil {
		listed, err := filer.queries.ListLibrarySeriesFilesOrderings(ctx, &filer.library.ID)
		if err != nil {
			return filesOrder{}, fmt.Errorf("list library %d's files' orders: %w", filer.library.ID, err)
		}
		filer.filesOrders = map[int64]filesOrder{}
		filer.filesOrdersSet = map[int64]sqlcgen.ListLibrarySeriesFilesOrderingsRow{}
		for _, set := range listed {
			filer.filesOrdersSet[set.SequenceID] = set
		}
	}
	if order, found := filer.filesOrders[seriesID]; found {
		return order, nil
	}
	order, err := filer.readFilesOrder(ctx, seriesID)
	if err != nil {
		return filesOrder{}, err
	}
	filer.filesOrders[seriesID] = order
	return order, nil
}

func (filer *libraryFiler) readFilesOrder(ctx context.Context, seriesID int64) (filesOrder, error) {
	set, isSet := filer.filesOrdersSet[seriesID]
	if !isSet {
		return filesOrder{name: airedOrdering}, nil
	}
	order := filesOrder{name: set.Name}
	provider, known := filer.providers[providerOf(filer.library.MetadataSource)]
	if !known || set.Provider == nil || *set.Provider != provider.Name() || set.SourceID == nil {
		return order, nil
	}
	order.provider = provider.Name()
	body, isStored, err := storedPart(ctx, filer.queries, provider, seriesID, groupPart(*set.SourceID))
	if err != nil || !isStored || len(body) == 0 {
		return order, err
	}
	parts, err := provider.ParseEpisodeGroup(body)
	if err != nil {
		return filesOrder{}, fmt.Errorf("read series %d's %s group: %w", seriesID, set.Name, err)
	}
	order.entries, order.isRead = map[[2]int]Episode{}, true
	for _, part := range parts {
		for _, entry := range part.Episodes {
			if entry.Aired.Number > 0 {
				order.entries[[2]int{part.Order, entry.Order + 1}] = entry.Aired
			}
		}
	}
	return order, nil
}

// aired is where a file numbered (season, number) in the files' order is in aired, with the
// episode the group's entry gives it; isListed is false where the group, read, lists no such place.
// In aired order a file's numbers are its place.
func (order filesOrder) aired(season, number int) (episode Episode, isListed bool) {
	if !order.isRead {
		return Episode{Season: season, Number: number}, true
	}
	episode, isListed = order.entries[[2]int{season, number}]
	return episode, isListed
}

// inFilesOrder is what a series' files' order does to a file numbered in it.
type inFilesOrder int

const (
	numberedInAired    inFilesOrder = iota // aired, or a group not fetched yet: its numbers are aired's
	placedByFilesOrder                     // its group's entry gave its aired place and its values
	notInFilesOrder                        // its group lists no such number: filed as an unnumbered special
)

// fromFilesOrder gives a file numbered in its series' files' order its aired place and its values
// from the group's entry (§3.3), which fileEpisode then keys and places it by.
func (filer *libraryFiler) fromFilesOrder(
	ctx context.Context, seriesID int64, season, number *int, spec *titleSpec,
) (inFilesOrder, error) {
	order, err := filer.filesOrderOf(ctx, seriesID)
	if err != nil || !order.isRead {
		return numberedInAired, err
	}
	episode, isListed := order.aired(*season, *number)
	if !isListed {
		return notInFilesOrder, nil
	}
	*season, *number = episode.Season, episode.Number
	spec.fromEpisode(order.provider, episode)
	return placedByFilesOrder, nil
}

// isPlacedAgainstFilesOrder is whether an episode's file, numbered (season, number), is at an aired
// place its series' files' order no longer gives those numbers, or at one where the order lists
// none: the order was set, or changed, since it was filed, and it is filed again
// (contradictedPlacements).
func (filer *libraryFiler) isPlacedAgainstFilesOrder(
	ctx context.Context, place sqlcgen.GetEpisodePlaceRow, season, number int,
) (bool, error) {
	order, err := filer.filesOrderOf(ctx, place.SequenceID)
	if err != nil {
		return false, err
	}
	episode, isListed := order.aired(season, number)
	isAtANumber := place.GroupNumber != nil && place.NumberInGroup != nil
	switch {
	case !isListed: // the group lists no such number: an unnumbered special, not at an aired number
		return isAtANumber, nil
	case !isAtANumber: // filed as one, before the order changed
		return true, nil
	default:
		return int64(episode.Season) != *place.GroupNumber || int64(episode.Number) != *place.NumberInGroup, nil
	}
}

// markAgainstFilesOrder marks a file contradicted that isNumberedAgainstFilesOrder.
func (filer *libraryFiler) markAgainstFilesOrder(
	ctx context.Context, file sqlcgen.ListIdentifyFilesRow, place sqlcgen.GetEpisodePlaceRow, relativePath string,
) error {
	against, err := filer.isNumberedAgainstFilesOrder(ctx, file, place, relativePath)
	if against {
		filer.contradicted[file.ID] = true
	}
	return err
}

// isNumberedAgainstFilesOrder is whether a file its own numbers place, in a library that reads a
// provider, is at an aired place its series' files' order does not give those numbers: one its
// episode NFO numbers, or one its path does not number, is placed otherwise.
func (filer *libraryFiler) isNumberedAgainstFilesOrder(
	ctx context.Context, file sqlcgen.ListIdentifyFilesRow, place sqlcgen.GetEpisodePlaceRow, relativePath string,
) (bool, error) {
	if filer.library.MetadataSource == sourceNFO {
		return false, nil
	}
	parsed := ParseEpisodePath(relativePath)
	if parsed.IsUnnumbered || parsed.Episode == 0 {
		return false, nil
	}
	nfo, hasNFO := filer.nfoFor(file)
	if hasNFO && nfo.Kind == nfoEpisodeDetails && (nfo.Season > 0 || nfo.Episode > 0) {
		return false, nil
	}
	return filer.isPlacedAgainstFilesOrder(ctx, place, parsed.Season, parsed.Episode)
}
