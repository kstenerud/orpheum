// SPDX-License-Identifier: AGPL-3.0-or-later

package artwork

import (
	"encoding/json"
	"fmt"
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// A frame still is named by the recipe it was taken by too: at the first, as a still was named
// before recipes were counted, so one taken then holds its video and none is taken again; at a
// raised recipe, a still an earlier one took holds nothing, and its video's still is taken again,
// named by the new recipe, retiring the old (D536).
func TestTake_AFrameStillTakenByAnEarlierRecipeIsTakenAgain(t *testing.T) {
	t.Parallel()
	world, stillFile := newHandleWorld(t)
	taken, takenFile := world.video(t, otherVideo)
	*stillFile = takenFile
	before := fmt.Sprintf("file:%d:5000:7", takenFile)
	if ref := frameStillRef(LocalImage{FileID: takenFile, SizeBytes: 5000, ModifiedAtMs: 7}, 1); ref != before {
		t.Errorf("a still at the first recipe is named %s; want it as before, %s", ref, before)
	}
	world.store(t, taken, sqlcgen.InsertArtworkParams{
		Kind: library.ArtStill, Source: SourceFrame, SourceRef: &before, SourceSha256: []byte{9},
	})
	if planned := world.pass(t, Found{}); len(planned) != 0 {
		t.Fatalf("planned %v at the first recipe; want nothing: the still taken before holds", planned)
	}
	world.bench.service.frameStillRecipe = 2
	if planned := world.pass(t, Found{}); !slices.Equal(planned, []int64{taken}) {
		t.Fatalf("planned %v at a raised recipe; want %d's still taken again", planned, taken)
	}
	var args artworkJobArgs
	if err := json.Unmarshal([]byte((*world.queue)[0].ArgsJSON), &args); err != nil {
		t.Fatal(err)
	}
	raised := before + ";recipe=2"
	if args.Refs[0].SourceRef != raised {
		t.Fatalf("the still asked again is named %s; want %s", args.Refs[0].SourceRef, raised)
	}
	world.bench.itemID = taken
	request := (*world.queue)[0]
	if err := world.bench.run(t, jobs.Job{
		ID: taken, Kind: request.Kind, Target: request.Target, ArgsJSON: request.ArgsJSON,
	}); err != nil {
		t.Fatal(err)
	}
	rows := world.bench.rows(t)
	if len(rows) != 1 || rows[0].SourceRef == nil || *rows[0].SourceRef != raised || rows[0].SourceSha256 == nil {
		t.Errorf("held %+v; want the still taken by the raised recipe alone, the earlier one's retired", rows)
	}
	if planned := world.pass(t, Found{}); len(planned) != 0 {
		t.Errorf("planned %v once the raised recipe's still was taken; want nothing", planned)
	}
}
