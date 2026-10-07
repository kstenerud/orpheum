// SPDX-License-Identifier: GPL-3.0-or-later

package pacing

import (
	"net"
	"syscall"
)

// ListenConfig is a listen config whose sockets send with BBR, which the connections they accept
// inherit (D263 §3): only the sender's congestion control counts, and BBR, unlike the kernel's
// default CUBIC, does not read random loss as congestion, so a film keeps its rate on a lossy link.
// A kernel without BBR keeps its default, and onRefused is told why.
func ListenConfig(onRefused func(error)) *net.ListenConfig {
	return &net.ListenConfig{Control: func(_, _ string, raw syscall.RawConn) error {
		if err := setBBR(raw); err != nil {
			onRefused(err)
		}
		return nil
	}}
}
