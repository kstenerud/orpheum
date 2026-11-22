// SPDX-License-Identifier: AGPL-3.0-or-later

package repeats

import (
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// D535 §6: a discarding rescan throws out what was measured of a video's bytes, at every version,
// and nothing measured of another video's.
func TestForgetMeasurements_ThrowsOutEveryVersionOfOneVideosMeasurement(t *testing.T) {
	t.Parallel()
	database := storetest.OpenMigrated(t)
	storetest.ExecRaw(t, database.Path(),
		`INSERT INTO repeat_measurement (payload_hash, version, outcome, measured_at) VALUES
		 (X'aa', 1, 'none', 1), (X'aa', 2, 'found', 2), (X'bb', 2, 'found', 2)`)
	video := library.DerivedOf{FileID: 1, PayloadHash: []byte{0xaa}}
	var isForgotten, isForgottenAgain bool
	err := database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
		var err error
		if isForgotten, err = ForgetMeasurements(t.Context(), queries, video); err != nil {
			return err
		}
		isForgottenAgain, err = ForgetMeasurements(t.Context(), queries, video)
		return err
	})
	if err != nil || !isForgotten || isForgottenAgain {
		t.Fatalf("forgetting = %v then %v, %v; want it forgotten, then nothing left to forget", isForgotten,
			isForgottenAgain, err)
	}
	left := storetest.QueryRaw(t, database.Path(), `SELECT hex(payload_hash) FROM repeat_measurement`)
	if !slices.Equal(left, []string{"BB"}) {
		t.Errorf("measurements left %v; want only the other video's", left)
	}
}
