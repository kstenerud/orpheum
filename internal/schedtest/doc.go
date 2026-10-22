// SPDX-License-Identifier: AGPL-3.0-or-later

// Package schedtest is the scheduling test bench of D329: a discrete-event simulation, in virtual
// time, of the resources work competes for (a disk, its reads, its sleep and its death), the work
// that wants them, and the policy that decides between them. The data-timeliness scenarios are run
// through it, and each policy is judged by what the people in them would have seen: how long a
// playback took to start and stalled, and how long the work a person waited for waited.
//
// It is phase 0 of work-scheduling.md §16. Today's scheduler is modelled here as the baseline
// (Today); each later phase plugs its real decider in behind the same Policy, so the bench judges
// the code that runs, not a second model of it: Decided is the node's own disk scheduler. It is test
// support: only tests import it.
//
// # Invariants
//
//   - Time is virtual and moves only from one event to the next: a run is the same every time, and
//     no test waits in real time (Sim; TestSim_EventsRunInTheirTimeOrderAndTiesInTheirOrder).
//   - A disk serves one read at a time, in the order its reads were sent, a read of another stream
//     than the last paying a seek; a sleeping disk spins up first; a dead disk answers nothing until
//     it is revived and its recovery has passed (Disk; disk_test.go).
//   - A direct play's client reads ahead to its buffer target and stalls only when its buffer is
//     empty (Playback; playback_test.go).
//   - Today is the node's work list as the queue inventory read it at 1bfd482c: numeric priority,
//     a reader limit per device that every reading job counts against, play-driven work past it, a
//     running job never displaced, and a stuck read holding its reader until its stall window ends
//     (today.go; today_test.go).
//   - Decided asks the disk scheduler for every read, a playback's and each piece of a job's, and
//     sends only what it lets go (decided.go; decided_test.go).
//
// # Read before changing
//
//   - D329 and docs/contributors/design/plans/work-scheduling.md §16.
//   - docs/contributors/design/plans/data-timeliness.md: the scenarios and their deadlines.
//   - docs/contributors/design/research/job-queues-inventory-2026-10-21.md: what Today models.
//
// # Where to change
//
//   - sim.go: the event loop.
//   - disk.go: what a disk does with reads.
//   - playback.go: a client playing a film.
//   - today.go: the baseline policy.
//   - decided.go: D329's policy, the node's disk scheduler (internal/node/diskschedule) deciding every
//     read of the device.
package schedtest
