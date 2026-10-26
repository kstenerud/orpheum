// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// A film its device cannot play as it is starts on its ladder, the one way to convert it, and its
// decision's address is its ladder's master playlist: not yet cut on its keyframes, it is cut on time,
// and its keyframes are read at play priority for its next start; cut, it starts on them (D260 §1,
// D344 §1, §2). Film 2, a Matroska file with TrueHD sound, cannot be played as it is by Chrome.
func TestPlayback_AFilmItsDeviceCannotPlayStartsOnItsLadderAtItsMasterPlaylist(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	master := func(started decision) string {
		return Prefix + "/bytes/media/chunks/" + started.MediaToken + "/master.m3u8"
	}
	before := harness.play(t, user, chrome(2))
	if before.Delivery != "hls" || before.OnLadder == nil || before.OnLadder.URL != master(before) ||
		before.URL != master(before) {
		t.Fatalf("film 2, not cut, started %+v; want it on its ladder, cut on time, at its master playlist", before)
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
	if after.OnLadder == nil || after.OnLadder.URL != master(after) || after.URL != master(after) {
		t.Errorf("film 2, cut, started %+v; want it on its ladder at its master playlist", after)
	}
	// The start says the share of the link its player chooses within (the constants sweep, 2026-10-20).
	if after.OnLadder != nil && after.OnLadder.LinkSharePercent != 70 {
		t.Errorf("the start's share of the link is %d%%; want the setting's 70", after.OnLadder.LinkSharePercent)
	}
}
