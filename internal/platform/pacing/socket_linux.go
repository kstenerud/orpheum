// SPDX-License-Identifier: GPL-3.0-or-later

package pacing

import (
	"errors"
	"fmt"
	"syscall"

	"golang.org/x/sys/unix"
)

// unpaced is the kernel's word for no pacing rate: ~0U.
const unpaced = ^uint32(0)

// lowerEffort is DSCP LE, 1, in the traffic class byte's top six bits (RFC 8622).
const lowerEffort = 1 << 2

// setSocket sets the socket's pacing rate and its traffic class: lower effort, or best effort (0),
// for IPv4 and, on an IPv6 socket, IPv6 alike.
func setSocket(raw syscall.RawConn, bytesPerSecond uint32, isLowerEffort bool) error {
	class := 0
	if isLowerEffort {
		class = lowerEffort
	}
	rate := int(int32(bytesPerSecond)) //nolint:gosec // the kernel reads it back as the unsigned it was
	var set error
	controlErr := raw.Control(func(fd uintptr) {
		socket := int(fd)
		set = unix.SetsockoptInt(socket, unix.SOL_SOCKET, unix.SO_MAX_PACING_RATE, rate)
		v4 := unix.SetsockoptInt(socket, unix.IPPROTO_IP, unix.IP_TOS, class)
		v6 := unix.SetsockoptInt(socket, unix.IPPROTO_IPV6, unix.IPV6_TCLASS, class)
		if v4 != nil && v6 != nil {
			set = errors.Join(set, v4)
		}
	})
	if err := errors.Join(controlErr, set); err != nil {
		return fmt.Errorf("pace a connection: %w", err)
	}
	return nil
}

// setBBR asks the kernel to send on the socket with BBR; ENOENT when the kernel has not loaded it.
func setBBR(raw syscall.RawConn) error {
	var set error
	if err := raw.Control(func(fd uintptr) {
		set = unix.SetsockoptString(int(fd), unix.IPPROTO_TCP, unix.TCP_CONGESTION, "bbr")
	}); err != nil {
		return fmt.Errorf("send with BBR: %w", err)
	}
	if set != nil {
		return fmt.Errorf("send with BBR: %w", set)
	}
	return nil
}
