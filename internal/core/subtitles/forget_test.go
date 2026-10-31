// SPDX-License-Identifier: AGPL-3.0-or-later

package subtitles

import (
	"testing"

	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// The text made of a video's own subtitle track whose probe's facts changed is thrown out, and no
// other track's, so the next play makes it again; a second time finds nothing (D392 §3).
func TestForgetText_ThrowsOutOneTracksText(t *testing.T) {
	t.Parallel()
	database := storetest.OpenMigrated(t)
	ctx, hash := t.Context(), []byte{7}
	err := database.Write(ctx, func(queries *sqlcgen.Queries) error {
		for _, streamIndex := range []int64{3, 4} {
			err := queries.KeepSubtitleText(ctx, sqlcgen.KeepSubtitleTextParams{
				SourceHash: hash, StreamIndex: streamIndex, ASS: []byte("a"), VTT: []byte("v"), MadeAt: 1,
			})
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	forget := func() bool {
		t.Helper()
		var isDropped bool
		writeErr := database.Write(ctx, func(queries *sqlcgen.Queries) error {
			var dropErr error
			isDropped, dropErr = ForgetText(ctx, queries,
				library.DerivedOf{FileID: 3, PayloadHash: hash, StreamIndex: 3})
			return dropErr
		})
		if writeErr != nil {
			t.Fatal(writeErr)
		}
		return isDropped
	}
	if !forget() {
		t.Fatal("the track's text was not thrown out")
	}
	queries := database.Read()
	textOf := func(streamIndex int64) error {
		_, err := queries.GetSubtitleText(ctx,
			sqlcgen.GetSubtitleTextParams{SourceHash: hash, StreamIndex: streamIndex})
		return err
	}
	if textOf(3) == nil {
		t.Error("the track's text is still kept")
	}
	if err := textOf(4); err != nil {
		t.Errorf("the other track's text: %v; want it kept", err)
	}
	if forget() {
		t.Error("a second throwing out found something to throw out")
	}
}
