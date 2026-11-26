// SPDX-License-Identifier: AGPL-3.0-or-later

package turnserver

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"syscall"
	"testing"
)

// A relay told to listen on port 0 lets the system choose again when the port UDP took is
// taken over TCP, and serves both on the port it ends with; a port it was given it never moves.
func TestListenOnOnePort_ChoosesAgainWhenTCPsPortIsTaken(t *testing.T) {
	t.Parallel()
	refusals := 2
	taken := func(ctx context.Context, network, address string) (net.Listener, error) {
		if refusals > 0 {
			refusals--
			return nil, &net.OpError{Op: "listen", Net: network, Err: syscall.EADDRINUSE}
		}
		return (&net.ListenConfig{}).Listen(ctx, network, address)
	}
	packets, streams, err := ListenOnOnePort(t.Context(), "127.0.0.1:0", taken)
	if err != nil {
		t.Fatalf("a port taken twice over TCP: %v", err)
	}
	t.Cleanup(func() {
		_ = packets.Close() //nolint:errcheck // the test is over
		_ = streams.Close() //nolint:errcheck // the test is over
	})
	udp := netip.MustParseAddrPort(packets.LocalAddr().String())
	tcp := netip.MustParseAddrPort(streams.Addr().String())
	if udp.Port() != tcp.Port() {
		t.Errorf("UDP on %v, TCP on %v: want one port", udp, tcp)
	}

	refusals = 1
	if _, _, err := ListenOnOnePort(t.Context(), "127.0.0.1:34780", taken); !errors.Is(err, syscall.EADDRINUSE) {
		t.Errorf("a named port taken over TCP: %v, want it refused", err)
	}
}
