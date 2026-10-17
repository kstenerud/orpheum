// SPDX-License-Identifier: AGPL-3.0-or-later

package identify

import (
	"context"
	"fmt"
	"slices"

	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// writeBatches writes units in batches, marking each unit's files written once its batch commits,
// until none is left. A batch that finds that someone else committed since commitsAtRead reads the
// library again with readInBatch, in its own transaction, and goes on; with none, it stops with
// errLibraryChanged. It returns the filer of the last read.
func (pass *Pass) writeBatches(
	ctx context.Context, filer *libraryFiler, units []filingUnit, commitsAtRead int64, written map[int64]bool,
	readInBatch func(*sqlcgen.Queries) (*libraryFiler, []filingUnit, error),
) (*libraryFiler, error) {
	batching := &batching{filer: filer, units: units, expected: commitsAtRead, readInBatch: readInBatch}
	for len(batching.units) > 0 {
		batch, batchRows, err := pass.writeBatch(ctx, batching)
		if err != nil {
			return batching.filer, fmt.Errorf("file a batch: %w", err)
		}
		for _, unit := range batch {
			for _, file := range unit.files {
				written[file.ID] = true
			}
		}
		if pass.betweenBatches != nil {
			pass.betweenBatches(ctx, batchRows)
		}
	}
	return batching.filer, nil
}

// batching is a pass's filing in batches: the filer of its last read, the units left, the commits
// it expects to find, and how it reads the library again inside a batch, if it may.
type batching struct {
	filer       *libraryFiler
	units       []filingUnit
	expected    int64
	readInBatch func(*sqlcgen.Queries) (*libraryFiler, []filingUnit, error)
}

// writeBatch writes the next batch of units, in one write, and returns them and the rows it changed.
func (pass *Pass) writeBatch(ctx context.Context, batching *batching) ([]filingUnit, int64, error) {
	var batch []filingUnit
	var batchRows int64
	err := pass.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		if pass.database.Commits() != batching.expected {
			if batching.readInBatch == nil {
				return errLibraryChanged
			}
			var readErr error
			if batching.filer, batching.units, readErr = batching.readInBatch(queries); readErr != nil {
				return fmt.Errorf("read it again in the batch: %w", readErr)
			}
		}
		changedAtStart, countErr := queries.CountRowsChangedByTheWriter(ctx)
		if countErr != nil {
			return fmt.Errorf("count the rows changed: %w", countErr)
		}
		batching.filer.queries = queries
		rest, identifyErr := batching.filer.identifyUnits(ctx, batching.units, changedAtStart, pass.batchRows)
		if identifyErr != nil {
			return identifyErr
		}
		batch, batching.units = batching.units[:len(batching.units)-len(rest)], rest
		changedAtEnd, countErr := queries.CountRowsChangedByTheWriter(ctx)
		batchRows = changedAtEnd - changedAtStart
		batching.expected = pass.database.Commits() + 1 // this one's, once it commits
		return countErr                                 //nolint:wrapcheck // the count read just above says the same
	})
	return batch, batchRows, err //nolint:wrapcheck // writeBatches says it was a batch
}

// isWritten is whether every file of a unit was in a unit a batch wrote.
func (unit filingUnit) isWritten(written map[int64]bool) bool {
	return !slices.ContainsFunc(unit.files, func(file sqlcgen.ListIdentifyFilesRow) bool { return !written[file.ID] })
}
