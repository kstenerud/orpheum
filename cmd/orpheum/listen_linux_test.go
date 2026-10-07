// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bytes"
	"log/slog"
	"net"
	"os"
	"slices"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

// The core's clients' connections send with BBR where the kernel has it, and the log says so once
// where it has not (D263 §3).
func TestListenForMedia_ItsConnectionsSendWithBBRWhereTheKernelHasIt(t *testing.T) {
	t.Parallel()
	available, err := os.ReadFile("/proc/sys/net/ipv4/tcp_available_congestion_control")
	if err != nil {
		t.Fatal(err)
	}
	hasBBR := slices.Contains(strings.Fields(string(available)), "bbr")
	var logged bytes.Buffer
	listener, err := listenForMedia(t.Context(), "127.0.0.1:0", slog.New(slog.NewTextHandler(&logged, nil)))
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
	isRefusalLogged := strings.Contains(logged.String(), "not BBR")
	if hasBBR != (congestion == "bbr") || hasBBR == isRefusalLogged {
		t.Errorf("BBR in the kernel %t: the connection sends with %q, refusal logged %t:\n%s",
			hasBBR, congestion, isRefusalLogged, logged.String())
	}
}
