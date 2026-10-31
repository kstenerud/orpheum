// SPDX-License-Identifier: AGPL-3.0-or-later

package testutil

import (
	"bytes"
	"os/exec"
	"testing"
)

// RunShell runs script with sh, its arguments after it, in environment alone (PATH among them, so a
// test puts its fakes of the tools the script calls first), and returns what it wrote to stdout and
// stderr, and how it exited: for a test of a shell script a household runs, as the install's
// bootstrap is (the root key audit's §3.9).
func RunShell(tb testing.TB, script string, arguments, environment []string) ([]byte, error) {
	tb.Helper()
	//nolint:gosec // G204: sh, the script on its standard input, argv as a slice
	command := exec.CommandContext(tb.Context(), "sh", append([]string{"-s", "--"}, arguments...)...)
	command.Stdin = bytes.NewBufferString(script)
	command.Env = environment
	return command.CombinedOutput()
}
