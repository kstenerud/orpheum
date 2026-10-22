// SPDX-License-Identifier: AGPL-3.0-or-later

package diskschedule

import (
	"testing"
	"time"
)

// start is any moment: the scheduler reads no clock of its own.
var start = time.Date(2026, 10, 22, 20, 0, 0, 0, time.UTC)

const megabyte = 1_000_000

func unlimited() Settings {
	return Settings{
		Limit: 1, BackgroundBytesPerSecond: 1e12, BackgroundFloorBytesPerSecond: 1e12,
		SlowPlaybackRead: time.Second,
	}
}

func next(t *testing.T, scheduler *Scheduler, now time.Time) Read {
	t.Helper()
	read, isSent, _ := scheduler.Next(now)
	if !isSent {
		t.Fatalf("no read went at %v", now)
	}
	return read
}

func TestScheduler_KeepsAtMostItsLimitOnTheDevice(t *testing.T) {
	t.Parallel()
	settings := unlimited()
	settings.Limit = 2
	scheduler := New(settings)
	for _, work := range []string{"a", "b", "c"} {
		scheduler.Add(Read{Work: work, Tier: Attended, Bytes: megabyte})
	}
	first, second := next(t, scheduler, start), next(t, scheduler, start)
	if _, isSent, _ := scheduler.Next(start); isSent {
		t.Fatal("a third read went with two out on a device whose limit is two")
	}
	scheduler.Done(start, first)
	if third := next(t, scheduler, start); first.Work != "a" || second.Work != "b" || third.Work != "c" {
		t.Errorf("sent %s, %s, then %s; want a, b, c", first.Work, second.Work, third.Work)
	}
}

func TestScheduler_OrdersByTierThenDeadlineThenArrival(t *testing.T) {
	t.Parallel()
	scheduler := New(unlimited())
	scheduler.Add(Read{Work: "background", Tier: Background, Bytes: 1})
	scheduler.Add(Read{Work: "attended, no deadline", Tier: Attended, Bytes: 1})
	scheduler.Add(Read{Work: "attended, late deadline", Tier: Attended, Deadline: start.Add(time.Hour), Bytes: 1})
	scheduler.Add(Read{Work: "attended, early deadline", Tier: Attended, Deadline: start.Add(time.Minute), Bytes: 1})
	scheduler.Add(Read{Work: "playback", Tier: Continuous, Bytes: 1})
	scheduler.Add(Read{Work: "attended, no deadline, later", Tier: Attended, Bytes: 1})
	want := []string{
		"playback", "attended, early deadline", "attended, late deadline", "attended, no deadline",
		"attended, no deadline, later", "background",
	}
	for _, work := range want {
		read := next(t, scheduler, start)
		scheduler.Done(start, read)
		if read.Work != work {
			t.Errorf("sent %q; want %q", read.Work, work)
		}
	}
}

// A long work asks one read at a time; a listing a person waits for, asked for meanwhile, goes before
// the long work's next piece instead of after its whole film.
func TestScheduler_AnAttendedReadGoesBeforeTheNextPieceOfBackgroundWork(t *testing.T) {
	t.Parallel()
	scheduler := New(unlimited())
	scheduler.Add(Read{Work: "read-through", Tier: Background, Bytes: 8 * megabyte})
	piece := next(t, scheduler, start)
	scheduler.Add(Read{Work: "listing", Tier: Attended, Bytes: 64_000})
	scheduler.Done(start, piece)
	scheduler.Add(Read{Work: "read-through", Tier: Background, Bytes: 8 * megabyte})
	if read := next(t, scheduler, start); read.Work != "listing" {
		t.Errorf("after the read-through's piece, sent %q; want the listing", read.Work)
	}
}

func TestScheduler_ASlowPlaybackReadHalvesTheBackgroundRate(t *testing.T) {
	t.Parallel()
	scheduler := New(Settings{
		Limit: 1, BackgroundBytesPerSecond: 100 * megabyte, BackgroundFloorBytesPerSecond: 10 * megabyte,
		SlowPlaybackRead: 200 * time.Millisecond,
	})
	scheduler.Add(Read{Work: "film", Tier: Continuous, Bytes: 5 * megabyte})
	playback := next(t, scheduler, start)
	scheduler.Done(start.Add(time.Second), playback) // slow: the background rate halves to 50 MB/s
	now := start.Add(time.Second)
	scheduler.Add(Read{Work: "read-through", Tier: Background, Bytes: 80 * megabyte})
	_, isSent, retryAt := scheduler.Next(now)
	// 50 MB in hand at 50 MB/s: 30 MB more takes 0.6 s.
	if isSent || retryAt.Sub(now) != 600*time.Millisecond {
		t.Errorf("an 80 MB background read sent %t, retry in %v; want held, retry in 600ms", isSent, retryAt.Sub(now))
	}
	if read := next(t, scheduler, now.Add(10*time.Second)); read.Work != "read-through" {
		t.Errorf("ten seconds on, sent %q; want the read-through, the rate climbed back", read.Work)
	}
}
