// SPDX-License-Identifier: GPL-3.0-or-later

package settings

import (
	"context"
	"fmt"

	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// SeedReport is what a seed did, and what it found and left alone.
type SeedReport struct {
	// Written are the settings that had no row and now have their starting value.
	Written []Key
	// Updated are settings nobody changed whose starting value this release changed: they now
	// have the new one. A setting a person set is never updated (D200).
	Updated []Key
	// Unknown are rows no setting in the registry reads: a typo, or a key a release removed.
	// They are reported, never deleted; removing them is the person's choice.
	Unknown []UnknownSetting
}

// UnknownSetting is a setting row the registry does not declare.
type UnknownSetting struct {
	Scope  Scope
	UserID *int64
	Key    string
}

func (unknown UnknownSetting) String() string {
	if unknown.UserID != nil {
		return fmt.Sprintf("%s (user %d)", unknown.Key, *unknown.UserID)
	}
	return unknown.Key
}

// Seed writes the starting value of every server setting that has no row: all of them on a new
// server, on an upgrade the keys the new release introduced, and any row that was removed. A row
// nobody changed follows its starting value, so a release that changes one — to fix a bad
// choice — moves it; a row a person set, even to the starting value, is never touched. It also
// reports the rows nothing reads. The server seeds at every start; a person can also seed with
// `orpheum seed-settings` or POST /config/seed.
func (service *Service) Seed(ctx context.Context) (SeedReport, error) {
	report := SeedReport{Written: []Key{}, Updated: []Key{}, Unknown: []UnknownSetting{}}
	now := service.clock.Now().UnixMilli()
	err := service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		rows, err := queries.ListSettingKeys(ctx)
		if err != nil {
			return fmt.Errorf("list setting keys: %w", err)
		}
		report.Unknown = unknownIn(rows)
		present := map[string]bool{}
		for _, row := range rows {
			if row.Scope == string(ScopeServer) {
				present[row.Key] = true
			}
		}
		for _, definition := range Registry() {
			if definition.Scope != ScopeServer {
				continue
			}
			seedErr := seedOne(ctx, queries, definition, present[string(definition.Key)], now, &report)
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
	ctx context.Context, queries *sqlcgen.Queries, definition Definition, isPresent bool, now int64,
	report *SeedReport,
) error {
	start := compact(definition.Start)
	if !isPresent {
		err := queries.InsertServerSettingIfMissing(ctx, sqlcgen.InsertServerSettingIfMissingParams{
			Key: string(definition.Key), Value: start, SetAt: now,
		})
		if err != nil {
			return fmt.Errorf("seed %s: %w", definition.Key, err)
		}
		report.Written = append(report.Written, definition.Key)
		return nil
	}
	updated, err := queries.UpdateUnmodifiedServerSetting(ctx, sqlcgen.UpdateUnmodifiedServerSettingParams{
		Key: string(definition.Key), Value: start, SetAt: now,
	})
	if err != nil {
		return fmt.Errorf("update %s to its starting value: %w", definition.Key, err)
	}
	if updated > 0 {
		report.Updated = append(report.Updated, definition.Key)
	}
	return nil
}

// Unknown lists the setting rows no setting in the registry reads, without writing anything.
func (service *Service) Unknown(ctx context.Context) ([]UnknownSetting, error) {
	rows, err := service.database.Read().ListSettingKeys(ctx)
	if err != nil {
		return nil, fmt.Errorf("list setting keys: %w", err)
	}
	return unknownIn(rows), nil
}

func unknownIn(rows []sqlcgen.ListSettingKeysRow) []UnknownSetting {
	unknown := []UnknownSetting{}
	for _, row := range rows {
		if _, known := Lookup(Key(row.Key)); !known {
			unknown = append(unknown, UnknownSetting{Scope: Scope(row.Scope), UserID: row.UserID, Key: row.Key})
		}
	}
	return unknown
}
