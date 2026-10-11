// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"context"
	"errors"
	"fmt"
)

// ErrStepChanged is a database where a step was applied as another text than this binary
// carries: its schema is not the step's. A committed step never changes, so it is a step edited
// after it was applied and before it was committed (plan-deviations P17).
var ErrStepChanged = errors.New("a migration step was applied as another text than this binary carries")

// ErrBeforeBaseline is a database at a version below this binary's baseline: the steps that
// would bring it there were folded into the baseline, which builds only a new file (D228).
var ErrBeforeBaseline = errors.New("this database is older than the baseline schema this binary starts from")

// RefuseBeforeBaseline refuses a file this binary's steps cannot bring forward: one between empty
// and the baseline's version (D228). A restore asks it of a backup before replacing anything.
func RefuseBeforeBaseline(state SchemaState) error {
	steps, err := Steps()
	if err != nil {
		return err
	}
	return beforeBaseline(steps, state)
}

// refuseBeforeBaseline refuses to migrate a file between empty and the baseline's version, and
// says how to start again.
func refuseBeforeBaseline(steps []Step, state SchemaState) error {
	err := beforeBaseline(steps, state)
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w. Move this file aside and start again with an empty one; a development database is "+
		"reset with `make dev-reset`, and the next `make dev` builds and seeds a new one", err)
}

// beforeBaseline describes a file between empty and the baseline's version, or returns nil.
func beforeBaseline(steps []Step, state SchemaState) error {
	if len(steps) == 0 || !steps[0].IsBaseline || state.Version == 0 || state.Version >= steps[0].Version {
		return nil
	}
	baseline := steps[0]
	return fmt.Errorf("%w: it is at schema %d, and %s builds schema %d only in a new file, having folded the "+
		"steps between (D228)", ErrBeforeBaseline, state.Version, baseline.Name, baseline.Version)
}

// checkAppliedSteps refuses a database where an applied step's recorded checksum differs from
// the step this binary carries. At a baseline's version, the checksum of the step it folded is
// as good as its own: a file that reached that version through the step is as current.
func (database *Database) checkAppliedSteps(ctx context.Context, steps []Step, appliedVersion int) error {
	if appliedVersion == 0 || len(steps) == 0 {
		return nil // a new file has no schema_step yet
	}
	carried := map[int]Step{}
	for _, step := range steps {
		carried[step.Version] = step
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
		step, isCarried := carried[version]
		if !isCarried || version > appliedVersion {
			continue // a step a baseline folded, or a newer binary's, which refuseNewer has judged
		}
		if checksum != step.Checksum && (!step.IsBaseline || checksum != step.FoldedChecksum) {
			return fmt.Errorf("%w: %s (the database has %.12s…, this binary %.12s…); a development "+
				"database is reset with `make dev-reset`", ErrStepChanged, step.Name, checksum, step.Checksum)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read the applied steps' checksums: %w", err)
	}
	return nil
}
