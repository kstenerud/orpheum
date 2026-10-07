// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !linux

package pacing

import (
	"errors"
	"syscall"
)

// unpaced is no pacing rate, where the kernel offers none.
const unpaced = ^uint32(0)

// setSocket does nothing where the kernel offers no pacing: the film's download goes as it would.
func setSocket(syscall.RawConn, uint32, bool) error { return nil }

// setBBR refuses where the kernel offers no BBR.
func setBBR(syscall.RawConn) error { return errors.New("send with BBR: not on this system") }
