// SPDX-License-Identifier: GPL-3.0-or-later

package testutil

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// MKVToolNixDirectoryVariable names the extracted MKVToolNix the integration tier edits
// Matroska files with. The Makefile's integration target sets it to what
// scripts/fetch_mkvtoolnix.py fetched.
const MKVToolNixDirectoryVariable = "ORPHEUM_TEST_MKVTOOLNIX_DIR"

// MKVToolNixCapability is the name a job lists in ORPHEUM_TEST_UNCONTROLLED to run without it.
const MKVToolNixCapability = "mkvtoolnix"

// MKVToolNix runs the pinned mkvmerge and mkvpropedit for a test. The node never runs them: it
// only reads media, and execx refuses anything but ffmpeg and ffprobe. That is why this lives
// here, in a package only tests may import (TestArch_TestutilIsImportedOnlyByTests).
type MKVToolNix struct {
	directory string
}

// RequireMKVToolNix returns the pinned MKVToolNix. Without it the test fails, unless the job
// lists "mkvtoolnix" in ORPHEUM_TEST_UNCONTROLLED (D190 §2).
func RequireMKVToolNix(tb testing.TB) MKVToolNix {
	tb.Helper()
	directory := os.Getenv(MKVToolNixDirectoryVariable)
	var absent error
	if directory == "" {
		absent = fmt.Errorf("%s is not set; MKVToolNix is pinned for linux/amd64 only, and "+
			"`make test-integration` fetches it there", MKVToolNixDirectoryVariable)
	} else if _, err := os.Stat(filepath.Join(directory, "usr", "bin", "mkvpropedit")); err != nil { //nolint:gosec,lll // the job's own MKVToolNix directory
		absent = fmt.Errorf("%s=%s: %w", MKVToolNixDirectoryVariable, directory, err)
	}
	Require(tb, MKVToolNixCapability, absent)
	return MKVToolNix{directory: directory}
}

// Run runs one MKVToolNix tool ("mkvmerge", "mkvpropedit") with arguments, and fails the test
// with the tool's output if it fails. Its environment holds only the path to its own libraries.
func (tools MKVToolNix) Run(tb testing.TB, tool string, arguments ...string) {
	tb.Helper()
	command := exec.CommandContext(tb.Context(), filepath.Join(tools.directory, "usr", "bin", tool), arguments...) //nolint:gosec,lll // the pinned tool, argv as a slice
	command.Env = []string{"LD_LIBRARY_PATH=" + filepath.Join(tools.directory, "usr", "lib")}
	command.Dir = "/"
	output, err := command.CombinedOutput()
	if err != nil {
		tb.Fatalf("%s %q: %v\n%s", tool, arguments, err, output)
	}
}
