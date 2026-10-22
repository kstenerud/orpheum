// SPDX-License-Identifier: AGPL-3.0-or-later

package diskschedule

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// holding asks for a place in a goroutine, and says on the channel when it holds one.
func holding(ctx context.Context, device *Device, read Read) <-chan func() {
	held := make(chan func(), 1)
	go func() {
		release, err := device.Hold(ctx, read)
		if err == nil {
			held <- release
		}
	}()
	return held
}

// A device with one place: a read waits while another holds it, and of the two waiting, the
// attended one is sent before the background one when the place is released.
func TestDevice_AWaitingReadIsSentInTheSchedulersOrderWhenThePlaceFrees(t *testing.T) {
	t.Parallel()
	device := NewDevice(clocktest.NewFake(start), unlimited())
	release, err := device.Hold(t.Context(), Read{Work: "first", Tier: Background, Bytes: 1})
	if err != nil {
		t.Fatal(err)
	}
	background := holding(t.Context(), device, Read{Work: "background", Tier: Background, Bytes: 1})
	waitUntilWaiting(t, device, 1)
	attended := holding(t.Context(), device, Read{Work: "attended", Tier: Attended, Bytes: 1})
	waitUntilWaiting(t, device, 2)
	release()
	releaseAttended := testutil.Receive(t, attended, "the attended read's turn")
	select {
	case <-background:
		t.Fatal("the background read was sent while the attended one held the device's one place")
	default:
	}
	releaseAttended()
	testutil.Receive(t, background, "the background read's turn")()
}

// A read whose caller gives up while it waits holds nothing, and the next is sent in its place.
func TestDevice_AReadWithdrawnWhileWaitingHoldsNothing(t *testing.T) {
	t.Parallel()
	device := NewDevice(clocktest.NewFake(start), unlimited())
	release, err := device.Hold(t.Context(), Read{Work: "first", Tier: Attended, Bytes: 1})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	withdrawn := make(chan error, 1)
	go func() {
		_, holdErr := device.Hold(ctx, Read{Work: "given up", Tier: Attended, Bytes: 1})
		withdrawn <- holdErr
	}()
	waitUntilWaiting(t, device, 1)
	cancel()
	if holdErr := testutil.Receive(t, withdrawn, "the withdrawn read's answer"); !errors.Is(holdErr, context.Canceled) {
		t.Errorf("a read given up while waiting answered %v; want context.Canceled", holdErr)
	}
	release()
	testutil.Receive(t, holding(t.Context(), device, Read{Work: "next", Tier: Background, Bytes: 1}),
		"the next read's turn")()
}

// A background read held back by the rate is sent once the device's clock has passed the wait.
func TestDevice_ABackgroundReadHeldByTheRateGoesWhenTheClockPassesItsWait(t *testing.T) {
	t.Parallel()
	fake := clocktest.NewFake(start)
	device := NewDevice(fake, Settings{
		Limit: 4, BackgroundBytesPerSecond: 10 * megabyte, BackgroundFloorBytesPerSecond: megabyte,
		SlowPlaybackRead: time.Second,
	})
	release, err := device.Hold(t.Context(), Read{Work: "first", Tier: Background, Bytes: 10 * megabyte})
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	held := holding(t.Context(), device, Read{Work: "second", Tier: Background, Bytes: 10 * megabyte})
	waitUntilWaiting(t, device, 1)
	select {
	case <-held:
		t.Fatal("a second 10 MB background read went at once at 10 MB/s")
	default:
	}
	fake.Advance(time.Second)
	testutil.Receive(t, held, "the second background read, a second on")()
}

// waitUntilWaiting waits until count reads wait on the device for their turn.
func waitUntilWaiting(t *testing.T, device *Device, count int) {
	t.Helper()
	ticker := make(chan struct{})
	go func() {
		defer close(ticker)
		for {
			device.mutex.Lock()
			waiting := len(device.turns)
			device.mutex.Unlock()
			if waiting >= count {
				return
			}
			time.Sleep(time.Millisecond)
		}
	}()
	testutil.Receive(t, ticker, "the reads to wait on the device")
}
