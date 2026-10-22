// SPDX-License-Identifier: AGPL-3.0-or-later

package schedtest

import (
	"testing"
	"time"
)

const megabyte = 1_000_000

// readAt returns when a read sent to disk now is read.
func readAt(sim *Sim, disk *Disk, stream string, bytes int64) *time.Duration {
	var at time.Duration = -1
	disk.Read(stream, bytes, func() { at = sim.Now() })
	return &at
}

func TestDisk_ServesReadsInOrderPayingASeekForAnotherStream(t *testing.T) {
	t.Parallel()
	var sim Sim
	disk := NewDisk(&sim, 100*megabyte, 10*time.Millisecond)
	first := readAt(&sim, disk, "a", 100*megabyte)
	second := readAt(&sim, disk, "a", 100*megabyte)
	third := readAt(&sim, disk, "b", 100*megabyte)
	sim.RunUntil(time.Minute)
	// The first read seeks to stream a; the second follows it; the third seeks to b.
	if *first != 1010*time.Millisecond || *second != 2010*time.Millisecond || *third != 3020*time.Millisecond {
		t.Errorf("reads ended at %v, %v, %v; want 1.01s, 2.01s, 3.02s", *first, *second, *third)
	}
}

func TestDisk_AReadAfterItSleptPaysItsSpinUp(t *testing.T) {
	t.Parallel()
	var sim Sim
	disk := NewDisk(&sim, 100*megabyte, 0)
	disk.SleepsAfter, disk.SpinUp = 20*time.Minute, 23*time.Second
	readAt(&sim, disk, "a", megabyte)
	sim.RunUntil(time.Hour)
	woken := readAt(&sim, disk, "a", megabyte)
	sim.RunUntil(2 * time.Hour)
	if *woken != time.Hour+23*time.Second+10*time.Millisecond || disk.Wakes != 1 {
		t.Errorf("a read after an hour asleep ended at %v with %d wakes; want 1h0m23.01s and 1", *woken, disk.Wakes)
	}
}

func TestDisk_ADeadDiskAnswersNothingUntilItIsRevivedAndRecovered(t *testing.T) {
	t.Parallel()
	var sim Sim
	disk := NewDisk(&sim, 100*megabyte, 0)
	sim.At(500*time.Millisecond, disk.Die)
	underway := readAt(&sim, disk, "a", 100*megabyte)
	var later *time.Duration
	sim.At(10*time.Second, func() { later = readAt(&sim, disk, "a", 100*megabyte) })
	sim.At(60*time.Second, func() { disk.Revive(48 * time.Second) })
	sim.RunUntil(5 * time.Minute)
	// Revived at 60 s, answering at 108 s: the read it died under is read again, then the next.
	if *underway != 109*time.Second || *later != 110*time.Second {
		t.Errorf("reads ended at %v and %v; want 1m49s and 1m50s", *underway, *later)
	}
}

func TestDisk_LoadSlowsEveryRead(t *testing.T) {
	t.Parallel()
	var sim Sim
	disk := NewDisk(&sim, 100*megabyte, 0)
	disk.SetLoad(2)
	read := readAt(&sim, disk, "a", 100*megabyte)
	sim.RunUntil(time.Minute)
	if *read != 2*time.Second {
		t.Errorf("a read under a load of 2 ended at %v; want 2s", *read)
	}
}
