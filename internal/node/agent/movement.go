// SPDX-License-Identifier: GPL-3.0-or-later

package agent

import (
	"sync"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/clock"
)

// movement is when a held job last moved: its FFmpeg's position advanced or its bytes grew, or its
// own reading read or listed something (execx.WithMoved). Its paused time is not counted, as a
// paused run's is neither a stall nor spent deadline (D240 §6). It starts when the job is held.
type movement struct {
	clock clock.Clock

	mutex       sync.Mutex
	lastMovedAt time.Time     // guarded by mutex
	pausedAt    time.Time     // guarded by mutex: zero while not paused
	pausedFor   time.Duration // guarded by mutex: the paused time since it last moved
}

func newMovement(clockSource clock.Clock) *movement {
	return &movement{clock: clockSource, lastMovedAt: clockSource.Now()}
}

// moved records movement now.
func (motion *movement) moved() {
	now := motion.clock.Now()
	motion.mutex.Lock()
	defer motion.mutex.Unlock()
	motion.lastMovedAt, motion.pausedFor = now, 0
	if !motion.pausedAt.IsZero() {
		motion.pausedAt = now
	}
}

// pause stops counting time until resume.
func (motion *movement) pause() {
	now := motion.clock.Now()
	motion.mutex.Lock()
	defer motion.mutex.Unlock()
	if motion.pausedAt.IsZero() {
		motion.pausedAt = now
	}
}

// resume counts time again.
func (motion *movement) resume() {
	now := motion.clock.Now()
	motion.mutex.Lock()
	defer motion.mutex.Unlock()
	if !motion.pausedAt.IsZero() {
		motion.pausedFor += now.Sub(motion.pausedAt)
		motion.pausedAt = time.Time{}
	}
}

// sinceMoved is how long it has not moved, its paused time apart.
func (motion *movement) sinceMoved() time.Duration {
	now := motion.clock.Now()
	motion.mutex.Lock()
	defer motion.mutex.Unlock()
	paused := motion.pausedFor
	if !motion.pausedAt.IsZero() {
		paused += now.Sub(motion.pausedAt)
	}
	return now.Sub(motion.lastMovedAt) - paused
}
