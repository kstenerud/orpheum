// SPDX-License-Identifier: AGPL-3.0-or-later

package store_test

import (
	"errors"
	"testing"

	"github.com/kstenerud/orpheum/internal/platform/querytally"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// A derivation that finds nothing to change runs once, in a read transaction, and never takes the
// writer; one that writes runs again on the writer, and its write is kept; any other failure is
// returned from the read, not retried (D303, DL9, DL11).
func TestWriteUnlessUnchanged_TakesTheWriterOnlyForADerivationThatWrites(t *testing.T) {
	t.Parallel()
	database := storetest.OpenMigrated(t)
	ctx, tally := querytally.New(t.Context())
	runs := 0
	wrote, err := database.WriteUnlessUnchanged(ctx, func(queries *sqlcgen.Queries) error {
		runs++
		_, readErr := queries.ListLiveLibraryIDs(ctx)
		return readErr //nolint:wrapcheck // the test reads it as it is
	})
	if err != nil || wrote || runs != 1 || tally.Writes() != 0 {
		t.Errorf("a derivation that only reads: wrote %v, %v, ran %d times, %d write transactions; want one read run",
			wrote, err, runs, tally.Writes())
	}

	runs = 0
	var itemID int64
	wrote, err = database.WriteUnlessUnchanged(ctx, func(queries *sqlcgen.Queries) error {
		runs++
		var insertErr error
		itemID, insertErr = queries.InsertItem(ctx, sqlcgen.InsertItemParams{Kind: "title", AddedAt: 1})
		return insertErr //nolint:wrapcheck // the test reads it as it is
	})
	if err != nil || !wrote || runs != 2 || tally.Writes() != 1 {
		t.Errorf("a derivation that writes: wrote %v, %v, ran %d times, %d write transactions; want a read run "+
			"refused, then one write", wrote, err, runs, tally.Writes())
	}
	if kind, kindErr := database.Read().GetItemKind(ctx, itemID); kindErr != nil || kind != "title" {
		t.Errorf("the derivation's item %d is %q, %v; want it kept", itemID, kind, kindErr)
	}

	failure := errors.New("the derivation failed")
	runs = 0
	_, err = database.WriteUnlessUnchanged(ctx, func(*sqlcgen.Queries) error {
		runs++
		return failure
	})
	if !errors.Is(err, failure) || runs != 1 {
		t.Errorf("a failing derivation: %v after %d runs; want its failure, from the one read run", err, runs)
	}
}
