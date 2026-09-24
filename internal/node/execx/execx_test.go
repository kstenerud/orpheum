// SPDX-License-Identifier: GPL-3.0-or-later

package execx

import (
	"errors"
	"slices"
	"strings"
	"testing"

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
	stdout, err := runner.Run(t.Context(), FFprobe, arguments, 1024)
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

	if _, err := runner.Run(t.Context(), FFmpeg, []string{"-version"}, 1024); err != nil {
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
		Stderr: noise + "/media/a.mkv: Invalid data found when processing input\n",
		Status: 183,
	})
	_, err := newRunner(t, fake).Run(t.Context(), FFprobe, []string{"-i", "file:/media/a.mkv"}, 1024)

	failure, isRunError := errors.AsType[*RunError](err)
	if !isRunError {
		t.Fatalf("error = %v, want a *RunError", err)
	}
	if failure.Status != 183 || failure.Tool != FFprobe {
		t.Errorf("status %d from %s, want 183 from ffprobe", failure.Status, failure.Tool)
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
	if _, err := runner.Run(t.Context(), FFprobe, nil, 10); !errors.Is(err, ErrOutputTooLarge) {
		t.Errorf("100 bytes against a limit of 10: error = %v, want ErrOutputTooLarge", err)
	}
	stdout, err := runner.Run(t.Context(), FFprobe, nil, 100)
	if err != nil || len(stdout) != 100 {
		t.Errorf("100 bytes against a limit of 100: %d bytes, %v; want all of it", len(stdout), err)
	}
}

func TestNewRunner_RefusesARelativeDirectory(t *testing.T) {
	t.Parallel()
	if _, err := NewRunner(".tools/ffmpeg"); err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Errorf("NewRunner(relative) = %v, want an absolute-path refusal", err)
	}
	fake := testutil.InstallFakeFFmpeg(t, testutil.FakeReply{})
	if _, err := newRunner(t, fake).Run(t.Context(), Tool("sh"), nil, 1); err == nil {
		t.Error("Run(sh) succeeded; execx runs only ffmpeg and ffprobe")
	}
}
