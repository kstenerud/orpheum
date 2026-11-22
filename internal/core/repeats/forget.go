// SPDX-License-Identifier: AGPL-3.0-or-later

package repeats

import (
	"context"
	"fmt"

	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// ForgetFailuresOfFiles forgets, in a rescan's write (identify.GivenUpForgetters, D535 §8), the failed
// measurements of the bytes of the files named, a JSON array of ids, at every version: their seasons
// are measured again. One that found repeats, or found none, is an answer and stays. It says how many
// it forgot.
func ForgetFailuresOfFiles(ctx context.Context, queries *sqlcgen.Queries, fileIDsJSON string) (int64, error) {
	forgotten, err := queries.ForgetFailedRepeatMeasurementsOfFiles(ctx, fileIDsJSON)
	if err != nil {
		return 0, fmt.Errorf("forget the failed repeat measurements: %w", err)
	}
	return forgotten, nil
}

// ForgetMeasurements throws out what was measured of a video's bytes to find its repeats, at every
// version, once a sound's facts it was heard by differ in a probe read again or a rescan discards what
// was made (library.DerivedDroppers, D392 §3, D535 §6), in the ingest's write that keeps the probe:
// its season is measured again, its repeats found again. It says whether there was any.
func ForgetMeasurements(ctx context.Context, queries *sqlcgen.Queries, of library.DerivedOf) (bool, error) {
	deleted, err := queries.DeleteRepeatMeasurements(ctx, of.PayloadHash)
	if err != nil {
		return false, fmt.Errorf("forget what was measured of file %d's repeats: %w", of.FileID, err)
	}
	return deleted > 0, nil
}
