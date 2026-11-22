// SPDX-License-Identifier: AGPL-3.0-or-later

package loudness

import (
	"context"
	"strconv"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/decide"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// aimAt is the settings of a level of -23 LUFS, raised by at most 20 dB.
type aimAt struct{}

func (aimAt) Int(_ context.Context, key settings.Key) (int64, error) {
	if key == settings.LoudnessTargetLUFS {
		return -23, nil
	}
	return 20, nil
}

// A sound is levelled by what this recipe measured of it; one another recipe alone measured levels
// nothing, as one not measured, until the read-through measures it again (D536 §3). Another recipe is
// this one's next here, which stands for any other: the schema holds no recipe below 1.
func TestLevels_AMeasurementOfAnotherRecipeLevelsNothing(t *testing.T) {
	t.Parallel()
	database := storetest.OpenMigrated(t)
	this, another := strconv.Itoa(nodeproto.LoudnessRecipe), strconv.Itoa(nodeproto.LoudnessRecipe+1)
	storetest.ExecRaw(t, database.Path(), `INSERT INTO loudness (payload_hash, stream_index, measured_mix, recipe,
		integrated_lufs, range_lu, true_peak_dbtp, measured_at)
		VALUES (X'04', 1, 'stereo', `+this+`, -30, 5, -10, 1), (X'04', 2, 'stereo', `+another+`, -30, 5, -10, 1)`)
	service := NewService(Parts{Database: database, Settings: aimAt{}})
	levels, err := service.Levels(t.Context(), []byte{0x04})
	if err != nil {
		t.Fatal(err)
	}
	want := decide.Levelling{IsMeasured: true, GainDB: 7}
	if len(levels) != 1 || levels[decide.SoundKey{StreamIndex: 1, Mix: decide.MixStereo}] != want {
		t.Errorf("the levels are %+v; want track 1's alone, %+v: track 2 was measured by another recipe", levels, want)
	}
}
