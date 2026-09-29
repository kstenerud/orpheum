// SPDX-License-Identifier: GPL-3.0-or-later

package execx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"slices"
	"syscall"
	"time"

	"github.com/kstenerud/orpheum/internal/node/confine"
)

// Tool names one of the bundled FFmpeg binaries.
type Tool string

// The tools execx runs, and nothing else.
const (
	FFmpeg  Tool = "ffmpeg"
	FFprobe Tool = "ffprobe"
)

// How much of a tool's stderr a failure carries: FFmpeg's last few lines, which name the cause.
const stderrTailBytes = 4096

// How long a cancelled run's pipes may stay open once its process group is killed, before
// Wait gives up on them.
const waitDelay = 10 * time.Second

// hardeningFlags is prepended to every run's arguments: no banner, and ffmpeg never reads stdin.
func hardeningFlags(tool Tool) ([]string, bool) {
	switch tool {
	case FFmpeg:
		return []string{"-nostdin", "-hide_banner"}, true
	case FFprobe:
		return []string{"-hide_banner"}, true
	}
	return nil, false
}

// FileInput is the input of a run that reads a file at an absolute path: named with the file:
// protocol, and only that protocol allowed, so a name such as "http:x" or "concat:a|b" is never
// read as anything but a file (cut-a §7).
func FileInput(absolutePath string) []string {
	return []string{"-protocol_whitelist", "file", "-i", "file:" + absolutePath}
}

// PipeInput is the input of a run that reads what it is given on stdin, with only the pipe
// protocol allowed.
func PipeInput() []string {
	return []string{"-protocol_whitelist", "pipe", "-i", "pipe:0"}
}

// ErrOutputTooLarge is returned when a run writes more to stdout than its caller allowed.
var ErrOutputTooLarge = errors.New("the tool wrote more to stdout than allowed")

// RunError is a run that failed: the exit status, or -1 when the process was killed, and the
// tail of what it wrote to stderr.
type RunError struct {
	Tool       Tool
	ExitStatus int
	StderrTail string
	Err        error
}

func (failure *RunError) Error() string {
	return fmt.Sprintf("%s failed (status %d): %v; stderr ends: %s",
		failure.Tool, failure.ExitStatus, failure.Err, failure.StderrTail)
}

func (failure *RunError) Unwrap() error { return failure.Err }

// Runner runs the bundled FFmpeg tools from one directory, each with an empty environment, in
// its own process group, from the root directory, with stdin closed unless a run feeds it. A
// confined runner starts each run as orpheum's shim, which confines itself to the run's access
// and then becomes the tool (cut-a §7).
type Runner struct {
	directory string
	// shim is orpheum's own executable, which each run re-executes to be confined; empty runs the
	// tool as it is, as a test's fake FFmpeg is.
	shim string
	// onStarted, when set by a test, is called once the process has started.
	onStarted func()
}

// NewRunner returns a runner for the tools in directory, which must be an absolute path, that
// runs them unconfined: a test's.
func NewRunner(directory string) (*Runner, error) {
	return NewConfinedRunner(directory, "")
}

// NewConfinedRunner returns a runner for the tools in directory that starts every run through
// shim, orpheum's own absolute path, so each run is confined to its access (cut-a §7). Both
// paths must be absolute; an empty shim runs the tools unconfined.
func NewConfinedRunner(directory, shim string) (*Runner, error) {
	if !filepath.IsAbs(directory) {
		return nil, fmt.Errorf("the FFmpeg directory must be an absolute path, not %q", directory)
	}
	if shim != "" && !filepath.IsAbs(shim) {
		return nil, fmt.Errorf("orpheum's own path must be absolute, not %q", shim)
	}
	return &Runner{directory: directory, shim: shim}, nil
}

// IsConfined is whether the runner starts its runs through the shim.
func (runner *Runner) IsConfined() bool { return runner.shim != "" }

// Run runs tool with arguments after the hardening flags, confined to access, and returns what
// it wrote to stdout, refusing more than stdoutLimit bytes. Cancelling ctx kills the tool and
// every process it started.
func (runner *Runner) Run(
	ctx context.Context, tool Tool, access confine.Access, arguments []string, stdoutLimitBytes int,
) ([]byte, error) {
	return runner.run(ctx, tool, access, arguments, nil, stdoutLimitBytes)
}

// RunInput is Run with input fed to the tool's stdin: an image the core sent, which reaches the
// tool as bytes on a pipe and never as a file name (D212). The caller restricts the tool to the
// pipe protocol.
func (runner *Runner) RunInput(
	ctx context.Context, tool Tool, access confine.Access, arguments []string, input []byte, stdoutLimitBytes int,
) ([]byte, error) {
	return runner.run(ctx, tool, access, arguments, input, stdoutLimitBytes)
}

// RunStreaming is Run for a long run whose stdout is read as it is written, such as FFmpeg's
// -progress lines: stdout goes to the writer, not into memory.
func (runner *Runner) RunStreaming(
	ctx context.Context, tool Tool, access confine.Access, arguments []string, stdout io.Writer,
) error {
	streams := pipes{stdout: stdout, stderrTailLimitBytes: stderrTailBytes}
	_, err := runner.runToExit(ctx, tool, access, arguments, streams)
	return err
}

// RunLogged is Run for a tool whose answer is in its log, not its output, as FFmpeg's ebur128
// filter writes its summary when it ends: stdout is discarded, and it returns the last logLimit
// bytes the tool wrote to stderr.
func (runner *Runner) RunLogged(
	ctx context.Context, tool Tool, access confine.Access, arguments []string, logLimitBytes int,
) (string, error) {
	streams := pipes{stdout: io.Discard, stderrTailLimitBytes: logLimitBytes}
	logged, err := runner.runToExit(ctx, tool, access, arguments, streams)
	if err != nil {
		return "", err
	}
	return logged.String(), nil
}

func (runner *Runner) run(
	ctx context.Context, tool Tool, access confine.Access, arguments []string, fed []byte, stdoutLimitBytes int,
) ([]byte, error) {
	stdout := &limitedBuffer{limitBytes: stdoutLimitBytes}
	stderrTail, err := runner.runToExit(ctx, tool, access, arguments,
		pipes{stdin: fed, stdout: stdout, stderrTailLimitBytes: stderrTailBytes})
	if err != nil {
		return nil, err
	}
	if stdout.isExceeded {
		return nil, &RunError{
			Tool: tool, ExitStatus: 0, StderrTail: stderrTail.String(),
			Err: fmt.Errorf("%w (%d bytes)", ErrOutputTooLarge, stdoutLimitBytes),
		}
	}
	return stdout.buffer.Bytes(), nil
}

// pipes are a run's streams: what its stdin is fed, nil to leave it closed; where its stdout goes;
// and how much of the end of its stderr is kept.
type pipes struct {
	stdin                []byte
	stdout               io.Writer
	stderrTailLimitBytes int
}

// runToExit runs tool to its end, with its streams as streams says, and returns the end of its stderr.
func (runner *Runner) runToExit(
	ctx context.Context, tool Tool, access confine.Access, arguments []string, streams pipes,
) (*tailBuffer, error) {
	hardened, known := hardeningFlags(tool)
	if !known {
		return nil, fmt.Errorf("execx runs only ffmpeg and ffprobe, not %q", tool)
	}
	path, argv := filepath.Join(runner.directory, string(tool)), slices.Concat(hardened, arguments)
	if runner.shim != "" {
		spec, err := confine.EncodeSpec(confine.Spec{Tool: path, Arguments: argv, Access: access})
		if err != nil {
			return nil, err //nolint:wrapcheck // confine says which path it refused
		}
		path, argv = runner.shim, []string{confine.ShimArgument, spec}
	}
	//nolint:gosec // the one sanctioned start: a configured directory, a fixed tool name, and argv as a slice
	command := exec.CommandContext(ctx, path, argv...)
	command.Env = []string{}
	command.Dir = "/"
	stderr := &tailBuffer{limitBytes: streams.stderrTailLimitBytes}
	command.Stdout = streams.stdout
	command.Stderr = stderr
	if streams.stdin != nil {
		command.Stdin = bytes.NewReader(streams.stdin)
	}
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Cancel = func() error {
		// The negative pid names the process group: the tool and anything it started.
		return syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
	}
	command.WaitDelay = waitDelay

	if err := command.Start(); err != nil {
		return nil, &RunError{Tool: tool, ExitStatus: -1, Err: err}
	}
	if runner.onStarted != nil {
		runner.onStarted()
	}
	err := command.Wait()
	switch {
	case ctx.Err() != nil:
		// Keep Wait's error too: exec.ErrWaitDelay there means something outlived the kill.
		return nil, &RunError{Tool: tool, ExitStatus: -1, StderrTail: stderr.String(), Err: errors.Join(ctx.Err(), err)}
	case err != nil:
		status := command.ProcessState.ExitCode()
		return nil, &RunError{Tool: tool, ExitStatus: status, StderrTail: stderr.String(), Err: err}
	}
	return stderr, nil
}

// limitedBuffer keeps up to limit bytes and notes whether more came. It never refuses a write,
// so a tool is never left blocked on a full pipe.
type limitedBuffer struct {
	buffer     bytes.Buffer
	limitBytes int
	isExceeded bool
}

func (limited *limitedBuffer) Write(data []byte) (int, error) {
	written := len(data)
	room := limited.limitBytes - limited.buffer.Len()
	if written > room {
		limited.isExceeded = true
		data = data[:max(room, 0)]
	}
	_, _ = limited.buffer.Write(data)
	return written, nil
}

// tailBuffer keeps the last limit bytes written to it.
type tailBuffer struct {
	data       []byte
	limitBytes int
}

func (tail *tailBuffer) Write(data []byte) (int, error) {
	tail.data = append(tail.data, data...)
	if excess := len(tail.data) - tail.limitBytes; excess > 0 {
		tail.data = tail.data[excess:]
	}
	return len(data), nil
}

func (tail *tailBuffer) String() string { return string(tail.data) }
