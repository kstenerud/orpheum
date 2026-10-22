// SPDX-License-Identifier: AGPL-3.0-or-later

package schedtest

import (
	"time"

	"github.com/kstenerud/orpheum/internal/node/diskschedule"
)

// Decided puts a device's reads through the node's disk scheduler (D329, work-scheduling.md §8):
// every read, a playback's and every piece of a job's, is asked of it, and it says what goes next.
type Decided struct {
	sim       *Sim
	disk      *Disk
	scheduler *diskschedule.Scheduler
	// isPumpArmed is whether a pump waits for the background rate already.
	isPumpArmed bool
	// dones are the callers of the reads asked and not yet sent, by work.
	dones pending
}

// NewDecided returns the disk scheduler's policy for disk.
func NewDecided(sim *Sim, disk *Disk, settings diskschedule.Settings) *Decided {
	return &Decided{sim: sim, disk: disk, scheduler: diskschedule.New(settings)}
}

// benchStart is the moment the scheduler's times count from: the bench's time is a duration since
// its run began.
func benchStart() time.Time { return time.Date(2026, 10, 21, 20, 0, 0, 0, time.UTC) }

func (decided *Decided) now() time.Time { return benchStart().Add(decided.sim.Now()) }

// ReadForPlay asks the scheduler for each of a playback's reads, as Continuous.
func (decided *Decided) ReadForPlay(stream string) func(int64, func()) {
	return func(bytes int64, done func()) {
		decided.ask(diskschedule.Read{Work: stream, Tier: diskschedule.Continuous, Bytes: bytes}, done)
	}
}

// Submit asks for a job's reads, a piece at a time, at its tier.
func (decided *Decided) Submit(job *Job) {
	job.RequestedAt = decided.sim.Now()
	decided.askPiece(job)
}

func (decided *Decided) askPiece(job *Job) {
	piece := min(job.Piece, job.Bytes-job.readSoFar)
	decided.ask(diskschedule.Read{Work: job.Name, Tier: job.Tier, Bytes: piece}, func() {
		job.readSoFar += piece
		if job.readSoFar >= job.Bytes {
			job.FinishedAt, job.HasFinished = decided.sim.Now(), true
			return
		}
		decided.askPiece(job)
	})
	if !job.HasStarted {
		job.StartedAt, job.HasStarted = decided.sim.Now(), true
	}
}

// pending are the callers of the reads asked and not yet answered, by work.
type pending map[string]func()

// ask adds a read and sends what may go.
func (decided *Decided) ask(read diskschedule.Read, done func()) {
	decided.scheduler.Add(read)
	decided.waitingDone()[read.Work] = done
	decided.pump()
}

func (decided *Decided) waitingDone() pending {
	if decided.dones == nil {
		decided.dones = pending{}
	}
	return decided.dones
}

// pump sends every read the scheduler lets go now, and comes back when the background rate allows
// the next.
func (decided *Decided) pump() {
	for {
		read, isSent, retryAt := decided.scheduler.Next(decided.now())
		if !isSent {
			if !retryAt.IsZero() && !decided.isPumpArmed {
				decided.isPumpArmed = true
				decided.sim.At(retryAt.Sub(benchStart()), func() { decided.isPumpArmed = false; decided.pump() })
			}
			return
		}
		done := decided.dones[read.Work]
		delete(decided.dones, read.Work)
		decided.disk.Read(read.Work, read.Bytes, func() {
			decided.scheduler.Done(decided.now(), read)
			done()
			decided.pump()
		})
	}
}
