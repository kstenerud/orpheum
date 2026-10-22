// SPDX-License-Identifier: AGPL-3.0-or-later

package schedtest

import (
	"testing"
	"time"
)

func TestPlayback_StartsOnItsFirstChunkAndNeverStallsOnAFreeDisk(t *testing.T) {
	t.Parallel()
	var sim Sim
	disk := NewDisk(&sim, 100*megabyte, 10*time.Millisecond)
	playback := NewPlayback(&sim, func(bytes int64, done func()) { disk.Read("film", bytes, done) },
		2.5*megabyte, 10*time.Minute)
	playback.Play()
	sim.RunUntil(time.Hour)
	// A 2 s chunk is 5 MB: 60 ms to read, with its seek.
	if playback.StartupTook() != 60*time.Millisecond || playback.Stalls != 0 {
		t.Errorf("started in %v with %d stalls; want 60ms and none", playback.StartupTook(), playback.Stalls)
	}
}

func TestPlayback_StallsOnceItsBufferRunsDryOnADeadDisk(t *testing.T) {
	t.Parallel()
	var sim Sim
	disk := NewDisk(&sim, 100*megabyte, 0)
	playback := NewPlayback(&sim, func(bytes int64, done func()) { disk.Read("film", bytes, done) },
		2.5*megabyte, 10*time.Minute)
	playback.Play()
	sim.At(2*time.Minute, disk.Die)
	sim.At(3*time.Minute, func() { disk.Revive(48 * time.Second) })
	sim.RunUntil(time.Hour)
	// About 30 s buffered when it died at 2:00; answering again at 3:48, it stalled about 78 s.
	if playback.Stalls != 1 || playback.Stalled < 75*time.Second || playback.Stalled > 80*time.Second {
		t.Errorf("stalled %d times for %v; want once, for about 78s", playback.Stalls, playback.Stalled)
	}
}
