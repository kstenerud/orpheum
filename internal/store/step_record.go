// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"errors"
	"fmt"
)

// stepRecordVersion is the step that creates schema_step; before it, nothing is recorded.
const stepRecordVersion = 9

// ErrStepChanged is a database where a step was applied as another text than this binary
// carries: its schema is not the step's. A committed step never changes, so it is a step edited
// after it was applied and before it was committed (plan-deviations P17).
var ErrStepChanged = errors.New("a migration step was applied as another text than this binary carries")

// checkAppliedSteps refuses a database where an applied step's recorded checksum differs from
// the step this binary carries.
func (database *Database) checkAppliedSteps(ctx context.Context, steps []Step, applied int) error {
	if applied < stepRecordVersion {
		return nil
	}
	rows, err := database.readers.QueryContext(ctx, "SELECT version, sha256 FROM schema_step ORDER BY version")
	if err != nil {
		return fmt.Errorf("read the applied steps' checksums: %w", err)
	}
	defer func() { _ = rows.Close() }() //nolint:errcheck // a read; rows.Err is checked
	for rows.Next() {
		var version int
		var checksum string
		if err := rows.Scan(&version, &checksum); err != nil {
			return fmt.Errorf("read an applied step's checksum: %w", err)
		}
		if version < 1 || version > len(steps) || version > applied {
			continue // a newer binary's step, which refuseNewer has judged
		}
		if step := steps[version-1]; step.Checksum != checksum {
			return fmt.Errorf("%w: %s (the database has %.12s…, this binary %.12s…); a development "+
				"database is reset with `make dev-reset`", ErrStepChanged, step.Name, checksum, step.Checksum)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read the applied steps' checksums: %w", err)
	}
	return nil
}

// recordAppliedSteps records the checksum of every applied step that has none: each new one,
// and, when schema_step is first created, the steps before it, with the text this binary carries.
func (database *Database) recordAppliedSteps(ctx context.Context, steps []Step, applied int) error {
	if applied < stepRecordVersion {
		return nil
	}
	// A database a newer release migrated has steps this binary does not carry; it records its own.
	for _, step := range steps[:min(applied, len(steps))] {
		_, err := database.writer.ExecContext(ctx,
			"INSERT INTO schema_step (version, sha256) VALUES (?, ?) ON CONFLICT (version) DO NOTHING",
			step.Version, step.Checksum)
		if err != nil {
			return fmt.Errorf("record %s's checksum: %w", step.Name, err)
		}
	}
	return nil
}
