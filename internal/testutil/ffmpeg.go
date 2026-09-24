// SPDX-License-Identifier: GPL-3.0-or-later

package testutil

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// FFmpegDirectoryVariable names the directory holding the pinned ffmpeg and ffprobe. The
// Makefile's integration target sets it to what `make ffmpeg` fetched.
const FFmpegDirectoryVariable = "ORPHEUM_TEST_FFMPEG_DIR"

// FFmpegCapability is the name a job lists in ORPHEUM_TEST_UNCONTROLLED to run without FFmpeg.
const FFmpegCapability = "ffmpeg"

// FFmpegDirectory returns the directory holding the pinned ffmpeg and ffprobe. Without it the
// test fails, unless the job lists "ffmpeg" in ORPHEUM_TEST_UNCONTROLLED (D190 §2).
func FFmpegDirectory(tb testing.TB) string {
	tb.Helper()
	directory, absent := findFFmpeg(os.Getenv(FFmpegDirectoryVariable))
	Require(tb, FFmpegCapability, absent)
	return directory
}

func findFFmpeg(directory string) (string, error) {
	if directory == "" {
		return "", fmt.Errorf("%s is not set; run the integration tier with `make test-integration`",
			FFmpegDirectoryVariable)
	}
	for _, tool := range []string{"ffmpeg", "ffprobe"} {
		info, err := os.Stat(filepath.Join(directory, tool)) //nolint:gosec // the job's own FFmpeg directory
		if err != nil {
			return "", fmt.Errorf("%s=%s: %w", FFmpegDirectoryVariable, directory, err)
		}
		if info.Mode()&0o111 == 0 {
			return "", fmt.Errorf("%s=%s: %s is not executable", FFmpegDirectoryVariable, directory, tool)
		}
	}
	return directory, nil
}

// fakeScript is the committed, executable fake. Each fake directory links to it rather than
// writing a copy: writing an executable while a parallel test forks lets the child inherit the
// open file, and running it then fails with "text file busy" (ETXTBSY).
func fakeScript(tb testing.TB) string {
	tb.Helper()
	_, source, _, found := runtime.Caller(0)
	if !found {
		tb.Fatal("cannot locate internal/testutil's source to find the fake ffmpeg")
	}
	return filepath.Join(filepath.Dir(source), "testdata", "fake-ffmpeg.sh")
}

// FakeReply is what every run of a fake tool writes and the status it exits with.
type FakeReply struct {
	Stdout string
	Stderr string
	Status int
	// HoldFIFO, when set, makes each run start a grandchild that holds stdout open and write
	// "ready" into this FIFO before waiting forever, so a test can cancel a run mid-flight.
	HoldFIFO string
}

// FakeCall is one recorded run of a fake tool.
type FakeCall struct {
	Tool        string
	Arguments   []string
	Environment []string
}

// FakeFFmpeg is a directory holding fake ffmpeg and ffprobe executables (testing.md §11).
type FakeFFmpeg struct {
	Directory string
}

// InstallFakeFFmpeg links fake ffmpeg and ffprobe into a new temporary directory. Every run
// records its arguments and environment, then answers with reply.
func InstallFakeFFmpeg(tb testing.TB, reply FakeReply) FakeFFmpeg {
	tb.Helper()
	directory := tb.TempDir()
	files := map[string][]byte{
		"reply.stdout": []byte(reply.Stdout),
		"reply.stderr": []byte(reply.Stderr),
		"reply.status": []byte(strconv.Itoa(reply.Status)),
	}
	for name, contents := range files {
		writeFile(tb, filepath.Join(directory, name), contents, 0o600)
	}
	script := fakeScript(tb)
	for _, tool := range []string{"ffmpeg", "ffprobe"} {
		if err := os.Symlink(script, filepath.Join(directory, tool)); err != nil {
			tb.Fatal(err)
		}
	}
	if reply.HoldFIFO != "" {
		err := os.Symlink(reply.HoldFIFO, filepath.Join(directory, "reply.hold"))
		if err != nil {
			tb.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(directory, "calls"), 0o700); err != nil {
		tb.Fatal(err)
	}
	return FakeFFmpeg{Directory: directory}
}

// Calls returns every recorded run, ordered by tool name and then by process id.
func (fake FakeFFmpeg) Calls(tb testing.TB) []FakeCall {
	tb.Helper()
	calls := filepath.Join(fake.Directory, "calls")
	names, err := filepath.Glob(filepath.Join(calls, "*.tool"))
	if err != nil {
		tb.Fatal(err)
	}
	slices.Sort(names)
	recorded := make([]FakeCall, 0, len(names))
	for _, name := range names {
		record := strings.TrimSuffix(name, ".tool")
		recorded = append(recorded, FakeCall{
			Tool:        strings.TrimSpace(string(readFile(tb, name))),
			Arguments:   terminated(string(readFile(tb, record+".arguments"))),
			Environment: splitNonEmpty(string(readFile(tb, record+".environment")), "\n"),
		})
	}
	return recorded
}

// Grandchild returns the pid of the process a held run started (FakeReply.HoldFIFO), and
// kills it when the test ends in case the code under test did not.
func (fake FakeFFmpeg) Grandchild(tb testing.TB) int {
	tb.Helper()
	pid, err := strconv.Atoi(strings.TrimSpace(string(readFile(tb, filepath.Join(fake.Directory, "grandchild.pid")))))
	if err != nil {
		tb.Fatalf("no grandchild pid recorded: %v", err)
	}
	tb.Cleanup(func() {
		process, findErr := os.FindProcess(pid)
		if findErr == nil {
			_ = process.Kill() //nolint:errcheck // already dead is the expected case
		}
	})
	return pid
}

// OnlyCall returns the one recorded run, failing the test unless there was exactly one.
func (fake FakeFFmpeg) OnlyCall(tb testing.TB) FakeCall {
	tb.Helper()
	calls := fake.Calls(tb)
	if len(calls) != 1 {
		tb.Fatalf("the fake ffmpeg ran %d times, want once: %+v", len(calls), calls)
	}
	return calls[0]
}

// terminated splits NUL-terminated arguments, keeping an empty one.
func terminated(text string) []string {
	if text == "" {
		return []string{}
	}
	return strings.Split(strings.TrimSuffix(text, "\x00"), "\x00")
}

func splitNonEmpty(text, separator string) []string {
	parts := strings.Split(text, separator)
	return slices.DeleteFunc(parts, func(part string) bool { return part == "" })
}

func writeFile(tb testing.TB, path string, contents []byte, mode os.FileMode) {
	tb.Helper()
	if err := os.WriteFile(path, contents, mode); err != nil {
		tb.Fatal(err)
	}
}

func readFile(tb testing.TB, path string) []byte {
	tb.Helper()
	contents, err := os.ReadFile(path) //nolint:gosec // a file the fake wrote in the test's directory
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		tb.Fatal(err)
	}
	return contents
}
