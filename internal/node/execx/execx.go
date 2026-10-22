// SPDX-License-Identifier: AGPL-3.0-or-later

package execx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/ffstreamer"
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

// defaultKillGrace is how long a killed run's process may take to end before it is left running
// and listed as stuck (Runner.Stuck): only a process waiting on a device outlives SIGKILL, and it
// ends when the device answers, if ever (D233 §10).
const defaultKillGrace = 10 * time.Second

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
	// quitGrace and termGrace are how long a stalled run has after SIGINT and after SIGTERM, and
	// killGrace after SIGKILL, before it is left running.
	quitGrace, termGrace, killGrace time.Duration
	// sampleGroup reads /proc for a watched run's process group; a test's says what it is told.
	sampleGroup func(pgid int) groupSample
	// signalRun sends a signal to a run's process and its group; a test's may ignore SIGKILL, as the
	// kernel does for a process waiting on a device.
	signalRun func(process *os.Process, signal syscall.Signal) error
	// onStarted, when set by a test, is called once the process has started.
	onStarted func()
	// logger is told, once, that a streamed film's pipe could not take its size (LoggingTo); nil
	// tells no one.
	logger *slog.Logger
	// pipeSizeTold is that telling, once.
	pipeSizeTold sync.Once

	stuck stuckProcesses
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
		quitGrace: defaultQuitGrace, termGrace: defaultTermGrace, killGrace: defaultKillGrace,
		sampleGroup: sampleGroup, signalRun: signalRun,
		stuck: stuckProcesses{byPID: map[int]StuckProcess{}},
	}, nil
}

// LoggingTo has the runner log to logger what concerns the node rather than one run: once, that a
// streamed film's pipe kept its default size, the user's pipe memory spent (ffstreamer's
// PROTOCOL.md §4.3), so every streamed run reads slower.
func (runner *Runner) LoggingTo(logger *slog.Logger) *Runner {
	runner.logger = logger
	return runner
}

// tellPipeSizeRefused logs, once, that a streamed film's pipe kept its default size.
func (runner *Runner) tellPipeSizeRefused(ctx context.Context, refused error) {
	if refused == nil || runner.logger == nil {
		return
	}
	runner.pipeSizeTold.Do(func() {
		runner.logger.WarnContext(ctx, "a streamed film's pipe could not take its size, so streamed runs "+
			"read slower: raise fs.pipe-user-pages-soft (docs/operators)", "err", refused)
	})
}

// signalRun sends signal to the tool's process itself, through its pidfd where the kernel has one
// (os.Process), so a process that left its group is still reached; then to its process group, which
// also reaches anything an unconfined tool started. A confined tool can start nothing and leave no
// group (confine's seccomp filter; ffstreamer's PROTOCOL.md §9.1, §9.8). A process or group already
// gone needs no signal, so neither is an error.
func signalRun(process *os.Process, signal syscall.Signal) error {
	err := process.Signal(signal)
	if errors.Is(err, os.ErrProcessDone) {
		err = nil
	}
	if groupErr := syscall.Kill(-process.Pid, signal); groupErr != nil && !errors.Is(groupErr, syscall.ESRCH) {
		err = errors.Join(err, groupErr)
	}
	return err
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

// RunOnFilm is Run for a run whose input is film, which arguments name with film.Input: granted to
// read its path, or, when it is streamed, served it through the node, the verdict then also taking
// its channel's account (D330).
func (runner *Runner) RunOnFilm(
	ctx context.Context, tool Tool, film Film, arguments []string, stdoutLimitBytes int,
) ([]byte, error) {
	stdout := &limitedBuffer{limitBytes: stdoutLimitBytes}
	stderrTail, err := runner.runToExit(ctx, tool, confine.Access{Read: film.Readable()}, arguments,
		pipes{stdout: stdout, stderrTailLimitBytes: stderrTailBytes, film: film.Stream})
	if err != nil {
		return nil, err
	}
	return stdout.kept(tool, stderrTail)
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
	// truncated; zero discards stdout, unless StdoutTo takes it.
	StdoutLimitBytes int
	// StdoutTo, when set, is given stdout as FFmpeg writes it, and none of it is kept: an answer
	// too long to hold, read as it comes. A run sets it or StdoutLimitBytes, not both.
	StdoutTo io.Writer
	// LogLimitBytes is how much of the end of stderr is returned, for a tool whose answer is in its
	// log, as FFmpeg's ebur128 filter writes its summary when it ends; zero returns none.
	LogLimitBytes int
	Watch         Watch
	// Film, when set, is the run's input served through the node over ffstreamer (D330), which the
	// arguments name with StreamedInput. The run's verdict then also takes the channel's account.
	Film *ffstreamer.Config
}

// Ran is what a watched run left: its stdout and the end of its log, as its WatchedRun asked.
// Stdout given to a writer (WatchedRun.StdoutTo) is not in it.
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
		pausing: pausingOf(ctx), moved: movedOf(ctx), film: run.Film,
	}
	var stdout *limitedBuffer
	switch {
	case run.StdoutTo != nil && run.StdoutLimitBytes > 0:
		return Ran{}, errors.New("a watched run keeps its stdout or gives it to a writer, not both")
	case run.StdoutTo != nil:
		streams.stdout = run.StdoutTo
	case run.StdoutLimitBytes > 0:
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
	moved                func()
	film                 *ffstreamer.Config
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
	ending := runEnding{exited: make(chan struct{}), leftRunning: make(chan struct{})}
	var stdout *countingWriter
	if streams.watch != nil {
		stdout = &countingWriter{to: streams.stdout}
		streams.stdout = stdout
	}
	command, stderr := runner.command(ctx, runCtx, path, argv, streams, ending)
	var progress *progressPipe
	if streams.watch != nil {
		var err error
		if progress, err = openProgressPipe(command); err != nil {
			return nil, &RunError{Tool: tool, ExitStatus: -1, Err: err}
		}
	}

	film, err := runner.startCommand(ctx, tool, command, progress, streams.film)
	if err != nil {
		return nil, err
	}
	if progress != nil {
		dog := newWatchdog(runner.clock, *streams.watch)
		dog.moved = streams.moved
		if streams.pausing != nil {
			streams.pausing.attach(command.Process, dog, film)
			defer streams.pausing.detach()
		}
		progress.startReading(dog)
		go runner.watchUntil(ending.exited, watched{dog: dog, pid: command.Process.Pid, stdout: stdout}, tool, stopRun)
	}
	if runner.onStarted != nil {
		runner.onStarted()
	}
	waited := waitInBackground(command, ending, progress)
	select {
	case err := <-waited:
		ended := endFilm(film, tool, streams.film, stderr, outcome(ctx, runCtx, tool, command, stderr, err))
		if ended != nil {
			return nil, ended
		}
		return stderr, nil
	case <-ending.leftRunning:
		finishFilmOnceExited(film, ending)
		left := leftRun{tool: tool, reading: access.Read, pid: command.Process.Pid, stderr: stderr, waited: waited}
		return nil, runner.leaveRunning(ctx, runCtx, left)
	}
}

// waitInBackground waits for the started command's end apart from its caller, which may stop
// waiting for one that outlives its kill: the answer comes when it ends, once its progress is read.
func waitInBackground(command *exec.Cmd, ending runEnding, progress *progressPipe) <-chan error {
	waited := make(chan error, 1)
	go func() {
		err := command.Wait()
		close(ending.exited)
		progress.finishReading()
		waited <- err
	}()
	return waited
}

// runEnding is how a run's end is told: exited closes once its process has ended, and leftRunning
// once it has outlived its kill by killGrace.
type runEnding struct {
	exited      chan struct{}
	leftRunning chan struct{}
}

// leaveRunning answers for a run whose process outlived SIGKILL: it is listed as stuck until it
// ends, which waited says, and the run's caller is answered now, so its slot is freed (D233 §10).
// A stalled run says it was left running; a cancelled one says it was cancelled.
func (runner *Runner) leaveRunning(ctx, runCtx context.Context, left leftRun) error {
	runner.stuck.add(StuckProcess{PID: left.pid, Tool: left.tool, Reading: left.reading, Since: runner.clock.Now()})
	go func() {
		<-left.waited
		runner.stuck.remove(left.pid)
	}()
	stalled, isStalled := errors.AsType[*StalledError](context.Cause(runCtx))
	if ctx.Err() != nil || !isStalled {
		return &RunError{
			Tool: left.tool, ExitStatus: -1, StderrTail: left.stderr.String(),
			Err: fmt.Errorf("its process %d outlived its kill and is left running: %w", left.pid, ctx.Err()),
		}
	}
	// Only a process waiting on a device outlives SIGKILL, whatever the last sample said.
	stalled.Condition, stalled.IsLeftRunning, stalled.StderrTail = ConditionWaitingOnDisk, true, left.stderr.String()
	return stalled
}

// leftRun is a run whose process outlived its kill: its tool, what it was reading, its process,
// the end of its log, and its Wait's answer, which comes when it ends.
type leftRun struct {
	tool    Tool
	reading []string
	pid     int
	stderr  *tailBuffer
	waited  <-chan error
}

// command is the tool's command at path with argv, its streams as streams says, and the buffer
// that keeps the end of its stderr. Ending runCtx stops it: at once, killing its process group,
// when ctx has ended, and gracefully when the watchdog ended it (stopStalled).
func (runner *Runner) command(
	ctx, runCtx context.Context, path string, argv []string, streams pipes, ending runEnding,
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
		if ctx.Err() == nil && streams.film == nil {
			runner.stopStalled(command.Process, ending)
			return nil
		}
		// A cancel, or a stalled run that reads a streamed film, which keeps nothing it would write
		// after a graceful quit (ffstreamer's PROTOCOL.md §9.8): killed at once.
		err := runner.signalRun(command.Process, syscall.SIGKILL)
		go runner.leaveRunningUnlessExited(ending)
		return err
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
	case (waitErr != nil && isDiskFailure(stderr.String())) || stderr.sawDemuxingFail():
		// Not the file's: its disk did not answer, as a network share mounted to give up says (D233 §10).
		// FFmpeg whose demuxer's read fails so ends its input there and exits 0, its answer made of
		// part of the file (ffmpeg-http-input-2026-10-22): the log is what says it.
		return &StalledError{
			Tool: tool, Cause: StallDiskFailed, Condition: ConditionWaitingOnDisk, StderrTail: stderr.String(),
		}
	case waitErr != nil:
		status := command.ProcessState.ExitCode()
		return &RunError{Tool: tool, ExitStatus: status, StderrTail: stderr.String(), Err: waitErr}
	}
	return nil
}

// isDiskFailure is whether a tool's log says a read failed for its disk, not its bytes: one of the
// errors a network share that gives up on a host that is gone returns, as strerror words them.
func isDiskFailure(log string) bool {
	diskFailures := []string{"Input/output error", "Stale file handle", "Connection timed out", "Host is down"}
	return slices.ContainsFunc(diskFailures, func(words string) bool { return strings.Contains(log, words) })
}

// watched is a run the watchdog judges: its watchdog, its process group, and its stdout counted.
type watched struct {
	dog    *watchdog
	pid    int
	stdout *countingWriter
}

// watchUntil judges the run at each of the watchdog's ticks until it has exited, sampling its
// process group first, and stops it, with the stall as the cause, once the watchdog says it has
// stalled.
func (runner *Runner) watchUntil(exited <-chan struct{}, run watched, tool Tool, stopRun context.CancelCauseFunc) {
	ticks, stopTicks := runner.watchTicks()
	defer stopTicks()
	for {
		select {
		case <-exited:
			return
		case <-ticks:
			run.dog.observeGroup(runner.sampleGroup(run.pid), run.stdout.count())
			if stalled := run.dog.verdict(tool); stalled != nil {
				stopRun(stalled)
				return
			}
		}
	}
}

// stopStalled stops a stalled run: SIGINT, FFmpeg's graceful quit, then SIGTERM, then SIGKILL, each
// after its grace unless the run has exited by then; and leaves it running if it outlives the
// SIGKILL.
func (runner *Runner) stopStalled(process *os.Process, ending runEnding) {
	_ = runner.signalRun(process, syscall.SIGINT) //nolint:errcheck // a run already gone needs no stopping
	escalation := []struct {
		grace  time.Duration
		signal syscall.Signal
	}{{runner.quitGrace, syscall.SIGTERM}, {runner.termGrace, syscall.SIGKILL}}
	go func() {
		for _, step := range escalation {
			if !waitUnlessExited(ending.exited, step.grace) {
				return
			}
			_ = runner.signalRun(process, step.signal) //nolint:errcheck // a run already gone needs no stopping
		}
		runner.leaveRunningUnlessExited(ending)
	}()
}

// leaveRunningUnlessExited says the run is left running once it has outlived its kill by killGrace.
func (runner *Runner) leaveRunningUnlessExited(ending runEnding) {
	if waitUnlessExited(ending.exited, runner.killGrace) {
		close(ending.leftRunning)
	}
}

// waitUnlessExited waits for grace, and is whether it passed without the run exiting.
func waitUnlessExited(exited <-chan struct{}, grace time.Duration) bool {
	timer := time.NewTimer(grace)
	defer timer.Stop()
	select {
	case <-exited:
		return false
	case <-timer.C:
		return true
	}
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

// tailBuffer keeps the last limit bytes written to it. A run left running may still write to it
// while its caller reads it.
type tailBuffer struct {
	mutex      sync.Mutex
	data       []byte // guarded by mutex
	limitBytes int
	// demuxingFailed is whether the log said, anywhere, that the demuxer's read failed as a disk that
	// does not answer fails it: kept apart from the tail, which what follows may have pushed it out
	// of. Guarded by mutex.
	demuxingFailed bool
	// unended is the start of a line whose end has not been written yet; guarded by mutex.
	unended []byte
}

func (tail *tailBuffer) Write(data []byte) (int, error) {
	tail.mutex.Lock()
	defer tail.mutex.Unlock()
	tail.data = append(tail.data, data...)
	if excess := len(tail.data) - tail.limitBytes; excess > 0 {
		tail.data = tail.data[excess:]
	}
	tail.noteDemuxing(data)
	return len(data), nil
}

// noteDemuxing looks in each whole line written for FFmpeg's word that its demuxer's read failed
// for its disk ("Error during demuxing: Input/output error").
func (tail *tailBuffer) noteDemuxing(data []byte) {
	const longestLineKept = 4096
	lines := append(append([]byte(nil), tail.unended...), data...)
	end := bytes.LastIndexByte(lines, '\n')
	if end < 0 {
		tail.unended = lines[max(0, len(lines)-longestLineKept):]
		return
	}
	for line := range bytes.SplitSeq(lines[:end], []byte("\n")) {
		if _, failure, isDemuxing := bytes.Cut(line, []byte(demuxingFailedWords)); isDemuxing &&
			isDiskFailure(string(failure)) {
			tail.demuxingFailed = true
		}
	}
	tail.unended = append([]byte(nil), lines[end+1:]...)
}

// demuxingFailedWords begin FFmpeg's line for a read its demuxer could not make, followed by the
// error as strerror words it.
const demuxingFailedWords = "Error during demuxing: "

func (tail *tailBuffer) sawDemuxingFail() bool {
	tail.mutex.Lock()
	defer tail.mutex.Unlock()
	return tail.demuxingFailed
}

func (tail *tailBuffer) String() string {
	tail.mutex.Lock()
	defer tail.mutex.Unlock()
	return string(tail.data)
}

// countingWriter passes what is written on to another writer and counts it, for the watchdog,
// which reads the count from another goroutine.
type countingWriter struct {
	to      io.Writer
	written atomic.Int64
}

func (counting *countingWriter) Write(data []byte) (int, error) {
	written, err := counting.to.Write(data)
	counting.written.Add(int64(written))
	return written, err //nolint:wrapcheck // the writer's own error, passed through
}

// count is what has been written; zero for nil.
func (counting *countingWriter) count() int64 {
	if counting == nil {
		return 0
	}
	return counting.written.Load()
}
