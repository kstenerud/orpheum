// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"context"
	"testing"

	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// writerChanges is how many rows the writer has changed since it opened, triggers' included.
func writerChanges(ctx context.Context, t *testing.T, database *Database) int {
	t.Helper()
	var changes int
	if err := database.writer.QueryRowContext(ctx, "SELECT total_changes()").Scan(&changes); err != nil {
		t.Fatal(err)
	}
	return changes
}

// A text written again as it is, and a fold set to what it is, change nothing (D303, DL9).
func TestTextWrites_LeaveAnUnchangedTextAlone(t *testing.T) {
	t.Parallel()
	database := openMigrated(t)
	ctx := t.Context()
	execRaw(t, database,
		"INSERT INTO item (id, kind, added_at, identification_state) VALUES (7, 'movie', 1, 'identified')")
	text := sqlcgen.UpsertOriginalTextParams{
		ItemID: 7, Language: "und", Name: "The General", Overview: new("A planet."), SearchAlternates: new("solaris"),
	}
	write := func(change func(*sqlcgen.Queries) error) int {
		t.Helper()
		before := writerChanges(ctx, t, database)
		if err := database.Write(ctx, change); err != nil {
			t.Fatal(err)
		}
		return writerChanges(ctx, t, database) - before
	}
	upsert := func(queries *sqlcgen.Queries) error { return queries.UpsertOriginalText(ctx, text) }
	first, again := write(upsert), write(upsert)
	fold := write(func(queries *sqlcgen.Queries) error {
		return queries.SetSearchAlternates(ctx, sqlcgen.SetSearchAlternatesParams{
			ItemID: 7, SearchAlternates: new("solaris"),
		})
	})
	if first == 0 || again != 0 || fold != 0 {
		t.Errorf("the first write changed %d rows, the same again %d, the same fold %d; want some, then none, "+
			"then none", first, again, fold)
	}
}
