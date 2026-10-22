// SPDX-License-Identifier: AGPL-3.0-or-later

package schedtest

import (
	"time"
)

// Playback is a client playing a film directly: it reads ahead, a chunk at a time, until its buffer
// holds BufferTarget, starts once it holds StartBuffer, and stalls when it plays everything it read.
type Playback struct {
	sim *Sim
	// read is how its reads reach the disk: the policy's way for play.
	read func(bytes int64, done func())
	// BytesPerSecond is the film's bitrate; Length its running time.
	BytesPerSecond float64
	Length         time.Duration
	// Chunk, StartBuffer and BufferTarget are how much it reads at once, holds before starting, and
	// keeps ahead.
	Chunk, StartBuffer, BufferTarget time.Duration

	// RequestedAt is when Play was pressed; StartedAt when the first frame showed, zero until then.
	RequestedAt, StartedAt time.Duration
	// Stalls counts the stalls after the start, and Stalled is how long they lasted in all.
	Stalls  int
	Stalled time.Duration

	read0       time.Duration // the media time read so far
	position    time.Duration // the media time played so far
	updatedAt   time.Duration // when position was last brought up to date
	isStalled   bool
	stalledFrom time.Duration
	isReading   bool
}

// NewPlayback returns a playback of a film of bytesPerSecond and length, read through read, with a
// client's usual chunk (2 s), start (2 s) and buffer target (30 s).
func NewPlayback(sim *Sim, read func(int64, func()), bytesPerSecond float64, length time.Duration) *Playback {
	return &Playback{
		sim: sim, read: read, BytesPerSecond: bytesPerSecond, Length: length,
		Chunk: 2 * time.Second, StartBuffer: 2 * time.Second, BufferTarget: 30 * time.Second,
	}
}

// Play presses Play now.
func (playback *Playback) Play() {
	playback.RequestedAt = playback.sim.Now()
	playback.readMore()
}

// StartupTook is how long the first frame took, or how long it has been waited for.
func (playback *Playback) StartupTook() time.Duration {
	if playback.StartedAt == 0 {
		return playback.sim.Now() - playback.RequestedAt
	}
	return playback.StartedAt - playback.RequestedAt
}

// update brings the position up to now, and notes a stall where it caught up with what was read.
func (playback *Playback) update() {
	now := playback.sim.Now()
	if playback.StartedAt > 0 && !playback.isStalled {
		playback.position += now - playback.updatedAt
		if playback.position >= playback.read0 && playback.read0 < playback.Length {
			playback.position = playback.read0
			playback.isStalled, playback.stalledFrom = true, now
			playback.Stalls++
		}
		playback.position = min(playback.position, playback.Length)
	}
	playback.updatedAt = now
}

// readMore reads the next chunk while the buffer is below its target; above it, it comes back when
// it falls there.
func (playback *Playback) readMore() {
	playback.update()
	if playback.isReading || playback.read0 >= playback.Length {
		return
	}
	ahead := playback.read0 - playback.position
	if playback.StartedAt > 0 && !playback.isStalled && ahead >= playback.BufferTarget {
		playback.sim.After(ahead-playback.BufferTarget+time.Millisecond, playback.readMore)
		return
	}
	chunk := min(playback.Chunk, playback.Length-playback.read0)
	playback.isReading = true
	playback.read(int64(chunk.Seconds()*playback.BytesPerSecond), func() { playback.chunkRead(chunk) })
	if playback.StartedAt > 0 && !playback.isStalled {
		// A stall can begin while the read is out: look again when the buffer would run dry.
		playback.sim.After(ahead+time.Millisecond, playback.update)
	}
}

// chunkRead takes a chunk in: the first frame once StartBuffer is held, the end of a stall once a
// chunk is.
func (playback *Playback) chunkRead(chunk time.Duration) {
	playback.update()
	playback.isReading = false
	playback.read0 += chunk
	now := playback.sim.Now()
	switch {
	case playback.StartedAt == 0 && playback.read0 >= min(playback.StartBuffer, playback.Length):
		playback.StartedAt = now
	case playback.isStalled:
		playback.isStalled = false
		playback.Stalled += now - playback.stalledFrom
	}
	playback.readMore()
}
