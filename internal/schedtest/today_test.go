// SPDX-License-Identifier: AGPL-3.0-or-later

package schedtest

import (
	"testing"
	"time"
)

// The 2026-10-21 failure (the queue inventory §4), in today's work list: a film plays from the NAS,
// its read-through raised to 1000 by interest; the share drops at 1:00 and a scan's listing at 20 is
// asked for; the share returns at 1:55 and answers at 2:43. The read-through, play-driven, holds the
// disk's one reader and reads the whole film first, so the listing waits long after the disk answers.
func TestToday_AListingWaitsBehindAPlayRaisedReadThroughOnceTheShareAnswers(t *testing.T) {
	t.Parallel()
	var sim Sim
	disk := NewDisk(&sim, 110*megabyte, 10*time.Millisecond)
	today := NewToday(&sim, disk)
	const filmBytesPerSecond = 2.5 * megabyte // 20 Mb/s
	film := NewPlayback(&sim, today.ReadForPlay("film"), filmBytesPerSecond, 2*time.Hour)
	readThrough := &Job{
		Name: "read-through", Bytes: int64(filmBytesPerSecond * 7200), Piece: 8 * megabyte,
		Priority: playDriven,
	}
	listing := &Job{Name: "listing", Bytes: 20 * megabyte, Piece: 64_000, Priority: 20}
	film.Play()
	today.Submit(readThrough)
	sim.At(time.Minute, disk.Die)
	sim.At(65*time.Second, func() { today.Submit(listing) })
	sim.At(115*time.Second, func() { disk.Revive(48 * time.Second) })
	sim.RunUntil(2 * time.Hour)

	answering := 163 * time.Second
	waitedAfterAnswering := listing.StartedAt - answering
	if !listing.HasStarted || waitedAfterAnswering < time.Minute {
		t.Errorf("the listing started %v after the share answered; today's work list makes it wait for the "+
			"read-through, over a minute", waitedAfterAnswering)
	}
	t.Logf("today: the listing waited %v after the share answered and ended at %v; the film stalled %d times for %v",
		waitedAfterAnswering, listing.FinishedAt, film.Stalls, film.Stalled)
}

// A background read-through on today's work list holds the disk's one reader, so a listing a person
// asked for at 20 waits for its whole film: a running job is never displaced (the inventory §3.8).
func TestToday_ARunningBackgroundJobIsNeverDisplacedForAHigherOne(t *testing.T) {
	t.Parallel()
	var sim Sim
	disk := NewDisk(&sim, 110*megabyte, 10*time.Millisecond)
	today := NewToday(&sim, disk)
	readThrough := &Job{Name: "read-through", Bytes: 11 * 1000 * megabyte, Piece: 8 * megabyte, Priority: 0}
	listing := &Job{Name: "listing", Bytes: 20 * megabyte, Piece: 64_000, Priority: 20}
	today.Submit(readThrough)
	sim.At(time.Second, func() { today.Submit(listing) })
	sim.RunUntil(time.Hour)
	if listing.StartedAt < readThrough.FinishedAt {
		t.Errorf("the listing started at %v, before the read-through ended at %v; today it waits for it",
			listing.StartedAt, readThrough.FinishedAt)
	}
}
