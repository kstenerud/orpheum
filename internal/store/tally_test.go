// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"fmt"
	"testing"

	"github.com/kstenerud/orpheum/internal/platform/querytally"
	"github.com/kstenerud/orpheum/internal/store/cachegen"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// Every way a statement reaches either database counts into its context's tally, by its query's
// name, and every write transaction counts as one (D303, DL6).
func TestTally_CountsEveryStatementByItsNameAndEveryWrite(t *testing.T) {
	t.Parallel()
	database := openMigrated(t)
	cache := openCacheAt(t, t.TempDir()+"/cache.db")
	ctx, tally := querytally.New(t.Context())
	err := database.Write(ctx, func(queries *sqlcgen.Queries) error {
		return queries.UpsertSetting(ctx, sqlcgen.UpsertSettingParams{Key: "probe", ValueJSON: "1", SetAt: 1})
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = database.Read().GetSetting(ctx, "probe"); err != nil {
		t.Fatal(err)
	}
	err = database.ReadTx(ctx, func(queries *sqlcgen.Queries) error {
		if _, readErr := queries.ListSettings(ctx); readErr != nil {
			return fmt.Errorf("list the settings: %w", readErr)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = database.Search(ctx, SearchQuery{
		Route: SearchRouteSubstring, IndexQuery: "x", Limit: 1,
		IsEveryLibrary: 1, LibraryIDsJSON: "[]",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = cache.Read().ListArtServed(ctx); err != nil {
		t.Fatal(err)
	}
	err = cache.Write(ctx, func(queries *cachegen.Queries) error {
		return queries.MarkArtServed(ctx, cachegen.MarkArtServedParams{SourceSha256Hex: "ab", LastServedEpochDay: 1})
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "GetSetting×1 ListArtServed×1 ListSettings×1 MarkArtServed×1 SearchSubstring×1 UpsertSetting×1"
	if got := tally.String(); got != want || tally.Writes() != 2 {
		t.Errorf("tallied %q and %d writes; want %q and 2", got, tally.Writes(), want)
	}
}
