// SPDX-License-Identifier: AGPL-3.0-or-later

// Package diskschedule is the node's disk scheduler of D329: for one device, which read goes next.
// Every read of the device is asked of it: a playback's, a listing's, a piece of a read-through.
// It keeps few reads on the device and the rest waiting where they can still be reordered, sends the
// next to the highest tier, the earliest deadline, then the first to come, and holds background
// reads to a rate that falls fast when a playback's read is slow and climbs back slowly
// (work-scheduling.md §8).
//
// Scheduler decides and does nothing else: no goroutines, no clock of its own, no disk. The caller
// says what happened and when, and asks what to send. So the scheduling bench (internal/schedtest)
// runs the same code in virtual time that the node runs in real time, through Device, which waits
// each read's turn on the node's clock.
//
// # Invariants
//
//   - At most Limit reads are on the device at once (TestScheduler_KeepsAtMostItsLimitOnTheDevice).
//   - A work has at most one read waiting, its newest: a long work is a run of short reads, each
//     asked when the last came back, and between two of them anything more urgent goes first
//     (TestScheduler_AnAttendedReadGoesBeforeTheNextPieceOfBackgroundWork).
//   - The order is the tier, then the earliest deadline, then arrival
//     (TestScheduler_OrdersByTierThenDeadlineThenArrival).
//   - Background reads take no more than their rate; a slow playback read halves it, and it climbs
//     back by a step a second (TestScheduler_ASlowPlaybackReadHalvesTheBackgroundRate).
//   - Device runs the Scheduler in real time: a read waits for its turn and holds its place until it
//     says it is done, a read given up while it waits holds nothing, and a read held back by the
//     background rate goes when the clock passes its wait (device_test.go).
//
// # Read before changing
//
//   - D329; docs/contributors/design/plans/work-scheduling.md §6, §7 and §8.
//   - docs/contributors/design/plans/data-timeliness.md, the scenarios it answers.
//
// # Where to change
//
//   - device.go: waiting for turns in real time.
//   - schedule.go: the order, the limit and the background rate. Run internal/schedtest's scenarios
//     after any change: they judge it by what the people in them would have seen.
package diskschedule
