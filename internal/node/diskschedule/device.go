// SPDX-License-Identifier: AGPL-3.0-or-later

package diskschedule

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/clock"
)

// Device runs a Scheduler for one device in real time: a caller asks to read, waits for its turn,
// reads, and says it is done; or holds a place for a run whose reads it cannot see. It is safe for
// concurrent use.
type Device struct {
	clock     clock.Clock
	mutex     sync.Mutex
	scheduler *Scheduler // guarded by mutex
	// turns are the callers waiting, by work: closed when their read is sent. Guarded by mutex.
	turns map[string]chan struct{}
	// sent are the reads out on the device, by work, for Done. Guarded by mutex.
	sent map[string]Read
	// isWakeArmed is whether a wake for the background rate is set already. Guarded by mutex.
	isWakeArmed bool
}

// NewDevice returns a device's scheduler running on clockSource.
func NewDevice(clockSource clock.Clock, settings Settings) *Device {
	return &Device{
		clock: clockSource, scheduler: New(settings), turns: map[string]chan struct{}{}, sent: map[string]Read{},
	}
}

// Hold waits for read's turn and holds its place on the device until release is called: a read
// the caller makes itself, or a run reading the file on its own, whose reads the scheduler cannot
// see. A work holds one place at a time. When ctx ends first, nothing is held.
func (device *Device) Hold(ctx context.Context, read Read) (release func(), err error) {
	turn := make(chan struct{})
	device.mutex.Lock()
	if _, isWaiting := device.turns[read.Work]; isWaiting {
		device.mutex.Unlock()
		return nil, fmt.Errorf("work %q already waits for its turn on the device", read.Work)
	}
	device.turns[read.Work] = turn
	device.scheduler.Add(read)
	device.pump()
	device.mutex.Unlock()
	select {
	case <-turn:
	case <-ctx.Done():
		if device.withdraw(read.Work) {
			return nil, fmt.Errorf("waiting for a turn on the device: %w", ctx.Err())
		}
		// The turn came as ctx ended: it is held, and released at once.
		device.release(read.Work)
		return nil, fmt.Errorf("waiting for a turn on the device: %w", ctx.Err())
	}
	var once sync.Once
	return func() { once.Do(func() { device.release(read.Work) }) }, nil
}

// Read waits for read's turn, reads with do, and says it is done.
func (device *Device) Read(ctx context.Context, read Read, do func() error) error {
	release, err := device.Hold(ctx, read)
	if err != nil {
		return err
	}
	defer release()
	return do()
}

// withdraw takes back a work's waiting read, and reports whether it was still waiting.
func (device *Device) withdraw(work string) bool {
	device.mutex.Lock()
	defer device.mutex.Unlock()
	if _, isSent := device.sent[work]; isSent {
		return false
	}
	delete(device.turns, work)
	device.scheduler.Cancel(work)
	return true
}

// release says a work's read is done, and sends what may go next.
func (device *Device) release(work string) {
	device.mutex.Lock()
	defer device.mutex.Unlock()
	read, isSent := device.sent[work]
	if !isSent {
		return
	}
	delete(device.sent, work)
	device.scheduler.Done(device.clock.Now(), read)
	device.pump()
}

// pump sends every read that may go now, and sets a wake for when the background rate lets the next
// go. The caller holds the mutex.
func (device *Device) pump() {
	for {
		read, isSent, retryAt := device.scheduler.Next(device.clock.Now())
		if !isSent {
			device.armWake(retryAt)
			return
		}
		device.sent[read.Work] = read
		if turn, isWaiting := device.turns[read.Work]; isWaiting {
			delete(device.turns, read.Work)
			close(turn)
		}
	}
}

// armWake pumps again at retryAt, unless a wake is set already or there is none to set. The caller
// holds the mutex.
func (device *Device) armWake(retryAt time.Time) {
	if retryAt.IsZero() || device.isWakeArmed {
		return
	}
	device.isWakeArmed = true
	timer := device.clock.NewTimer(retryAt.Sub(device.clock.Now()))
	go func() {
		<-timer.C()
		device.mutex.Lock()
		defer device.mutex.Unlock()
		device.isWakeArmed = false
		device.pump()
	}()
}
