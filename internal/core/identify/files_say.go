// SPDX-License-Identifier: AGPL-3.0-or-later

package identify

import (
	"context"
	"errors"
	"fmt"
	"maps"

	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// filesQuery is what a film's or a series' own files and NFOs say it is, its match set aside: the
// name and year a rescan searches for, and the provider ids they give (D357 §2). After a match the
// stored name is the provider's ("The Crowd" for the file "Crowd (1928).mkv"), so a match
// made again from the stored name would only find the old answer again.
type filesQuery struct {
	name string
	year int
	ids  map[string]string
	// isNFOSource is a film whose NFO speaks for it: it is not matched at a provider (D182).
	isNFOSource bool
}

// noteFilesSay keeps what a title's files say it is, before its match is laid over them, when the
// filer reads for a rescan (filesSayOf). Only a film is matched by itself; an episode is its
// series', and an extra or an other video never is.
func (filer *libraryFiler) noteFilesSay(titleID int64, specs []titleSpec) {
	if filer.filesSay == nil {
		return
	}
	best := bestEvidence(specs)
	if best.kind != KindMovie {
		return
	}
	if held, found := filer.filesSay[titleID]; found && (held.isNFOSource || best.source != sourceNFO) {
		return // the first unit's, unless a later one's NFO speaks for it
	}
	filer.filesSay[titleID] = filesQuery{
		name: best.name, year: best.year, ids: maps.Clone(best.ids), isNFOSource: best.source == sourceNFO,
	}
}

// noteSeriesFilesSay keeps what a series' folders and tvshow.nfo say it is, for a rescan.
func (filer *libraryFiler) noteSeriesFilesSay(seriesID int64, spec seriesSpec) {
	if filer.filesSay == nil {
		return
	}
	if _, found := filer.filesSay[seriesID]; found && !spec.hasNFO {
		return
	}
	filer.filesSay[seriesID] = filesQuery{name: spec.name, year: spec.year, ids: maps.Clone(spec.ids)}
}

// filesSayReads is how often filesSayOf reads a library that would change, filing it between.
const filesSayReads = 3

// errLibraryKeptChanging is a library that changed each time a rescan read what its files say.
var errLibraryKeptChanging = errors.New("the library changed each time it was read")

// filesSayOf is what each film's and series' files and NFOs in a library say it is, read as a pass
// reads them, by id (D357 §2). It reads in a read alone, which an unchanged library completes; a
// library the pass would change is filed first, then read again.
func (pass *Pass) filesSayOf(ctx context.Context, libraryID int64) (map[int64]filesQuery, error) {
	sorter, err := pass.sorter(ctx)
	if err != nil {
		return nil, err
	}
	var refused error // the write the last read would have made, refused: what keeps the library changing
	for range filesSayReads {
		said := map[int64]filesQuery{}
		wouldWrite, err := pass.database.WouldWrite(ctx, func(queries *sqlcgen.Queries) error {
			refused = nil
			clear(said)
			filer := pass.newFiler(queries, sorter, &Report{})
			filer.filesSay = said
			if loadErr := filer.load(ctx, libraryID); loadErr != nil {
				return loadErr
			}
			units, unitsErr := filer.filingUnits(ctx)
			if unitsErr != nil {
				return unitsErr
			}
			_, refused = filer.identifyUnits(ctx, units, -1, 0)
			return refused
		})
		if err != nil {
			return nil, fmt.Errorf("read what library %d's files say: %w", libraryID, err)
		}
		if !wouldWrite {
			return said, nil
		}
		if _, err := pass.IdentifyLibrary(ctx, libraryID); err != nil {
			return nil, err
		}
	}
	// The refused write names the statement and the item (a stuck rescan logged only that it waited,
	// for hours: one MarkGuessedMatch a pass).
	return nil, fmt.Errorf("library %d: %w; the write its last read would make: %w", libraryID,
		errLibraryKeptChanging, refused)
}
