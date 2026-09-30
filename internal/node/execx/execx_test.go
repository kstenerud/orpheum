// SPDX-License-Identifier: GPL-3.0-or-later

package execx

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/testutil"
)

func newRunner(t *testing.T, fake testutil.FakeFFmpeg) *Runner {
	t.Helper()
	runner, err := NewRunner(fake.Directory)
	if err != nil {
		t.Fatal(err)
	}
	return runner
}

func TestRun_PassesHardenedArgumentsAndAnEmptyEnvironment(t *testing.T) {
	t.Parallel()
	fake := testutil.InstallFakeFFmpeg(t, testutil.FakeReply{Stdout: `{"streams":[]}`})
	runner := newRunner(t, fake)

	arguments := []string{"-of", "json", "-i", "file:/media/a b.mkv", ""}
	stdout, err := runner.Run(t.Context(), FFprobe, confine.Access{}, arguments, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if string(stdout) != `{"streams":[]}` {
		t.Errorf("stdout = %q", stdout)
	}
	call := fake.OnlyCall(t)
	wantArguments := []string{"-hide_banner", "-of", "json", "-i", "file:/media/a b.mkv", ""}
	if call.Tool != "ffprobe" || !slices.Equal(call.Arguments, wantArguments) {
		t.Errorf("ran %s %q, want ffprobe %q", call.Tool, call.Arguments, wantArguments)
	}
	// The fake's own /bin/sh adds PWD, which also shows the run started in /. Nothing else of
	// this test's environment (HOME, PATH, TMPDIR, ...) may reach the tool (D204).
	if !slices.Equal(call.Environment, []string{"PWD=/"}) {
		t.Errorf("the tool's environment is %q, want only the shell's PWD=/", call.Environment)
	}

	if _, err := runner.Run(t.Context(), FFmpeg, confine.Access{}, []string{"-version"}, 1024); err != nil {
		t.Fatal(err)
	}
	calls := fake.Calls(t)
	ffmpegCall := slices.IndexFunc(calls, func(call testutil.FakeCall) bool { return call.Tool == "ffmpeg" })
	if got := calls[ffmpegCall].Arguments; !slices.Equal(got, []string{"-nostdin", "-hide_banner", "-version"}) {
		t.Errorf("ffmpeg ran with %q, want -nostdin and -hide_banner first", got)
	}
}

func TestRun_AFailureCarriesTheStatusAndTheTailOfStderr(t *testing.T) {
	t.Parallel()
	noise := strings.Repeat("frame=1 fps=0.0 q=0.0\n", 400)
	fake := testutil.InstallFakeFFmpeg(t, testutil.FakeReply{
		Stderr:     noise + "/media/a.mkv: Invalid data found when processing input\n",
		ExitStatus: 183,
	})
	_, err := newRunner(t, fake).Run(t.Context(), FFprobe, confine.Access{}, []string{"-i", "file:/media/a.mkv"}, 1024)

	failure, isRunError := errors.AsType[*RunError](err)
	if !isRunError {
		t.Fatalf("error = %v, want a *RunError", err)
	}
	if failure.ExitStatus != 183 || failure.Tool != FFprobe {
		t.Errorf("status %d from %s, want 183 from ffprobe", failure.ExitStatus, failure.Tool)
	}
	if !strings.HasSuffix(failure.StderrTail, "Invalid data found when processing input\n") {
		t.Errorf("the stderr tail does not end with the cause: %q", failure.StderrTail)
	}
	if len(failure.StderrTail) > stderrTailBytes {
		t.Errorf("the stderr tail is %d bytes, want at most %d", len(failure.StderrTail), stderrTailBytes)
	}
	if !strings.Contains(err.Error(), "Invalid data found") {
		t.Errorf("the error does not say why: %v", err)
	}
}

func TestRun_RefusesMoreOutputThanAllowed(t *testing.T) {
	t.Parallel()
	fake := testutil.InstallFakeFFmpeg(t, testutil.FakeReply{Stdout: strings.Repeat("x", 100)})
	runner := newRunner(t, fake)
	if _, err := runner.Run(t.Context(), FFprobe, confine.Access{}, nil, 10); !errors.Is(err, ErrOutputTooLarge) {
		t.Errorf("100 bytes against a limit of 10: error = %v, want ErrOutputTooLarge", err)
	}
	stdout, err := runner.Run(t.Context(), FFprobe, confine.Access{}, nil, 100)
	if err != nil || len(stdout) != 100 {
		t.Errorf("100 bytes against a limit of 100: %d bytes, %v; want all of it", len(stdout), err)
	}
}

func TestRunWatched_ReturnsTheEndOfTheLogAndNotTheOutput(t *testing.T) {
	t.Parallel()
	noise := strings.Repeat("[aac @ 0x1] a warning early in the run\n", 400)
	fake := testutil.InstallFakeFFmpeg(t, testutil.FakeReply{
		Stdout: "output nobody reads", Stderr: noise + "[ebur128@stereo1 @ 0x2] Summary:\n    I: -23.0 LUFS\n",
	})
	arguments := []string{"-f", "null", "-"}
	ran, err := newRunner(t, fake).RunWatched(t.Context(), WatchedRun{
		Arguments: arguments, LogLimitBytes: 1024,
		Watch: Watch{StartGrace: time.Hour, StallWindow: time.Hour, Deadline: time.Hour},
	})
	if err != nil {
		t.Fatal(err)
	}
	logged := ran.Log
	if !strings.HasSuffix(logged, "Summary:\n    I: -23.0 LUFS\n") || len(logged) != 1024 ||
		strings.Contains(logged, "output nobody reads") {
		t.Errorf("logged %d bytes ending %q; want the last 1024 bytes of stderr, the summary last",
			len(logged), logged[max(0, len(logged)-60):])
	}
}

func TestNewRunner_RefusesARelativeDirectory(t *testing.T) {
	t.Parallel()
	if _, err := NewRunner(".tools/ffmpeg"); err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Errorf("NewRunner(relative) = %v, want an absolute-path refusal", err)
	}
	fake := testutil.InstallFakeFFmpeg(t, testutil.FakeReply{})
	if _, err := newRunner(t, fake).Run(t.Context(), Tool("sh"), confine.Access{}, nil, 1); err == nil {
		t.Error("Run(sh) succeeded; execx runs only ffmpeg and ffprobe")
	}
}

func TestRunInput_FeedsTheToolsStdinAndRunClosesIt(t *testing.T) {
	t.Parallel()
	fake := testutil.InstallFakeFFmpeg(t, testutil.FakeReply{Stdout: "done"})
	runner := newRunner(t, fake)
	_, err := runner.RunInput(t.Context(), FFmpeg, confine.Access{}, []string{"-i", "pipe:0"},
		[]byte("image bytes"), 1024)
	if err != nil {
		t.Fatal(err)
	}
	if got := fake.OnlyCall(t).Stdin; string(got) != "image bytes" {
		t.Errorf("the tool read %q on stdin", got)
	}
	if _, err := runner.Run(t.Context(), FFprobe, confine.Access{}, []string{"-version"}, 1024); err != nil {
		t.Fatal(err)
	}
	for _, call := range fake.Calls(t) {
		if call.Tool == "ffprobe" && len(call.Stdin) != 0 {
			t.Errorf("a run that was not fed read %q on stdin", call.Stdin)
		}
	}
}

func TestRunStreaming_WritesStdoutToTheWriterAndCarriesAFailure(t *testing.T) {
	t.Parallel()
	fake := testutil.InstallFakeFFmpeg(t, testutil.FakeReply{Stdout: "out_time_us=1000000\nprogress=end\n"})
	var progress strings.Builder
	err := newRunner(t, fake).RunStreaming(t.Context(), FFmpeg, confine.Access{}, []string{"-i", "x"}, &progress)
	if err != nil {
		t.Fatal(err)
	}
	if progress.String() != "out_time_us=1000000\nprogress=end\n" {
		t.Errorf("the writer got %q, want the tool's stdout", progress.String())
	}
	failing := testutil.InstallFakeFFmpeg(t, testutil.FakeReply{Stderr: "No such file", ExitStatus: 1})
	err = newRunner(t, failing).RunStreaming(t.Context(), FFmpeg, confine.Access{}, []string{"-i", "x"}, &progress)
	var failure *RunError
	if !errors.As(err, &failure) || failure.ExitStatus != 1 || !strings.Contains(failure.StderrTail, "No such file") {
		t.Errorf("a failing run = %v, want a RunError with its status and stderr", err)
	}
}

func TestRun_AConfinedRunnerStartsTheShimWithTheToolItsArgumentsAndItsAccess(t *testing.T) {
	t.Parallel()
	fake := testutil.InstallFakeFFmpeg(t, testutil.FakeReply{Stdout: "done"})
	// The fake ffmpeg stands in for orpheum's shim, and records how it was started.
	runner, err := NewConfinedRunner(fake.Directory, filepath.Join(fake.Directory, "ffmpeg"))
	if err != nil {
		t.Fatal(err)
	}
	if !runner.IsConfined() {
		t.Error("a runner with a shim says it is not confined")
	}
	access := confine.Access{Read: []string{"/media/a.mkv"}}
	if _, err := runner.Run(t.Context(), FFprobe, access, []string{"-i", "file:/media/a.mkv"}, 1024); err != nil {
		t.Fatal(err)
	}
	call := fake.OnlyCall(t)
	if len(call.Arguments) != 2 || call.Arguments[0] != confine.ShimArgument {
		t.Fatalf("the shim was started with %q, want %s and the run's spec", call.Arguments, confine.ShimArgument)
	}
	var spec confine.Spec
	if err := json.Unmarshal([]byte(call.Arguments[1]), &spec); err != nil {
		t.Fatal(err)
	}
	want := confine.Spec{
		Tool: filepath.Join(fake.Directory, "ffprobe"), Arguments: []string{"-hide_banner", "-i", "file:/media/a.mkv"},
		Access: access,
	}
	if spec.Tool != want.Tool || !slices.Equal(spec.Arguments, want.Arguments) ||
		!slices.Equal(spec.Access.Read, want.Access.Read) {
		t.Errorf("the shim was asked for %+v, want %+v", spec, want)
	}
	if _, err := NewConfinedRunner(fake.Directory, "orpheum"); err == nil {
		t.Error("a relative shim path was accepted")
	}
}
