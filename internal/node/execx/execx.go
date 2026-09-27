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

// hardening is prepended to every run's arguments: no banner, and ffmpeg never reads stdin.
func hardening(tool Tool) ([]string, bool) {
	switch tool {
	case FFmpeg:
		return []string{"-nostdin", "-hide_banner"}, true
	case FFprobe:
		return []string{"-hide_banner"}, true
	}
	return nil, false
}

// ErrOutputTooLarge is returned when a run writes more to stdout than its caller allowed.
var ErrOutputTooLarge = errors.New("the tool wrote more to stdout than allowed")

// RunError is a run that failed: the exit status, or -1 when the process was killed, and the
// tail of what it wrote to stderr.
type RunError struct {
	Tool       Tool
	Status     int
	StderrTail string
	Err        error
}

func (failure *RunError) Error() string {
	return fmt.Sprintf("%s failed (status %d): %v; stderr ends: %s",
		failure.Tool, failure.Status, failure.Err, failure.StderrTail)
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
	// started, when set by a test, is called once the process has started.
	started func()
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
	ctx context.Context, tool Tool, access confine.Access, arguments []string, stdoutLimit int,
) ([]byte, error) {
	return runner.run(ctx, tool, access, arguments, nil, stdoutLimit)
}

// RunInput is Run with input fed to the tool's stdin: an image the core sent, which reaches the
// tool as bytes on a pipe and never as a file name (D212). The caller restricts the tool to the
// pipe protocol.
func (runner *Runner) RunInput(
	ctx context.Context, tool Tool, access confine.Access, arguments []string, input []byte, stdoutLimit int,
) ([]byte, error) {
	return runner.run(ctx, tool, access, arguments, input, stdoutLimit)
}

// RunStreaming is Run for a long run whose stdout is read as it is written, such as FFmpeg's
// -progress lines: stdout goes to the writer, not into memory.
func (runner *Runner) RunStreaming(
	ctx context.Context, tool Tool, access confine.Access, arguments []string, stdout io.Writer,
) error {
	_, err := runner.start(ctx, tool, access, arguments, nil, stdout)
	return err
}

func (runner *Runner) run(
	ctx context.Context, tool Tool, access confine.Access, arguments []string, fed []byte, stdoutLimit int,
) ([]byte, error) {
	stdout := &limitedBuffer{limit: stdoutLimit}
	finished, err := runner.start(ctx, tool, access, arguments, fed, stdout)
	if err != nil {
		return nil, err
	}
	if stdout.exceeded {
		return nil, &RunError{
			Tool: tool, Status: 0, StderrTail: finished.String(),
			Err: fmt.Errorf("%w (%d bytes)", ErrOutputTooLarge, stdoutLimit),
		}
	}
	return stdout.buffer.Bytes(), nil
}

// start runs tool to its end, writing its stdout to stdout, and returns the tail of its stderr.
func (runner *Runner) start(
	ctx context.Context, tool Tool, access confine.Access, arguments []string, fed []byte, stdout io.Writer,
) (*tailBuffer, error) {
	hardened, known := hardening(tool)
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
	stderr := &tailBuffer{limit: stderrTailBytes}
	command.Stdout = stdout
	command.Stderr = stderr
	if fed != nil {
		command.Stdin = bytes.NewReader(fed)
	}
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Cancel = func() error {
		// The negative pid names the process group: the tool and anything it started.
		return syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
	}
	command.WaitDelay = waitDelay

	if err := command.Start(); err != nil {
		return nil, &RunError{Tool: tool, Status: -1, Err: err}
	}
	if runner.started != nil {
		runner.started()
	}
	err := command.Wait()
	switch {
	case ctx.Err() != nil:
		// Keep Wait's error too: exec.ErrWaitDelay there means something outlived the kill.
		return nil, &RunError{Tool: tool, Status: -1, StderrTail: stderr.String(), Err: errors.Join(ctx.Err(), err)}
	case err != nil:
		status := command.ProcessState.ExitCode()
		return nil, &RunError{Tool: tool, Status: status, StderrTail: stderr.String(), Err: err}
	}
	return stderr, nil
}

// limitedBuffer keeps up to limit bytes and notes whether more came. It never refuses a write,
// so a tool is never left blocked on a full pipe.
type limitedBuffer struct {
	buffer   bytes.Buffer
	limit    int
	exceeded bool
}

func (limited *limitedBuffer) Write(data []byte) (int, error) {
	written := len(data)
	room := limited.limit - limited.buffer.Len()
	if written > room {
		limited.exceeded = true
		data = data[:max(room, 0)]
	}
	_, _ = limited.buffer.Write(data)
	return written, nil
}

// tailBuffer keeps the last limit bytes written to it.
type tailBuffer struct {
	data  []byte
	limit int
}

func (tail *tailBuffer) Write(data []byte) (int, error) {
	tail.data = append(tail.data, data...)
	if excess := len(tail.data) - tail.limit; excess > 0 {
		tail.data = tail.data[excess:]
	}
	return len(data), nil
}

func (tail *tailBuffer) String() string { return string(tail.data) }
