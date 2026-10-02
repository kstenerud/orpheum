// SPDX-License-Identifier: GPL-3.0-or-later

package execx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"syscall"
	"time"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/platform/clock"
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
	// clock is what the watchdog judges a stall by; a test's is a clocktest.Fake.
	clock clock.Clock
	// watchTicks are when the watchdog judges a watched run, and how to stop them; a test's are
	// its own.
	watchTicks func() (ticks <-chan time.Time, stop func())
	// quitGrace and termGrace are how long a stalled run has after SIGINT and after SIGTERM.
	quitGrace, termGrace time.Duration
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
	return &Runner{
		directory: directory, shim: shim, clock: clock.System{}, watchTicks: systemWatchTicks,
		quitGrace: defaultQuitGrace, termGrace: defaultTermGrace,
	}, nil
}

// systemWatchTicks ticks every watchCheckEvery.
func systemWatchTicks() (<-chan time.Time, func()) {
	ticker := time.NewTicker(watchCheckEvery)
	return ticker.C, ticker.Stop
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

// RunStreaming is Run for a run whose stdout is read as it is written: stdout goes to the
// writer, not into memory.
func (runner *Runner) RunStreaming(
	ctx context.Context, tool Tool, access confine.Access, arguments []string, stdout io.Writer,
) error {
	streams := pipes{stdout: stdout, stderrTailLimitBytes: stderrTailBytes}
	_, err := runner.runToExit(ctx, tool, access, arguments, streams)
	return err
}

// WatchedRun is one FFmpeg run the watchdog watches (Watch): its access and arguments after the
// hardening flags, what it is fed, and how much of its output and log is kept.
type WatchedRun struct {
	Access    confine.Access
	Arguments []string
	// Input is fed to FFmpeg's stdin; nil leaves it closed.
	Input []byte
	// StdoutLimitBytes is how much of stdout is kept, and a run that writes more is refused, never
	// truncated; zero discards stdout.
	StdoutLimitBytes int
	// LogLimitBytes is how much of the end of stderr is returned, for a tool whose answer is in its
	// log, as FFmpeg's ebur128 filter writes its summary when it ends; zero returns none.
	LogLimitBytes int
	Watch         Watch
}

// Ran is what a watched run left: its stdout and the end of its log, as its WatchedRun asked.
type Ran struct {
	Stdout []byte
	Log    string
}

// RunWatched runs FFmpeg as run says, with its -progress lines on a pipe of their own, and stops
// it when the watchdog sees its position stall: a graceful quit, then SIGTERM, then SIGKILL of its
// process group. A run it stopped returns a *StalledError. Cancelling ctx kills the tool and
// every process it started at once.
func (runner *Runner) RunWatched(ctx context.Context, run WatchedRun) (Ran, error) {
	streams := pipes{
		stdin: run.Input, stdout: io.Discard, stderrTailLimitBytes: stderrTailBytes, watch: &run.Watch,
		pausing: pausingOf(ctx),
	}
	var stdout *limitedBuffer
	if run.StdoutLimitBytes > 0 {
		stdout = &limitedBuffer{limitBytes: run.StdoutLimitBytes}
		streams.stdout = stdout
	}
	if run.LogLimitBytes > 0 {
		streams.stderrTailLimitBytes = run.LogLimitBytes
	}
	logged, err := runner.runToExit(ctx, FFmpeg, run.Access, run.Arguments, streams)
	if err != nil {
		return Ran{}, err
	}
	var ran Ran
	if run.LogLimitBytes > 0 {
		ran.Log = logged.String()
	}
	if stdout != nil {
		if ran.Stdout, err = stdout.kept(FFmpeg, logged); err != nil {
			return Ran{}, err
		}
	}
	return ran, nil
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
	return stdout.kept(tool, stderrTail)
}

// pipes are a run's streams: what its stdin is fed, nil to leave it closed; where its stdout goes;
// how much of the end of its stderr is kept; and, for a watched run, how it is watched.
type pipes struct {
	stdin                []byte
	stdout               io.Writer
	stderrTailLimitBytes int
	watch                *Watch
	pausing              *Pausing
}

// runToExit runs tool to its end, with its streams as streams says, and returns the end of its stderr.
func (runner *Runner) runToExit(
	ctx context.Context, tool Tool, access confine.Access, arguments []string, streams pipes,
) (*tailBuffer, error) {
	hardened, known := hardeningFlags(tool)
	if !known {
		return nil, fmt.Errorf("execx runs only ffmpeg and ffprobe, not %q", tool)
	}
	if streams.watch != nil {
		hardened = append(hardened, "-progress", "pipe:"+strconv.Itoa(progressFD))
	}
	path, argv := filepath.Join(runner.directory, string(tool)), slices.Concat(hardened, arguments)
	if runner.shim != "" {
		spec, err := confine.EncodeSpec(confine.Spec{Tool: path, Arguments: argv, Access: access})
		if err != nil {
			return nil, err //nolint:wrapcheck // confine says which path it refused
		}
		path, argv = runner.shim, []string{confine.ShimArgument, spec}
	}
	// The watchdog stops a run by ending runCtx with the stall as its cause; ctx ending is a cancel.
	runCtx, stopRun := context.WithCancelCause(ctx)
	defer stopRun(nil)
	exited := make(chan struct{})
	command, stderr := runner.command(ctx, runCtx, path, argv, streams, exited)
	var progress *progressPipe
	if streams.watch != nil {
		var err error
		if progress, err = openProgressPipe(command); err != nil {
			return nil, &RunError{Tool: tool, ExitStatus: -1, Err: err}
		}
	}

	if err := command.Start(); err != nil {
		progress.abandon()
		return nil, &RunError{Tool: tool, ExitStatus: -1, Err: err}
	}
	if progress != nil {
		dog := newWatchdog(runner.clock, *streams.watch)
		if streams.pausing != nil {
			streams.pausing.attach(command.Process.Pid, dog)
			defer streams.pausing.detach()
		}
		progress.startReading(dog)
		go runner.watchUntil(exited, dog, tool, stopRun)
	}
	if runner.onStarted != nil {
		runner.onStarted()
	}
	err := command.Wait()
	close(exited)
	progress.finishReading()
	if ended := outcome(ctx, runCtx, tool, command, stderr, err); ended != nil {
		return nil, ended
	}
	return stderr, nil
}

// command is the tool's command at path with argv, its streams as streams says, and the buffer
// that keeps the end of its stderr. Ending runCtx stops it: at once, killing its process group,
// when ctx has ended, and gracefully when the watchdog ended it (stopStalled).
func (runner *Runner) command(
	ctx, runCtx context.Context, path string, argv []string, streams pipes, exited <-chan struct{},
) (*exec.Cmd, *tailBuffer) {
	//nolint:gosec // the one sanctioned start: a configured directory, a fixed tool name, and argv as a slice
	command := exec.CommandContext(runCtx, path, argv...)
	command.Env = []string{}
	command.Dir = "/"
	stderr := &tailBuffer{limitBytes: streams.stderrTailLimitBytes}
	command.Stdout = streams.stdout
	command.Stderr = stderr
	if streams.stdin != nil {
		command.Stdin = bytes.NewReader(streams.stdin)
	}
	command.SysProcAttr = toolProcessAttributes()
	command.Cancel = func() error {
		if ctx.Err() == nil {
			runner.stopStalled(command.Process.Pid, exited)
			return nil
		}
		// The negative pid names the process group: the tool and anything it started.
		return syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
	}
	command.WaitDelay = waitDelay
	return command, stderr
}

// outcome is how a run that has exited ended, given Wait's error: nil, a cancel, a stall the
// watchdog stopped, or the tool failing.
func outcome(
	ctx, runCtx context.Context, tool Tool, command *exec.Cmd, stderr *tailBuffer, waitErr error,
) error {
	stalled, isStalled := errors.AsType[*StalledError](context.Cause(runCtx))
	switch {
	case ctx.Err() != nil:
		// Keep Wait's error too: exec.ErrWaitDelay there means something outlived the kill.
		return &RunError{Tool: tool, ExitStatus: -1, StderrTail: stderr.String(), Err: errors.Join(ctx.Err(), waitErr)}
	case isStalled:
		stalled.StderrTail = stderr.String()
		return stalled
	case waitErr != nil:
		status := command.ProcessState.ExitCode()
		return &RunError{Tool: tool, ExitStatus: status, StderrTail: stderr.String(), Err: waitErr}
	}
	return nil
}

// watchUntil judges the run at each of the watchdog's ticks until it has exited, and stops it,
// with the stall as the cause, once the watchdog says it has stalled.
func (runner *Runner) watchUntil(exited <-chan struct{}, dog *watchdog, tool Tool, stopRun context.CancelCauseFunc) {
	ticks, stopTicks := runner.watchTicks()
	defer stopTicks()
	for {
		select {
		case <-exited:
			return
		case <-ticks:
			if stalled := dog.verdict(tool); stalled != nil {
				stopRun(stalled)
				return
			}
		}
	}
}

// stopStalled stops a stalled run's process group: SIGINT, FFmpeg's graceful quit, then SIGTERM,
// then SIGKILL, each after its grace unless the run has exited by then.
func (runner *Runner) stopStalled(pid int, exited <-chan struct{}) {
	_ = syscall.Kill(-pid, syscall.SIGINT) //nolint:errcheck // a group already gone needs no stopping
	escalation := []struct {
		grace  time.Duration
		signal syscall.Signal
	}{{runner.quitGrace, syscall.SIGTERM}, {runner.termGrace, syscall.SIGKILL}}
	go func() {
		for _, step := range escalation {
			timer := time.NewTimer(step.grace)
			select {
			case <-exited:
				timer.Stop()
				return
			case <-timer.C:
			}
			_ = syscall.Kill(-pid, step.signal) //nolint:errcheck // a group already gone needs no stopping
		}
	}()
}

// progressPipe is a watched run's -progress pipe: FFmpeg writes into it as descriptor
// progressFD, and the watchdog reads it.
type progressPipe struct {
	reader, writer *os.File
	isRead         chan struct{}
}

// openProgressPipe opens the pipe and hands its writing end to command as descriptor progressFD.
func openProgressPipe(command *exec.Cmd) (*progressPipe, error) {
	reader, writer, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("open the progress pipe: %w", err)
	}
	command.ExtraFiles = []*os.File{writer} // the first extra file is descriptor 3, progressFD
	return &progressPipe{reader: reader, writer: writer, isRead: make(chan struct{})}, nil
}

// abandon closes the pipe of a run that never started; nil does nothing.
func (pipe *progressPipe) abandon() {
	if pipe == nil {
		return
	}
	_ = pipe.writer.Close() //nolint:errcheck // a pipe closes without error
	_ = pipe.reader.Close() //nolint:errcheck // a pipe closes without error
}

// startReading closes the node's copy of the writing end, which the tool now holds, and reads what
// the tool writes into dog until the tool's end closes.
func (pipe *progressPipe) startReading(dog *watchdog) {
	_ = pipe.writer.Close() //nolint:errcheck // a pipe closes without error
	go func() {
		defer close(pipe.isRead)
		dog.readProgress(pipe.reader)
	}()
}

// finishReading waits for the last of the progress once the run has exited, and no longer than
// waitDelay for a process that outlived it with the pipe open; nil does nothing.
func (pipe *progressPipe) finishReading() {
	if pipe == nil {
		return
	}
	timer := time.NewTimer(waitDelay)
	defer timer.Stop()
	select {
	case <-pipe.isRead:
	case <-timer.C:
	}
	_ = pipe.reader.Close() //nolint:errcheck // a pipe closes without error
	<-pipe.isRead
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

// kept is what the buffer kept, or the refusal of a run that wrote more than it allows.
func (limited *limitedBuffer) kept(tool Tool, stderrTail *tailBuffer) ([]byte, error) {
	if limited.isExceeded {
		return nil, &RunError{
			Tool: tool, ExitStatus: 0, StderrTail: stderrTail.String(),
			Err: fmt.Errorf("%w (%d bytes)", ErrOutputTooLarge, limited.limitBytes),
		}
	}
	return limited.buffer.Bytes(), nil
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
