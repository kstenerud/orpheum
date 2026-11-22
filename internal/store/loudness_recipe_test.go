// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"fmt"
	"testing"
)

// Step 0100 gives the loudness key its recipe (D536 §3): every measurement made before it is of
// recipe 1, the first, kept as it was, so the deploy that brings the step measures nothing again; and
// it raises the compatible version, so an older binary refuses the file.
func TestMigrate_AMeasurementMadeBeforeTheLoudnessKeyHeldItsRecipeIsRecipeOne(t *testing.T) {
	t.Parallel()
	steps, err := Steps()
	if err != nil {
		t.Fatal(err)
	}
	var before []Step
	for _, step := range steps {
		if step.Version < 100 {
			before = append(before, step)
		}
	}
	database := openTemporary(t)
	if err := database.migrate(t.Context(), before, testMigrateOptions(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	execRaw(t, database, `INSERT INTO loudness (payload_hash, stream_index, measured_mix, integrated_lufs, range_lu,
		true_peak_dbtp, failure, measured_at) VALUES (X'04', 1, 'stereo', -20.5, 6, -1.5, NULL, 7),
		(X'04', 1, 'multichannel', -21.5, 7, NULL, 'ffmpeg exited 1', 8)`)
	if err := database.migrate(t.Context(), steps, testMigrateOptions(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	rows := queryRows(t, database, `SELECT payload_hash, stream_index, measured_mix, recipe,
		quote(integrated_lufs), quote(range_lu), quote(true_peak_dbtp), quote(failure), measured_at
		FROM loudness ORDER BY measured_mix`, 9)
	want := "[[\x04 1 multichannel 1 -21.5 7.0 NULL 'ffmpeg exited 1' 8] [\x04 1 stereo 1 -20.5 6.0 -1.5 NULL 7]]"
	if got := fmt.Sprint(rows); got != want {
		t.Errorf("after step 0100 the measurements are %q; want %q, each kept as recipe 1", got, want)
	}
	// An older binary writes a measurement by the key it knew, which is no longer one.
	if state, err := database.State(t.Context()); err != nil || state.CompatibleVersion < 100 {
		t.Errorf("after step 0100, state = %+v (%v); want compatible_version 100 at least", state, err)
	}
}
