// SPDX-License-Identifier: AGPL-3.0-or-later

package readthrough

import (
	"testing"

	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// What a read-through kept of one track or picture whose probe's facts changed is thrown out, both of
// a track's mixes and nothing of another track, so the sweep reads it again; a second time finds
// nothing (D392 §3).
func TestForget_ThrowsOutOneTracksLoudnessAndOnePicturesTimeline(t *testing.T) {
	t.Parallel()
	database := storetest.OpenMigrated(t)
	ctx, hash := t.Context(), []byte{7}
	err := database.Write(ctx, func(queries *sqlcgen.Queries) error {
		for _, kept := range []sqlcgen.UpsertLoudnessParams{
			{PayloadHash: hash, StreamIndex: 1, MeasuredMix: "stereo", IntegratedLUFS: new(-20.0), MeasuredAt: 1},
			{PayloadHash: hash, StreamIndex: 1, MeasuredMix: "multichannel", IntegratedLUFS: new(-21.0), MeasuredAt: 1},
			{PayloadHash: hash, StreamIndex: 2, MeasuredMix: "stereo", IntegratedLUFS: new(-22.0), MeasuredAt: 1},
		} {
			if err := queries.UpsertLoudness(ctx, kept); err != nil {
				return err
			}
		}
		return queries.UpsertVideoTimeline(ctx, sqlcgen.UpsertVideoTimelineParams{
			PayloadHash: hash, VideoStreamIndex: 0, KeyframesMsJSON: "[0]", BytesInEachSecondJSON: "[]", MadeAt: 1,
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	forget := func(drop library.DropInWrite, streamIndex int) bool {
		t.Helper()
		var isDropped bool
		of := library.DerivedOf{FileID: 3, PayloadHash: hash, StreamIndex: streamIndex}
		writeErr := database.Write(ctx, func(queries *sqlcgen.Queries) error {
			var dropErr error
			isDropped, dropErr = drop(ctx, queries, of)
			return dropErr
		})
		if writeErr != nil {
			t.Fatal(writeErr)
		}
		return isDropped
	}
	if !forget(ForgetLoudness, 1) || !forget(ForgetTimeline, 0) {
		t.Fatal("a kept measurement or timeline was not thrown out")
	}
	measured, err := database.Read().ListLoudnessOfPayload(ctx, hash)
	if err != nil || len(measured) != 1 || measured[0].StreamIndex != 2 {
		t.Errorf("the measurements left = %+v, %v; want the other track's alone", measured, err)
	}
	if _, err := database.Read().GetVideoTimeline(ctx, sqlcgen.GetVideoTimelineParams{
		PayloadHash: hash, VideoStreamIndex: 0,
	}); err == nil {
		t.Error("the picture's timeline is still kept")
	}
	if forget(ForgetLoudness, 1) || forget(ForgetTimeline, 0) {
		t.Error("a second throwing out found something to throw out")
	}
}
