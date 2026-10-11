// SPDX-License-Identifier: AGPL-3.0-or-later

package pacing

import (
	"context"
	"net"
	"os"
	"slices"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

// acceptedOnLoopback is the server's side of a TCP connection over the loopback.
func acceptedOnLoopback(t *testing.T) net.Conn {
	t.Helper()
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() }) //nolint:errcheck // test teardown
	client, err := (&net.Dialer{}).DialContext(t.Context(), "tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() }) //nolint:errcheck // test teardown
	server, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close() }) //nolint:errcheck // test teardown
	return server
}

// socketOptions are the connection's pacing rate and IPv4 traffic class, as the kernel has them.
func socketOptions(t *testing.T, conn net.Conn) (rate uint32, class int) {
	t.Helper()
	tcp, isTCP := conn.(*net.TCPConn)
	if !isTCP {
		t.Fatalf("%T is not TCP", conn)
	}
	raw, err := tcp.SyscallConn()
	if err != nil {
		t.Fatal(err)
	}
	var rateErr, classErr error
	var rateValue int
	if err := raw.Control(func(fd uintptr) {
		socket := int(fd)
		rateValue, rateErr = unix.GetsockoptInt(socket, unix.SOL_SOCKET, unix.SO_MAX_PACING_RATE)
		class, classErr = unix.GetsockoptInt(socket, unix.IPPROTO_IP, unix.IP_TOS)
	}); err != nil || rateErr != nil || classErr != nil {
		t.Fatal(err, rateErr, classErr)
	}
	return uint32(rateValue), class //nolint:gosec // the kernel's unsigned rate, read back as it was set
}

func TestPace_PacesTheConnectionAndMarksItLowerEffortAndUnpaceSetsItBack(t *testing.T) {
	t.Parallel()
	conn := acceptedOnLoopback(t)
	ctx := ConnContext(t.Context(), conn)
	if err := Pace(ctx, 250_000); err != nil {
		t.Fatal(err)
	}
	if rate, class := socketOptions(t, conn); rate != 250_000 || class != lowerEffort {
		t.Errorf("paced: rate %d, class %#x; want 250000 bytes a second, DSCP LE (%#x)", rate, class, lowerEffort)
	}
	if err := Unpace(ctx); err != nil {
		t.Fatal(err)
	}
	if rate, class := socketOptions(t, conn); rate != unpaced || class != 0 {
		t.Errorf("unpaced: rate %d, class %#x; want no pacing (%d) and best effort", rate, class, unpaced)
	}
}

func TestPace_LeavesARequestWithNoKeptConnectionAlone(t *testing.T) {
	t.Parallel()
	piped, other := net.Pipe()
	t.Cleanup(func() { _, _ = piped.Close(), other.Close() }) //nolint:errcheck // test teardown
	for _, ctx := range []context.Context{t.Context(), ConnContext(t.Context(), piped)} {
		if err := Pace(ctx, 1000); err != nil {
			t.Errorf("pacing with no socket: %v, want nothing done", err)
		}
	}
}

func TestListenConfig_AcceptsConnectionsThatSendWithBBRWhereTheKernelHasIt(t *testing.T) {
	t.Parallel()
	available, err := os.ReadFile("/proc/sys/net/ipv4/tcp_available_congestion_control")
	if err != nil {
		t.Fatal(err)
	}
	hasBBR := slices.Contains(strings.Fields(string(available)), "bbr")
	var refused []error
	listener, err := ListenConfig(func(err error) { refused = append(refused, err) }).
		Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() }) //nolint:errcheck // test teardown
	client, err := (&net.Dialer{}).DialContext(t.Context(), "tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() }) //nolint:errcheck // test teardown
	accepted, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = accepted.Close() }) //nolint:errcheck // test teardown
	tcp, isTCP := accepted.(*net.TCPConn)
	if !isTCP {
		t.Fatalf("%T is not TCP", accepted)
	}
	raw, err := tcp.SyscallConn()
	if err != nil {
		t.Fatal(err)
	}
	var congestion string
	var getErr error
	if err := raw.Control(func(fd uintptr) {
		congestion, getErr = unix.GetsockoptString(int(fd), unix.IPPROTO_TCP, unix.TCP_CONGESTION)
	}); err != nil || getErr != nil {
		t.Fatal(err, getErr)
	}
	if hasBBR && (congestion != "bbr" || len(refused) != 0) {
		t.Errorf("with BBR in the kernel, the accepted connection sends with %q (refused %v); want bbr",
			congestion, refused)
	}
	if !hasBBR && (congestion == "bbr" || len(refused) != 1) {
		t.Errorf("without BBR in the kernel: %q, refused %v; want the default, refused once", congestion, refused)
	}
}
