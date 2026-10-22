// SPDX-License-Identifier: AGPL-3.0-or-later

package ffstreamer

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// sizePipe asks for a pipe of size bytes, through SyscallConn's Control and never Fd, which would
// put the descriptor in blocking mode, and then closing it would no longer free a blocked writer
// (§4.3).
func sizePipe(pipe *os.File, size int) error {
	raw, err := pipe.SyscallConn()
	if err != nil {
		return fmt.Errorf("reach the pipe's descriptor: %w", err)
	}
	var sizeErr error
	if err := raw.Control(func(descriptor uintptr) {
		_, sizeErr = unix.FcntlInt(descriptor, unix.F_SETPIPE_SZ, size)
	}); err != nil {
		return fmt.Errorf("reach the pipe's descriptor: %w", err)
	}
	if sizeErr != nil {
		return fmt.Errorf("make the data pipe %d bytes: %w", size, sizeErr)
	}
	return nil
}
