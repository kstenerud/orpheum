// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build !linux

package ffstreamer

import (
	"errors"
	"os"
)

// sizePipe is Linux's alone: elsewhere a pipe keeps its size, and the protocol works, only slower.
func sizePipe(*os.File, int) error {
	return errors.New("a pipe's size is set on Linux only")
}
