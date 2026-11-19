// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/querytally"
	"github.com/kstenerud/orpheum/internal/store/cachegen"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// A write prepares each statement that returns no rows once, however often it runs it, on either
// database, and the next write prepares its own; each run still counts in the tally.
func TestWrite_PreparesEachStatementThatReturnsNoRowsOnceAWrite(t *testing.T) {
	t.Parallel()
	database := openMigrated(t)
	cache := openCacheAt(t, t.TempDir()+"/cache.db")
	var kept []string
	database.onKeep = func(query string, _ *sql.Stmt) { kept = append(kept, query) }
	cache.database.onKeep = database.onKeep
	ctx, tally := querytally.New(t.Context())
	upsertFive := func(queries *sqlcgen.Queries) error {
		for index := range 5 {
			err := queries.UpsertSetting(ctx, sqlcgen.UpsertSettingParams{
				Key: fmt.Sprintf("probe-%d", index), ValueJSON: "1", SetAt: 1,
			})
			if err != nil {
				return fmt.Errorf("upsert setting %d: %w", index, err)
			}
		}
		return nil
	}
	for range 2 {
		if err := database.Write(ctx, upsertFive); err != nil {
			t.Fatal(err)
		}
	}
	err := cache.Write(ctx, func(queries *cachegen.Queries) error {
		for _, source := range []string{"ab", "cd", "ef"} {
			served := cachegen.MarkArtServedParams{SourceSha256Hex: source, LastServedEpochDay: 1}
			if err := queries.MarkArtServed(ctx, served); err != nil {
				return fmt.Errorf("mark %s served: %w", source, err)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(kept) != 3 || !strings.Contains(kept[0], "setting") || kept[1] != kept[0] ||
		!strings.Contains(kept[2], "art_served") {
		t.Errorf("the writes prepared %d statements: %q; want the setting upsert once a write and the art mark once",
			len(kept), kept)
	}
	if want := "UpsertSetting×10 MarkArtServed×3"; tally.String() != want {
		t.Errorf("tallied %q; want %q, every run counted", tally.String(), want)
	}
}

// Every statement a write kept is closed when the write ends: committed, rolled back, or ended by
// a panic, which database/sql's own close at the transaction's end would not reach.
func TestWrite_ClosesTheStatementsItKeptWhenItEnds(t *testing.T) {
	t.Parallel()
	database := openMigrated(t)
	var kept []*sql.Stmt
	database.onKeep = func(_ string, statement *sql.Stmt) { kept = append(kept, statement) }
	ctx := t.Context()
	upsert := func(queries *sqlcgen.Queries) error {
		return queries.UpsertSetting(ctx, sqlcgen.UpsertSettingParams{Key: "probe", ValueJSON: "1", SetAt: 1})
	}
	if err := database.Write(ctx, upsert); err != nil {
		t.Fatal(err)
	}
	refused := errors.New("refused")
	err := database.Write(ctx, func(queries *sqlcgen.Queries) error { return errors.Join(upsert(queries), refused) })
	if !errors.Is(err, refused) {
		t.Fatalf("the refused write returned %v; want its refusal", err)
	}
	transaction, err := database.writer.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := transaction.Rollback(); err != nil {
			t.Errorf("roll back the panicked write: %v", err)
		}
	})
	func() {
		defer func() {
			if recover() == nil {
				t.Error("the change did not panic")
			}
		}()
		err := database.keepingStatements(transaction, func(connection tallying) error {
			if err := upsert(sqlcgen.New(connection)); err != nil {
				t.Errorf("upsert before the panic: %v", err)
			}
			panic("the change panics")
		})
		t.Errorf("keepingStatements returned %v; want the change's panic", err)
	}()
	if len(kept) != 3 {
		t.Fatalf("the committed, rolled-back and panicked writes kept %d statements; want 1 each", len(kept))
	}
	for index, ending := range []string{"committed", "rolled back", "panicked"} {
		_, err := kept[index].ExecContext(ctx, "after", "1", 1)
		if err == nil || !strings.Contains(err.Error(), "statement is closed") {
			t.Errorf("the %s write's kept statement ran after it with %v; want it closed", ending, err)
		}
	}
}

// A write whose change panics is rolled back, so the next write has the writer: left open, the
// transaction held the one writer connection and the next write waited until its context ended.
func TestWrite_APanickingChangeLeavesTheWriterToTheNextWrite(t *testing.T) {
	t.Parallel()
	database := openMigrated(t)
	panicked := func() (value any) {
		defer func() { value = recover() }()
		half := sqlcgen.UpsertSettingParams{Key: "half", ValueJSON: "1", SetAt: 1}
		err := database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
			if err := queries.UpsertSetting(t.Context(), half); err != nil {
				return fmt.Errorf("upsert: %w", err)
			}
			panic("a change's bug")
		})
		t.Errorf("the panicking write returned %v; want its panic to go on", err)
		return nil
	}()
	if panicked != "a change's bug" {
		t.Errorf("the write's panic: %v; want the change's own", panicked)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	err := database.Write(ctx, func(queries *sqlcgen.Queries) error {
		return queries.UpsertSetting(ctx, sqlcgen.UpsertSettingParams{Key: "next", ValueJSON: "1", SetAt: 1})
	})
	if err != nil {
		t.Fatalf("the write after a panicking one: %v; want it to have the writer", err)
	}
}
