// SPDX-License-Identifier: GPL-3.0-or-later

package identify

import (
	"fmt"
	"testing"

	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// Every kind of artwork the code names is one the artwork table's CHECK accepts: a kind the
// schema renamed (thumb became still, D227 §6) would otherwise be refused only when an image
// arrives.
func TestArtKinds_AreTheOnesTheArtworkTableAccepts(t *testing.T) {
	t.Parallel()
	database := storetest.OpenMigrated(t)
	storetest.ExecRaw(t, database.Path(), "INSERT INTO item (id, kind, added_at) VALUES (1, 'title', 1)")
	for _, kind := range []string{ArtPoster, ArtBackdrop, ArtLogo, ArtStill} {
		storetest.ExecRaw(t, database.Path(),
			fmt.Sprintf("INSERT INTO artwork (item_id, kind, source) VALUES (1, '%s', 'tmdb')", kind))
	}
}
