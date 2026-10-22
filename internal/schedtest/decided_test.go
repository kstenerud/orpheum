// SPDX-License-Identifier: AGPL-3.0-or-later

package schedtest

import (
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/node/diskschedule"
)

// nasSettings are a NAS share's on a 1 Gb/s link: one read out, background up to the link's speed
// and never below a tenth of it, and a playback read slow past half a second.
func nasSettings() diskschedule.Settings {
	return diskschedule.Settings{
		Limit: 1, BackgroundBytesPerSecond: 110 * megabyte, BackgroundFloorBytesPerSecond: 11 * megabyte,
		SlowPlaybackRead: 500 * time.Millisecond,
	}
}

// The 2026-10-21 failure (TestToday_AListingWaitsBehindAPlayRaisedReadThroughOnceTheShareAnswers),
// through the node's disk scheduler: the read-through is background work in pieces, the listing a
// person's, so the listing reads as soon as the share answers, and the film stalls no longer.
func TestDecided_AListingReadsAsSoonAsTheShareAnswers(t *testing.T) {
	t.Parallel()
	var sim Sim
	disk := NewDisk(&sim, 110*megabyte, 10*time.Millisecond)
	decided := NewDecided(&sim, disk, nasSettings())
	const filmBytesPerSecond = 2.5 * megabyte
	film := NewPlayback(&sim, decided.ReadForPlay("film"), filmBytesPerSecond, 2*time.Hour)
	readThrough := &Job{
		Name: "read-through", Bytes: int64(filmBytesPerSecond * 7200), Piece: 8 * megabyte,
		Tier: diskschedule.Background,
	}
	listing := &Job{Name: "listing", Bytes: 20 * megabyte, Piece: 64_000, Tier: diskschedule.Attended}
	film.Play()
	decided.Submit(readThrough)
	sim.At(time.Minute, disk.Die)
	sim.At(65*time.Second, func() { decided.Submit(listing) })
	sim.At(115*time.Second, func() { disk.Revive(48 * time.Second) })
	sim.RunUntil(2 * time.Hour)

	answering := 163 * time.Second
	if !listing.HasFinished || listing.FinishedAt-answering > 5*time.Second {
		t.Errorf("the listing ended %v after the share answered; want within 5s", listing.FinishedAt-answering)
	}
	if film.Stalls != 1 || film.Stalled > 75*time.Second {
		t.Errorf("the film stalled %d times for %v; want once, for no longer than the share was gone past its "+
			"buffer (about 73s)", film.Stalls, film.Stalled)
	}
	t.Logf("decided: the listing ended %v after the share answered; the film stalled %d times for %v; "+
		"the read-through ended at %v", listing.FinishedAt-answering, film.Stalls, film.Stalled, readThrough.FinishedAt)
}
