// SPDX-License-Identifier: AGPL-3.0-or-later

package turnserver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"syscall"
)

// portChoices is how many ports the system may choose before a relay listening on port 0 gives up:
// UDP's chosen port can be taken over TCP by someone else.
const portChoices = 5

// ListenOnOnePort listens on address over UDP, then over TCP (with listenTCP) on the port UDP took.
// When address leaves the port to the system and that port is taken over TCP, it lets the system
// choose again, up to portChoices times; a port named in address is taken or refused as it is.
func ListenOnOnePort(ctx context.Context, address string,
	listenTCP func(context.Context, string, string) (net.Listener, error),
) (net.PacketConn, net.Listener, error) {
	_, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, nil, fmt.Errorf("TURN's listen address %q: %w", address, err)
	}
	for choice := 1; ; choice++ {
		packets, err := (&net.ListenConfig{}).ListenPacket(ctx, "udp", address)
		if err != nil {
			return nil, nil, fmt.Errorf("listen for TURN on UDP %s: %w", address, err)
		}
		streams, err := listenTCP(ctx, "tcp", packets.LocalAddr().String())
		if err == nil {
			return packets, streams, nil
		}
		_ = packets.Close() //nolint:errcheck // the listen's error is the one to report
		if port != "0" && port != "" || !errors.Is(err, syscall.EADDRINUSE) || choice == portChoices {
			return nil, nil, fmt.Errorf("listen for TURN on TCP %s: %w", packets.LocalAddr(), err)
		}
	}
}
