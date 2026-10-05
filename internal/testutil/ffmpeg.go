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
	directory, whyAbsent := findFFmpeg(os.Getenv(FFmpegDirectoryVariable))
	Require(tb, FFmpegCapability, whyAbsent)
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
	// FFmpegStdout, when set, is what ffmpeg writes to stdout in place of Stdout, which ffprobe still
	// writes: a run whose output is not the probe's.
	FFmpegStdout string
	Stderr       string
	ExitStatus   int
	// HoldFIFOPath, when set, makes each run start a grandchild that holds stdout open and write
	// "ready" into this FIFO before waiting forever, so a test can cancel a run mid-flight.
	HoldFIFOPath string
	// Progress, when set, is what each run writes to its -progress pipe (descriptor 3) first: a
	// watched run's, which only a watched run has open.
	Progress string
	// Hang, when set, makes each run hang after its progress, as a stalled FFmpeg does
	// (FakeFFmpeg.HungPID).
	Hang FakeHang
}

// FakeHang is how a hanging fake ends.
type FakeHang string

// The ways a hanging fake ends.
const (
	// HangUntilQuit hangs until SIGINT, FFmpeg's graceful quit.
	HangUntilQuit FakeHang = "until-quit"
	// HangUntilKilled ignores SIGINT and SIGTERM, and hangs until SIGKILL.
	HangUntilKilled FakeHang = "until-killed"
)

// EBUR128Summary is how FFmpeg's ebur128 filter named name logs what it measured when it closes:
// a FakeReply's Stderr, for a test of reading a loudness measurement.
func EBUR128Summary(name, integratedLUFS, rangeLU, truePeakDBFS string) string {
	return "[ebur128@" + name + " @ 0x55d1] Summary:\n\n  Integrated loudness:\n    I:         " + integratedLUFS +
		" LUFS\n    Threshold: -45.2 LUFS\n\n  Loudness range:\n    LRA:        " + rangeLU +
		" LU\n    Threshold: -55.0 LUFS\n    LRA low:   -41.0 LUFS\n    LRA high:  -20.1 LUFS\n\n" +
		"  True peak:\n    Peak:      " + truePeakDBFS + " dBFS\n"
}

// FakeCall is one recorded run of a fake tool.
type FakeCall struct {
	Tool        string
	Arguments   []string
	Environment []string
	// Stdin is what the run read on its standard input: nothing, unless the run was fed.
	Stdin []byte
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
		"reply.status": []byte(strconv.Itoa(reply.ExitStatus)),
	}
	if reply.FFmpegStdout != "" {
		files["reply.ffmpeg.stdout"] = []byte(reply.FFmpegStdout)
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
	if reply.Progress != "" {
		writeFile(tb, filepath.Join(directory, "reply.progress"), []byte(reply.Progress), 0o600)
	}
	if reply.Hang != "" {
		writeFile(tb, filepath.Join(directory, "reply.hang"), []byte(reply.Hang), 0o600)
	}
	if reply.HoldFIFOPath != "" {
		err := os.Symlink(reply.HoldFIFOPath, filepath.Join(directory, "reply.hold"))
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
			Arguments:   splitNULTerminated(string(readFile(tb, record+".arguments"))),
			Environment: splitNonEmpty(string(readFile(tb, record+".environment")), "\n"),
			Stdin:       readFile(tb, record+".stdin"),
		})
	}
	return recorded
}

// GrandchildPID returns the pid of the process a held run started (FakeReply.HoldFIFOPath), and
// kills it when the test ends in case the code under test did not.
func (fake FakeFFmpeg) GrandchildPID(tb testing.TB) int {
	tb.Helper()
	return fake.recordedPID(tb, "grandchild.pid")
}

// recordedPID reads the pid the fake recorded in name, and kills that process when the test ends.
func (fake FakeFFmpeg) recordedPID(tb testing.TB, name string) int {
	tb.Helper()
	pid, err := strconv.Atoi(strings.TrimSpace(string(readFile(tb, filepath.Join(fake.Directory, name)))))
	if err != nil {
		tb.Fatalf("no pid recorded in %s: %v", name, err)
	}
	tb.Cleanup(func() {
		process, findErr := os.FindProcess(pid)
		if findErr == nil {
			_ = process.Kill() //nolint:errcheck // already dead is the expected case
		}
	})
	return pid
}

// HungPID returns the pid of a hanging run (FakeReply.Hang), and kills it when the test ends in
// case the code under test did not.
func (fake FakeFFmpeg) HungPID(tb testing.TB) int {
	tb.Helper()
	return fake.recordedPID(tb, "hung.pid")
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

// splitNULTerminated splits NUL-terminated arguments, keeping an empty one.
func splitNULTerminated(text string) []string {
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
