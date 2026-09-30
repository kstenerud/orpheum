// SPDX-License-Identifier: GPL-3.0-or-later

package settings

import (
	"context"
	"fmt"

	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// SeedReport is what a seed did, and what it found and left alone.
type SeedReport struct {
	// Inserted are the settings that had no row and now have their starting value.
	Inserted []Key
	// Updated are settings nobody changed whose starting value this release changed: they now
	// have the new one. A setting a person set is never updated (D201).
	Updated []Key
	// Unused are rows no setting reads. They are reported, never deleted (D202).
	Unused []UnusedSetting
}

// Seed writes the starting value of every server setting that has no row: all of them on a new
// server, on an upgrade the keys the new release introduced, and any row that was removed. A row
// nobody changed follows its starting value, so a release that changes one — to fix a bad
// choice — moves it; a row a person set, even to the starting value, is never touched. It also
// reports the rows nothing reads. The server seeds at every start, before anything reads a setting
// (D201, D202).
func (service *Service) Seed(ctx context.Context) (SeedReport, error) {
	report := SeedReport{Inserted: []Key{}, Updated: []Key{}, Unused: []UnusedSetting{}}
	setAt := service.clock.Now().UnixMilli()
	err := service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		rows, err := queries.ListAllSettings(ctx)
		if err != nil {
			return fmt.Errorf("list setting keys: %w", err)
		}
		report.Unused = unusedIn(rows)
		isPresentByKey := map[string]bool{}
		for _, row := range rows {
			if row.Scope == string(ScopeServer) {
				isPresentByKey[row.Key] = true
			}
		}
		for _, definition := range Registry() {
			if definition.Scope != ScopeServer {
				continue
			}
			seedErr := seedOne(ctx, queries, definition, isPresentByKey[string(definition.Key)], setAt, &report)
			if seedErr != nil {
				return seedErr
			}
		}
		return nil
	})
	return report, err
}

// seedOne writes a missing setting's starting value, or moves an unmodified one to it.
func seedOne(
	ctx context.Context, queries *sqlcgen.Queries, definition Definition, isPresent bool, setAt int64,
	report *SeedReport,
) error {
	start := compactJSON(definition.StartJSON)
	if !isPresent {
		err := queries.InsertServerSettingIfMissing(ctx, sqlcgen.InsertServerSettingIfMissingParams{
			Key: string(definition.Key), ValueJSON: start, SetAt: setAt,
		})
		if err != nil {
			return fmt.Errorf("seed %s: %w", definition.Key, err)
		}
		report.Inserted = append(report.Inserted, definition.Key)
		return nil
	}
	updatedCount, err := queries.UpdateUnmodifiedServerSetting(ctx, sqlcgen.UpdateUnmodifiedServerSettingParams{
		Key: string(definition.Key), ValueJSON: start, SetAt: setAt,
	})
	if err != nil {
		return fmt.Errorf("update %s to its starting value: %w", definition.Key, err)
	}
	if updatedCount > 0 {
		report.Updated = append(report.Updated, definition.Key)
	}
	return nil
}
