// SPDX-License-Identifier: AGPL-3.0-or-later

package artwork

import (
	"fmt"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/library/librarytest"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// The frame stills taken from a file whose probe's facts they were taken by changed are retired, so
// the next pass takes one again; another image of its item stays, and a second retirement finds none
// (D392 §3).
func TestRetireFrameStills_RetiresOnlyTheFilesFrameStills(t *testing.T) {
	t.Parallel()
	world := newFrameWorld(t, "movie", "tmdb")
	titleID, fileID := world.video(t, librarytest.TitleSpec{Kind: "movie"})
	world.store(t, titleID, sqlcgen.InsertArtworkParams{
		Kind: library.ArtStill, Source: SourceFrame, SourceRef: new(fmt.Sprintf("file:%d:5000:7", fileID)),
		SourceSha256: []byte{9},
	})
	world.store(t, titleID, sqlcgen.InsertArtworkParams{
		Kind: library.ArtPoster, Source: "tmdb", SourceRef: new("/poster.jpg"), SourceSha256: []byte{8},
	})
	retire := func() bool {
		t.Helper()
		var isRetired bool
		err := world.bench.database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
			var retireErr error
			isRetired, retireErr = world.bench.service.RetireFrameStills(t.Context(), queries,
				library.DerivedOf{FileID: fileID})
			return retireErr
		})
		if err != nil {
			t.Fatal(err)
		}
		return isRetired
	}
	if !retire() {
		t.Error("the file's frame still was not retired")
	}
	rows, err := world.bench.database.Read().ListLiveArtwork(t.Context(), titleID)
	if err != nil || len(rows) != 1 || rows[0].Source != "tmdb" {
		t.Errorf("the title's live images = %+v, %v; want its poster alone", rows, err)
	}
	if retire() {
		t.Error("a second retirement found a frame still to retire")
	}
}
