// SPDX-License-Identifier: GPL-3.0-or-later

package testutil

import (
	"os"
	"os/exec"
	"testing"
)

// RunSelf runs this test binary again, as a process of its own, with arguments, and returns what
// it wrote to stdout and stderr, and how it exited: for a test whose code must run in a separate
// process, as the escape test's confined tool does (D112).
func RunSelf(tb testing.TB, arguments ...string) ([]byte, error) {
	tb.Helper()
	self, err := os.Executable()
	if err != nil {
		tb.Fatal(err)
	}
	//nolint:gosec // G204: this test binary, argv as a slice
	command := exec.CommandContext(tb.Context(), self, arguments...)
	return command.CombinedOutput()
}
