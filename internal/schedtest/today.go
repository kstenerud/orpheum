// SPDX-License-Identifier: AGPL-3.0-or-later

package schedtest

import (
	"slices"
	"time"

	"github.com/kstenerud/orpheum/internal/node/diskschedule"
)

// Job is a piece of work that reads a disk: a scan's listing, a read-through, an extraction.
type Job struct {
	Name string
	// Bytes is how much it reads in all, in reads of Piece.
	Bytes, Piece int64
	// Priority is today's number: 1000 or more is play-driven (the queue inventory §1.1).
	Priority int
	// Tier is its tier under D329, which Decided schedules by.
	Tier diskschedule.Tier

	// RequestedAt, StartedAt and FinishedAt are when it was asked for, began reading, and ended.
	RequestedAt, StartedAt, FinishedAt time.Duration
	// HasStarted, HasFinished and HasStalled say which of those happened, and whether it ended by
	// its stall window rather than by reading it all.
	HasStarted, HasFinished, HasStalled bool

	readSoFar int64
	isPaused  bool
	// readOut counts the reads sent and not answered, so a stall's check knows whether the one it
	// watched came back.
	readOut  int
	readSent int
}

// IsPlayDriven is today's flag: a priority of 1000 or more.
func (job *Job) IsPlayDriven() bool { return job.Priority >= playDriven }

// playDriven is the priority from which work is play-driven (nodes/assign.go:97).
const playDriven = 1000

// Today is the node's work list for one device as it is today (the queue inventory §2.2): work in
// numeric order and then arrival; background work starts only under the reader limit; play-driven
// work starts at once, past the limit, and pauses background work; a running job is never displaced;
// a job whose read stands still for its stall window ends, freeing its reader, while the read itself
// stays on the disk until the disk answers.
type Today struct {
	sim  *Sim
	disk *Disk
	// ReaderLimit is the device's reader limit (registry.go: 1 for a spinning or unknown disk).
	ReaderLimit int
	// StallWindow is how long a read may stand still before its job is answered as stalled
	// (read.go: 3 min for a read-through).
	StallWindow time.Duration
	waiting     []*Job
	running     []*Job
}

// NewToday returns today's work list for disk.
func NewToday(sim *Sim, disk *Disk) *Today {
	return &Today{sim: sim, disk: disk, ReaderLimit: 1, StallWindow: 3 * time.Minute}
}

// ReadForPlay sends a playback's read straight to the disk: play-driven, it waits for no limit.
func (today *Today) ReadForPlay(stream string) func(int64, func()) {
	return func(bytes int64, done func()) { today.disk.Read(stream, bytes, done) }
}

// Submit gives the work list a job.
func (today *Today) Submit(job *Job) {
	job.RequestedAt = today.sim.Now()
	today.waiting = append(today.waiting, job)
	slices.SortStableFunc(today.waiting, func(first, second *Job) int { return second.Priority - first.Priority })
	today.startDue()
}

// startDue starts what may start: play-driven work at once, pausing background work; background work
// while fewer jobs than the limit read the device.
func (today *Today) startDue() {
	for index := 0; index < len(today.waiting); {
		job := today.waiting[index]
		if !job.IsPlayDriven() && today.reading() >= today.ReaderLimit {
			index++
			continue
		}
		today.waiting = slices.Delete(today.waiting, index, index+1)
		if job.IsPlayDriven() {
			today.pauseBackground(true)
		}
		today.start(job)
	}
}

// reading is how many jobs read the device: every running job not paused, play-driven ones too
// (working.go:79, :217-221; the queue inventory §2.2, §3.6).
func (today *Today) reading() int {
	count := 0
	for _, job := range today.running {
		if !job.isPaused {
			count++
		}
	}
	return count
}

// pauseBackground pauses or continues the background jobs reading the device (D240 §6).
func (today *Today) pauseBackground(pause bool) {
	for _, job := range today.running {
		if job.IsPlayDriven() || job.isPaused == pause {
			continue
		}
		job.isPaused = pause
		if !pause && job.readOut == 0 {
			today.readNext(job)
		}
	}
}

func (today *Today) start(job *Job) {
	job.StartedAt, job.HasStarted = today.sim.Now(), true
	today.running = append(today.running, job)
	today.readNext(job)
}

// readNext sends a job's next piece, and watches it for its stall window.
func (today *Today) readNext(job *Job) {
	if job.isPaused || job.HasFinished || job.readOut > 0 {
		return
	}
	if job.readSoFar >= job.Bytes {
		today.end(job, false)
		return
	}
	piece := min(job.Piece, job.Bytes-job.readSoFar)
	job.readOut++
	job.readSent++
	sent := job.readSent
	today.disk.Read(job.Name, piece, func() {
		job.readOut--
		if job.HasFinished {
			return
		}
		job.readSoFar += piece
		today.readNext(job)
	})
	today.sim.After(today.StallWindow, func() {
		if !job.HasFinished && job.readOut > 0 && job.readSent == sent {
			today.end(job, true)
		}
	})
}

// end ends a job, frees its reader, and starts what was waiting for it.
func (today *Today) end(job *Job, stalled bool) {
	job.FinishedAt, job.HasFinished, job.HasStalled = today.sim.Now(), true, stalled
	today.running = slices.DeleteFunc(today.running, func(running *Job) bool { return running == job })
	if job.IsPlayDriven() && !slices.ContainsFunc(today.running, (*Job).IsPlayDriven) {
		today.pauseBackground(false)
	}
	today.startDue()
}
