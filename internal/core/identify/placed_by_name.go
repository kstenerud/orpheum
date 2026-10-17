// SPDX-License-Identifier: AGPL-3.0-or-later

package identify

import (
	"context"
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/kstenerud/orpheum/internal/core/textindex"
	"github.com/kstenerud/orpheum/internal/platform/bounded"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// unplacedPathsRead bounds how many of a series' unnumbered files a look reads: one is enough.
const unplacedPathsRead = 200

// hasUnplacedEpisodes is whether a series has an episode with no number whose file is in no
// specials or extras folder and not named as bonus material: one whose name may place it in a
// season (D307 §2).
func hasUnplacedEpisodes(ctx context.Context, queries *sqlcgen.Queries, seriesID int64) (bool, error) {
	paths, err := queries.ListUnnumberedEpisodePathsOfSeries(ctx, sqlcgen.ListUnnumberedEpisodePathsOfSeriesParams{
		SeriesID: seriesID, MaxRows: unplacedPathsRead,
	})
	if err != nil {
		return false, fmt.Errorf("list series %d's episodes with no number: %w", seriesID, err)
	}
	return slices.ContainsFunc(paths, isPlaceableByName), nil
}

// isPlaceableByName is whether a file with no number is one its name may place in a season: in no
// specials or extras folder, and not named as bonus material.
func isPlaceableByName(filePath string) bool {
	_, isBonus := seasonFromFolders(path.Dir(filePath))
	return !isBonus && !isBonusName(path.Base(filePath))
}

// listedSeasons are the seasons a series' own stored response, gzipped as kept, lists, the
// specials' 0 left out; none before it is fetched, or from one that cannot be read.
func listedSeasons(provider Provider, ownBodyGzip []byte) []int {
	if len(ownBodyGzip) == 0 {
		return nil
	}
	body, err := bounded.Gunzip(ownBodyGzip, maxStoredResponseBytes)
	if err != nil {
		return nil
	}
	metadata, err := provider.Parse(KindSeries, body)
	if err != nil {
		return nil
	}
	return slices.DeleteFunc(slices.Clone(metadata.SeasonNumbers), func(season int) bool { return season <= 0 })
}

// placement is where an episode is placed: its season and number.
type placement struct {
	season, number int
}

// placedByName is the season and number one episode of a series' stored listings of seasons has
// for a name, as a guess (D306 §2, D307 §2): the one whose name is the file's, or nearly, of all
// the seasons listed; false when no episode, or more than one, is that near.
func (filer *libraryFiler) placedByName(
	ctx context.Context, seriesID int64, provider Provider, seriesName, name string,
) (placement, bool, error) {
	ownBody, _, err := filer.storedResponse(ctx, seriesID, provider.Name(), "")
	if err != nil {
		return placement{}, false, fmt.Errorf("read series %d's own response: %w", seriesID, err)
	}
	wanted := textindex.SameWork(name)
	if wanted == "" {
		return placement{}, false, nil
	}
	// A file's name often carries its series' before its own: "Orbitorama - Comet Pilot 3000".
	withoutSeries := wanted
	if series := textindex.SameWork(seriesName); series != "" && len(wanted) > len(series) {
		withoutSeries = strings.TrimSpace(strings.TrimPrefix(wanted, series))
	}
	best, bestScore, isTied := Episode{}, 0.0, false
	for _, listed := range listedSeasons(provider, ownBody) {
		episodes, _, listErr := filer.seasonListing(ctx, seriesID, provider, listed)
		if listErr != nil {
			return placement{}, false, listErr
		}
		for _, episode := range episodes {
			listedName := textindex.SameWork(episode.Name)
			score := max(nameNearness(wanted, listedName), nameNearness(withoutSeries, listedName))
			switch {
			case score > bestScore:
				best, bestScore, isTied = episode, score, false
			case score == bestScore && score > 0:
				isTied = true
			}
		}
	}
	if bestScore < nearEnough || isTied {
		return placement{}, false, nil
	}
	return placement{season: best.Season, number: best.Number}, true, nil
}

// nearEnough is how near a file's name must be to a listed episode's to be placed there: the
// same, ending with it, or differing in a few letters of ten (a spelling, a dropped word).
const nearEnough = 0.85

// nameNearness is how near two folded names are, 1 for the same: 0.95 for a name that ends with a
// listed one long enough not to end too many ("Quillwick - Born to be King" and "Born to Be
// King"), else one less the share of letters that differ.
func nameNearness(wanted, listed string) float64 {
	switch {
	case listed == "":
		return 0
	case wanted == listed:
		return 1
	case len(listed) >= minimumSpecialName && strings.HasSuffix(wanted, listed):
		return 0.95
	}
	first, second := []rune(wanted), []rune(listed)
	longest := max(len(first), len(second))
	return 1 - float64(editDistance(first, second))/float64(longest)
}

// editDistance is how many letters must change, be added or go to make one into the other.
func editDistance(first, second []rune) int {
	previous := make([]int, len(second)+1)
	current := make([]int, len(second)+1)
	for index := range previous {
		previous[index] = index
	}
	for row := 1; row <= len(first); row++ {
		current[0] = row
		for column := 1; column <= len(second); column++ {
			cost := 1
			if first[row-1] == second[column-1] {
				cost = 0
			}
			current[column] = min(previous[column]+1, current[column-1]+1, previous[column-1]+cost)
		}
		previous, current = current, previous
	}
	return previous[len(second)]
}
