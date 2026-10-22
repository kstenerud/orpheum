// SPDX-License-Identifier: AGPL-3.0-or-later

package diskschedule

import (
	"slices"
	"time"
)

// Tier is how much a read's work is wanted: a playback's reads are Continuous, the rest follow the
// tiers of work-scheduling.md §7.
type Tier int

// The tiers, lowest first.
const (
	Background Tier = iota
	Predicted
	Attended
	Interactive
	Continuous
)

// Read is one read of the device a work asks for.
type Read struct {
	// Work names the work it is a piece of: a work has one read waiting or out at once.
	Work string
	Tier Tier
	// Deadline is when its want needs it; zero for none, which comes after any deadline.
	Deadline time.Time
	Bytes    int64

	arrival int
	sentAt  time.Time
}

// Settings are a device's: how many reads it has out at once, and the background rate's bounds and
// the playback latency that halves it.
type Settings struct {
	Limit int
	// BackgroundBytesPerSecond is the rate background reads may take at most, and its floor the least
	// they keep, so background work always moves.
	BackgroundBytesPerSecond, BackgroundFloorBytesPerSecond float64
	// SlowPlaybackRead is how long a playback read may take before the background rate halves.
	SlowPlaybackRead time.Duration
}

// Scheduler is one device's.
type Scheduler struct {
	settings Settings
	waiting  []*Read
	out      int
	arrivals int
	// rate is the background rate now, and tokens the background bytes it may send, refilled at
	// rate since refilledAt.
	rate, tokens float64
	refilledAt   time.Time
}

// New returns a device's scheduler.
func New(settings Settings) *Scheduler {
	settings.Limit = max(1, settings.Limit)
	return &Scheduler{
		settings: settings, rate: settings.BackgroundBytesPerSecond, tokens: settings.BackgroundBytesPerSecond,
	}
}

// Add asks for a read. A work that already has a read waiting has it replaced: its newer want
// decides.
func (scheduler *Scheduler) Add(read Read) {
	scheduler.arrivals++
	read.arrival = scheduler.arrivals
	scheduler.Cancel(read.Work)
	scheduler.waiting = append(scheduler.waiting, &read)
}

// Cancel withdraws a work's waiting read: its want ended.
func (scheduler *Scheduler) Cancel(work string) {
	scheduler.waiting = slices.DeleteFunc(scheduler.waiting, func(waiting *Read) bool { return waiting.Work == work })
}

// Next is the read to send to the device now, if one may go. When background reads wait on their
// rate alone, retryAt is when the next may go.
func (scheduler *Scheduler) Next(now time.Time) (next Read, isSent bool, retryAt time.Time) {
	if scheduler.out >= scheduler.settings.Limit || len(scheduler.waiting) == 0 {
		return Read{}, false, time.Time{}
	}
	scheduler.refill(now)
	slices.SortStableFunc(scheduler.waiting, compareReads)
	read := scheduler.waiting[0]
	if read.Tier == Background && scheduler.tokens < float64(read.Bytes) {
		short := float64(read.Bytes) - scheduler.tokens
		return Read{}, false, now.Add(time.Duration(short / scheduler.rate * float64(time.Second)))
	}
	scheduler.waiting = scheduler.waiting[1:]
	if read.Tier == Background {
		scheduler.tokens -= float64(read.Bytes)
	}
	scheduler.out++
	read.sentAt = now
	return *read, true, time.Time{}
}

// Done says a read sent came back, and how long it took: a slow playback read halves the
// background rate.
func (scheduler *Scheduler) Done(now time.Time, read Read) {
	scheduler.out = max(0, scheduler.out-1)
	scheduler.refill(now)
	if read.Tier == Continuous && now.Sub(read.sentAt) > scheduler.settings.SlowPlaybackRead {
		scheduler.rate = max(scheduler.settings.BackgroundFloorBytesPerSecond, scheduler.rate/2)
		scheduler.tokens = min(scheduler.tokens, scheduler.rate)
	}
}

// refill adds the background bytes earned since the last refill, and climbs the rate back towards
// its bound by a tenth of the bound for each second.
func (scheduler *Scheduler) refill(now time.Time) {
	if scheduler.refilledAt.IsZero() {
		scheduler.refilledAt = now
		return
	}
	elapsed := now.Sub(scheduler.refilledAt).Seconds()
	if elapsed <= 0 {
		return
	}
	scheduler.refilledAt = now
	bound := scheduler.settings.BackgroundBytesPerSecond
	scheduler.rate = min(bound, scheduler.rate+bound/10*elapsed)
	scheduler.tokens = min(scheduler.rate, scheduler.tokens+scheduler.rate*elapsed)
}

// compareReads orders reads by tier, highest first, then by deadline, earliest first and none last,
// then by arrival.
func compareReads(first, second *Read) int {
	if first.Tier != second.Tier {
		return int(second.Tier - first.Tier)
	}
	switch {
	case first.Deadline.Equal(second.Deadline):
	case first.Deadline.IsZero():
		return 1
	case second.Deadline.IsZero():
		return -1
	default:
		return first.Deadline.Compare(second.Deadline)
	}
	return first.arrival - second.arrival
}
