// SPDX-License-Identifier: AGPL-3.0-or-later

package rtcapi

import (
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
)

// A connection is paced at twice its window a round trip, never above 300 Mb/s, and at the ceiling
// before SCTP has measured a round trip (D397).
func TestPaceRate_IsTwiceTheWindowARoundTripUnderTheCeiling(t *testing.T) {
	t.Parallel()
	for _, example := range []struct {
		cwnd uint32
		srtt time.Duration
		want float64
	}{
		{cwnd: 64 << 10, srtt: 20 * time.Millisecond, want: 2 * (64 << 10) / 0.020},
		{cwnd: 512 << 10, srtt: 50 * time.Millisecond, want: 2 * (512 << 10) / 0.050},
		{cwnd: 4 << 20, srtt: 5 * time.Millisecond, want: 300_000_000 / 8},
		{cwnd: 64 << 10, srtt: 0, want: 300_000_000 / 8},
		{cwnd: 0, srtt: 20 * time.Millisecond, want: 300_000_000 / 8},
	} {
		if got := paceRate(example.cwnd, example.srtt); got != example.want {
			t.Errorf("paceRate(%d, %v) = %.0f; want %.0f", example.cwnd, example.srtt, got, example.want)
		}
	}
}

// The pacer lets 16 KiB go to a followed address at once, then holds what goes there to its rate:
// over any stretch of time, at most the burst and the rate times the stretch (D397).
func TestPacer_HoldsAFollowedAddressToItsRateAndBurst(t *testing.T) {
	t.Parallel()
	fake := clocktest.NewFake(time.Unix(1_800_000_000, 0))
	paced := newPacer(t.Context(), fake)
	farEnd := netip.MustParseAddrPort("192.0.2.7:50000")
	paced.follow(farEnd, func() (uint32, time.Duration) { return 64 << 10, 20 * time.Millisecond })
	paced.refresh()
	rate := paceRate(64<<10, 20*time.Millisecond)

	const packet, packets = 1200, 2000
	start := fake.Now()
	sentAt := make([]time.Duration, 0, packets)
	for range packets {
		paced.wait(net.UDPAddrFromAddrPort(farEnd), packet)
		sentAt = append(sentAt, fake.Now().Sub(start))
	}
	for first := range sentAt {
		for last := first; last < len(sentAt); last++ {
			bytes := float64((last - first + 1) * packet)
			allowed := paceBurstBytes + packet + rate*(sentAt[last]-sentAt[first]).Seconds()
			if bytes > allowed+1 {
				t.Fatalf("packets %d to %d sent %.0f bytes in %v; at most %.0f", first, last, bytes,
					sentAt[last]-sentAt[first], allowed)
			}
		}
	}
	want := time.Duration((packets*packet - paceBurstBytes) / rate * float64(time.Second))
	if took := sentAt[len(sentAt)-1]; took < want-time.Millisecond || took > want+time.Millisecond {
		t.Errorf("%d bytes took %v; at %.0f B/s after a %d-byte burst, want %v", packets*packet, took,
			rate, paceBurstBytes, want)
	}
}

// What goes to an address no connection follows, or one forgotten, goes at once: STUN, ICE's checks
// and a connection that ended are never held.
func TestPacer_LetsAnUnfollowedAddressGoAtOnce(t *testing.T) {
	t.Parallel()
	fake := clocktest.NewFake(time.Unix(1_800_000_000, 0))
	paced := newPacer(t.Context(), fake)
	followed := netip.MustParseAddrPort("192.0.2.7:50000")
	paced.follow(followed, func() (uint32, time.Duration) { return 1 << 10, 100 * time.Millisecond })
	paced.refresh()
	start := fake.Now()
	for range 1000 {
		paced.wait(net.UDPAddrFromAddrPort(netip.MustParseAddrPort("192.0.2.8:50000")), 1200)
	}
	if took := fake.Now().Sub(start); took != 0 {
		t.Errorf("1000 packets to an address no connection follows waited %v; want none", took)
	}
	paced.forget(followed)
	for range 1000 {
		paced.wait(net.UDPAddrFromAddrPort(netip.AddrPortFrom(netip.MustParseAddr("::ffff:192.0.2.7"), 50000)), 1200)
	}
	if took := fake.Now().Sub(start); took != 0 {
		t.Errorf("1000 packets to a forgotten address waited %v; want none", took)
	}
}

// A response's channel may hold the client's whole credit in flight before it waits, and waits
// down to no less than half of it: at 1 MiB and 256 KiB, the bound in flight, not the credit nor
// the window, held a paced channel to ~80 Mb/s at 50 ms (D397).
func TestBuffered_LetsTheClientsWholeCreditBeInFlight(t *testing.T) {
	t.Parallel()
	credit := InitialCreditMessages * MaxMessageBytes
	if bufferedHighBytes < credit {
		t.Errorf("a channel waits at %d bytes buffered; want at least the credit's %d", bufferedHighBytes, credit)
	}
	if bufferedLowBytes < credit/2 || bufferedLowBytes >= bufferedHighBytes {
		t.Errorf("a channel waits down to %d bytes; want at least %d and under %d", bufferedLowBytes,
			credit/2, bufferedHighBytes)
	}
}

// A send never reads its connection's window: SCTP sends holding its association's lock, which
// reading the window takes, and a send that read it hung the connection's close.
func TestPacer_ASendNeverReadsTheWindow(t *testing.T) {
	t.Parallel()
	paced := newPacer(t.Context(), clocktest.NewFake(time.Unix(1_800_000_000, 0)))
	// The window waits for the send to end, as reading SCTP's waits for the association's lock.
	sent := make(chan struct{})
	farEnd := netip.MustParseAddrPort("192.0.2.7:50000")
	ctx := t.Context()
	paced.follow(farEnd, func() (uint32, time.Duration) {
		select {
		case <-sent:
		case <-ctx.Done():
		}
		return 64 << 10, 20 * time.Millisecond
	})
	go func() {
		paced.wait(net.UDPAddrFromAddrPort(farEnd), 1200)
		close(sent)
	}()
	select {
	case <-sent:
	case <-time.After(5 * time.Second):
		t.Fatal("a send holding the association's lock waited on it")
	}
}
