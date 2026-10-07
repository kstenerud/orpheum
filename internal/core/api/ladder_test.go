// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// A film cut into chunks starts on its ladder, even one this browser plays only from a prepared copy,
// and no copy is prepared for it (D260 §1); not yet cut, it is prepared as before, and its keyframes
// are read at play priority for its next start. Film 2, a Matroska file with TrueHD sound, is
// prepared for Chrome.
func TestPlayback_ACutFilmStartsOnItsLadderAndPreparesNoCopy(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	before := harness.play(t, user, chrome(2))
	if before.OnLadder != nil || !before.IsPreparing || len(harness.openJobs(t, jobs.KindPrepare)) != 1 {
		t.Fatalf("film 2, not cut, started %+v; want it prepared as before", before)
	}
	reads := harness.openJobs(t, jobs.KindReadThrough)
	if len(reads) != 1 || reads[0].Priority != jobs.PriorityPlayNow {
		t.Errorf("the read-throughs open = %+v; want film 2's at play priority, for its ladder", reads)
	}
	err := harness.database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
		return queries.UpsertVideoTimeline(t.Context(), sqlcgen.UpsertVideoTimelineParams{
			PayloadHash: []byte{0x0c, 0x0d}, KeyframesMsJSON: "[0,10000,20000]",
			BytesInEachSecondJSON: "[" + strings.Repeat("300000,", 29) + "300000]",
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	after := harness.play(t, user, chrome(2))
	if after.OnLadder == nil || after.IsPreparing ||
		after.OnLadder.URL != Prefix+"/bytes/media/chunks/"+after.MediaToken+"/master.m3u8" {
		t.Errorf("film 2, cut, started %+v; want it on its ladder, nothing prepared", after)
	}
	if prepares := harness.openJobs(t, jobs.KindPrepare); len(prepares) != 1 {
		t.Errorf("the prepares open = %d; want only the first start's", len(prepares))
	}
}
