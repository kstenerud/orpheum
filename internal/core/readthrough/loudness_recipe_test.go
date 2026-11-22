// SPDX-License-Identifier: AGPL-3.0-or-later

package readthrough

import (
	"testing"

	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// The loudness table's key, a track's measured mix of a file's bytes, is held by its schema to
// 'multichannel' or 'stereo', so it has no place for the recipe that measured it: a raised
// nodeproto.LoudnessRecipe would keep every measurement an earlier recipe made as its own, and the
// sweep would measure nothing again. The recipe stays at its first version until the key holds it,
// which is a migration and the owner's (D536 §3).
func TestLoudnessRecipe_IsOneUntilTheLoudnessKeyHoldsIt(t *testing.T) {
	t.Parallel()
	if recipe := nodeproto.LoudnessRecipe; recipe != 1 {
		t.Errorf("nodeproto.LoudnessRecipe is %d; the loudness table's key holds no recipe, so a measurement "+
			"an earlier recipe made would be kept as this one's. Give the key its recipe first (D536 §3)", recipe)
	}
}
