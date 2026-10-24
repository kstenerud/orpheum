// SPDX-License-Identifier: AGPL-3.0-or-later

package interest

import (
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// episode is one of a series' episodes, in aired order, with its live files and where its person
// is in it.
type episode struct {
	titleID int64
	// season is the season it is in, 0 for the specials and those with no season.
	season  int64
	fileIDs []int64
	// isPartway is whether its person is partway through one of its files, and isFinished whether
	// they finished one; watchedAt is when they last watched one, 0 for never.
	isPartway, isFinished bool
	watchedAt             int64
}

// episodesOf are a series' episodes, from its files in aired order, as a person stands in them.
func episodesOf(rows []sqlcgen.ListSeriesEpisodeFilesWithPositionsRow, thresholds library.ResumeThresholds) []episode {
	var episodes []episode
	for _, row := range rows {
		if len(episodes) == 0 || episodes[len(episodes)-1].titleID != row.TitleID {
			episodes = append(episodes, episode{titleID: row.TitleID, season: row.SeasonNumber})
		}
		last := &episodes[len(episodes)-1]
		last.fileIDs = append(last.fileIDs, row.FileID)
		if row.PositionMs == nil {
			continue
		}
		last.isPartway = last.isPartway || thresholds.IsPartwayThrough(*row.PositionMs, row.DurationMs)
		last.isFinished = last.isFinished || thresholds.IsFinished(*row.PositionMs, row.DurationMs)
		if row.PositionReadAt != nil && *row.PositionReadAt > last.watchedAt {
			last.watchedAt = *row.PositionReadAt
		}
	}
	return episodes
}

// episodesAhead are the count episodes from the person's place in the series (the owner,
// 2026-10-21): the one they are partway through, the latest watched when several are; else the one
// after the last they finished, the latest finished; else the first.
func episodesAhead(episodes []episode, count int) []episode {
	start, partwayAt, finishedAt := 0, int64(-1), int64(-1)
	for index, each := range episodes {
		switch {
		case each.isPartway && each.watchedAt > partwayAt:
			start, partwayAt = index, each.watchedAt
		case partwayAt < 0 && each.isFinished && each.watchedAt > finishedAt:
			start, finishedAt = index+1, each.watchedAt
		}
	}
	if start >= len(episodes) {
		return nil // every episode up to the last is finished
	}
	return episodes[start:min(len(episodes), start+count)]
}

// episodesAheadInSeason are the count episodes of a series' season from the person's place in it
// (the owner, 2026-10-24: interest follows the page, and a season's page is that season's): their
// place among its episodes alone, as episodesAhead finds it; else, when they finished its last, its
// first.
func episodesAheadInSeason(episodes []episode, season int64, count int) []episode {
	var inSeason []episode
	for _, each := range episodes {
		if each.season == season {
			inSeason = append(inSeason, each)
		}
	}
	if ahead := episodesAhead(inSeason, count); len(ahead) > 0 {
		return ahead
	}
	return inSeason[:min(len(inSeason), count)]
}
