// SPDX-License-Identifier: AGPL-3.0-or-later

package identify

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// UnplacedEpisodes is a series some of whose episodes have no number though no specials or extras
// folder holds them: their files' names were likely read wrongly, as a series' "1x4" were on the
// owner's NAS (2026-10-16), and the admin is told (D306 §6).
type UnplacedEpisodes struct {
	SeriesID   int64
	SeriesName string
	FileCount  int
}

// unplacedAtLeast is how many episodes of a series must be unplaced for it to be told: a few are an
// unaired pilot or specials a folder does not name, which need nobody (staging, 2026-10-16: Red
// Dwarf's three specials, each in a folder of its own).
const unplacedAtLeast = 5

// unplacedFilesRead is the row limit a look gives the read of unnumbered episodes' files: none (a
// negative one reads them all). Which of them count is decided by their folders' and names' words,
// in Go, so no query can total the series; a read that stopped short would under-report (D432 §4).
const unplacedFilesRead = -1

// SeriesWithUnplacedEpisodes are the series with at least unplacedAtLeast episodes whose files have
// no number and are in no specials or extras folder, in the order of their ids.
func (chooser *Chooser) SeriesWithUnplacedEpisodes(ctx context.Context) ([]UnplacedEpisodes, error) {
	rows, err := chooser.matcher.database.Read().ListUnnumberedEpisodeFiles(ctx, unplacedFilesRead)
	if err != nil {
		return nil, fmt.Errorf("list the episodes with no number: %w", err)
	}
	return unplacedOf(rows), nil
}

// isBonusName is whether a file's own name says it is bonus material, as a specials or extras
// folder's would, or as a disc's are named: "Season 6 Special Features - Commercials", "Music Video -
// Its Song Name" (staging, 2026-10-16: an animated series' sixteen).
func isBonusName(name string) bool {
	if isBonusFolder(name) {
		return true
	}
	lower := strings.ToLower(name)
	for _, word := range []string{"music video", "commercial", "documentary", "trailer", "interview", "making of"} {
		if strings.Contains(lower, word) {
			return true
		}
	}
	return false
}

// unplacedOf counts each series' unnumbered files outside a specials or extras folder.
func unplacedOf(rows []sqlcgen.ListUnnumberedEpisodeFilesRow) []UnplacedEpisodes {
	var found []UnplacedEpisodes
	for _, row := range rows {
		if _, isBonus := seasonFromFolders(path.Dir(row.Path)); isBonus || isBonusName(path.Base(row.Path)) {
			continue
		}
		if last := len(found) - 1; last >= 0 && found[last].SeriesID == row.SeriesID {
			found[last].FileCount++
			continue
		}
		found = append(found, UnplacedEpisodes{SeriesID: row.SeriesID, SeriesName: row.SeriesName, FileCount: 1})
	}
	kept := found[:0]
	for _, series := range found {
		if series.FileCount >= unplacedAtLeast {
			kept = append(kept, series)
		}
	}
	return kept
}
