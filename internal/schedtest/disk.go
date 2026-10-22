// SPDX-License-Identifier: AGPL-3.0-or-later

package schedtest

import (
	"time"
)

// Disk is one device: it serves one read at a time, in the order the reads were sent, as a
// spinning disk or a NAS behind one mount does below whatever ordered them. A read of another
// stream than the last pays a seek; a read after the disk slept pays its spin-up; a dead disk
// answers nothing until it is revived and its recovery (an NFS client's reconnection) has passed.
type Disk struct {
	sim *Sim
	// BytesPerSecond is how fast it reads one stream, with nothing else on it.
	BytesPerSecond float64
	// Seek is what a read of another stream than the last costs first.
	Seek time.Duration
	// SleepsAfter is how long without a read it takes to spin down; zero for a disk that never
	// sleeps. SpinUp is what its next read then costs first.
	SleepsAfter, SpinUp time.Duration
	// load slows every read: 2 is another program taking half of it (a RAID scrub).
	load       float64
	queue      []*diskRead
	isBusy     bool
	lastStream string
	lastReadAt time.Duration
	isDead     bool
	// answersFrom is when a revived disk answers again.
	answersFrom time.Duration
	// Wakes counts the reads that found it asleep.
	Wakes int
}

// NewDisk returns an awake disk, reading bytesPerSecond, whose reads of another stream cost seek.
func NewDisk(sim *Sim, bytesPerSecond float64, seek time.Duration) *Disk {
	return &Disk{sim: sim, BytesPerSecond: bytesPerSecond, Seek: seek, load: 1}
}

// diskRead is one read sent to the disk.
type diskRead struct {
	stream string
	bytes  int64
	done   func()
}

// Read sends a read of bytes for stream; done runs when it has been read.
func (disk *Disk) Read(stream string, bytes int64, done func()) {
	disk.queue = append(disk.queue, &diskRead{stream: stream, bytes: bytes, done: done})
	disk.serveNext()
}

// Queued is how many reads were sent and not yet read, the one being read among them.
func (disk *Disk) Queued() int {
	if disk.isBusy {
		return len(disk.queue) + 1
	}
	return len(disk.queue)
}

// SetLoad slows every read by factor from now: 1 is the disk to itself.
func (disk *Disk) SetLoad(factor float64) { disk.load = max(1, factor) }

// Die makes the disk answer nothing: the read it is on, and every read after, waits.
func (disk *Disk) Die() { disk.isDead = true }

// Revive makes the disk answer again once recovery has passed, as an NFS client reconnects some
// time after its server returns.
func (disk *Disk) Revive(recovery time.Duration) {
	disk.isDead = false
	disk.answersFrom = disk.sim.Now() + recovery
	disk.sim.At(disk.answersFrom, disk.serveNext)
}

// IsAnswering is whether a read sent now would be read: neither dead nor recovering.
func (disk *Disk) IsAnswering() bool { return !disk.isDead && disk.sim.Now() >= disk.answersFrom }

// serveNext starts the next read when the disk is free and answering.
func (disk *Disk) serveNext() {
	if disk.isBusy || len(disk.queue) == 0 || !disk.IsAnswering() {
		return
	}
	next := disk.queue[0]
	disk.queue = disk.queue[1:]
	disk.isBusy = true
	took := disk.costOf(next)
	disk.sim.After(took, func() { disk.finish(next) })
}

// costOf is how long a read takes from now: a spin-up if it slept, a seek if it is another stream's,
// and its bytes at the disk's speed under its load.
func (disk *Disk) costOf(read *diskRead) time.Duration {
	var cost time.Duration
	if disk.SleepsAfter > 0 && disk.lastReadAt > 0 && disk.sim.Now()-disk.lastReadAt >= disk.SleepsAfter {
		cost += disk.SpinUp
		disk.Wakes++
	}
	if read.stream != disk.lastStream {
		cost += disk.Seek
	}
	seconds := float64(read.bytes) / disk.BytesPerSecond * disk.load
	return cost + time.Duration(seconds*float64(time.Second))
}

// finish ends a read, unless the disk died under it, when the read waits for its revival.
func (disk *Disk) finish(read *diskRead) {
	if !disk.IsAnswering() {
		disk.sim.At(max(disk.answersFrom, disk.sim.Now()), func() { disk.finishWhenAnswering(read) })
		return
	}
	disk.isBusy = false
	disk.lastStream, disk.lastReadAt = read.stream, disk.sim.Now()
	read.done()
	disk.serveNext()
}

// finishWhenAnswering finishes a read the disk died under, once it answers again; a disk still
// dead leaves it waiting for Revive's own call.
func (disk *Disk) finishWhenAnswering(read *diskRead) {
	if disk.isDead {
		disk.queue = append([]*diskRead{read}, disk.queue...)
		disk.isBusy = false
		return
	}
	if disk.sim.Now() < disk.answersFrom {
		disk.sim.At(disk.answersFrom, func() { disk.finishWhenAnswering(read) })
		return
	}
	disk.finish(read)
}
