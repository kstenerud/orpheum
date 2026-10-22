// SPDX-License-Identifier: AGPL-3.0-or-later

package execx

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/diskwatch"
	"github.com/kstenerud/orpheum/internal/node/ffstreamer"
)

// Where a watched run's streamed film is: its channel's two pipes, after the progress pipe at 3, so
// the descriptors FFmpeg inherits have no gap (ffstreamer's PROTOCOL.md §4.2).
const (
	filmReadFD  = progressFD + 1
	filmWriteFD = progressFD + 2
)

// StreamedInput names a run's input as its film served through the node (WatchedRun.Film, D330),
// FFmpeg keeping cacheBlocks blocks of it (ffstreamer.CacheBlocks): only the ffstreamer protocol is
// allowed, so the run reads nothing else.
func StreamedInput(cacheBlocks int) []string {
	return ffstreamer.InputArguments(filmReadFD, filmWriteFD, cacheBlocks)
}

// Film is the film a run reads (D330): by its path, or, when Stream is set, served through the node,
// the run given no path to it.
type Film struct {
	// Path is the film's absolute path: what a run reading it by path is granted, and what names it
	// in the node's log.
	Path string
	// Size is its size when the node opened it, which bounds how long reading it may take.
	Size int64
	// Stream, when set, is the channel that serves it, and the run is granted no path to it.
	Stream *ffstreamer.Config
	// CacheBlocks is how many blocks of a streamed film FFmpeg keeps (ffstreamer.CacheBlocks).
	CacheBlocks int
}

// Input names the film as a run's input: by path (FileInput), or streamed (StreamedInput).
func (film Film) Input() []string {
	if film.Stream != nil {
		return StreamedInput(film.CacheBlocks)
	}
	return FileInput(film.Path)
}

// Readable is what a run reading the film is granted to read of it: its path, or nothing when it is
// streamed.
func (film Film) Readable() []string {
	if film.Stream != nil {
		return nil
	}
	return []string{film.Path}
}

// ReadingDeadline is the deadline of a run that reads the whole film (deadlineForReading), by the
// size it had when the node opened it, so bounding a run never stats a file whose disk may not answer.
func (film Film) ReadingDeadline(slowestReadBytesPerSecond int64) time.Duration {
	return deadlineForReading(film.Size, slowestReadBytesPerSecond)
}

// openFilm makes a run's film channel and hands FFmpeg its ends at filmReadFD and filmWriteFD:
// after a watched run's progress pipe, and after a closed descriptor 3 for a run with none.
func openFilm(command *exec.Cmd, film *ffstreamer.Config) (*ffstreamer.Channel, error) {
	if len(command.ExtraFiles) == 0 {
		command.ExtraFiles = []*os.File{nil}
	}
	if len(command.ExtraFiles) != filmReadFD-3 {
		return nil, errors.New("a streamed film's descriptors follow the progress pipe alone")
	}
	channel, err := ffstreamer.Open(*film)
	if err != nil {
		return nil, fmt.Errorf("open the film's channel: %w", err)
	}
	command.ExtraFiles = append(command.ExtraFiles, channel.ChildFiles()...)
	return channel, nil
}

// startCommand starts command, with its film's channel when the run reads one, and serves the film
// once it has started; on failure it abandons the progress pipe and the channel.
func (runner *Runner) startCommand(
	ctx context.Context, tool Tool, command *exec.Cmd, progress *progressPipe, film *ffstreamer.Config,
) (*ffstreamer.Channel, error) {
	var channel *ffstreamer.Channel
	if film != nil {
		var err error
		if channel, err = openFilm(command, film); err != nil {
			progress.abandon()
			return nil, &RunError{Tool: tool, ExitStatus: -1, Err: err}
		}
	}
	if err := command.Start(); err != nil {
		progress.abandon()
		if channel != nil {
			channel.Abandon()
		}
		return nil, &RunError{Tool: tool, ExitStatus: -1, Err: err}
	}
	if channel != nil {
		runner.tellPipeSizeRefused(ctx, channel.PipeSizeRefused())
		runner.startFilm(ctx, channel, command)
	}
	return channel, nil
}

// endFilm is how a run that has exited ended, its film's account taken in when it read one.
func endFilm(channel *ffstreamer.Channel, tool Tool, film *ffstreamer.Config, stderr *tailBuffer, ended error) error {
	if channel == nil {
		return ended
	}
	return judgeFilm(tool, channel.Finish(), film.Run, stderr, ended)
}

// finishFilmOnceExited ends the film of a run left running past its kill, once it does exit.
func finishFilmOnceExited(channel *ffstreamer.Channel, ending runEnding) {
	if channel != nil {
		go func() {
			<-ending.exited
			channel.Finish()
		}()
	}
}

// startFilm serves the film to the started run. A breach of the protocol, a read the film's
// storage fails, or a budget passed ends the run with SIGKILL, to its process by its pidfd: no
// graceful quit, since nothing it would write after one is kept (§9.8).
func (runner *Runner) startFilm(ctx context.Context, channel *ffstreamer.Channel, command *exec.Cmd) {
	channel.Start(ctx, func() {
		_ = runner.signalRun(command.Process, syscall.SIGKILL) //nolint:errcheck // a run already gone needs no kill
	})
}

// judgeFilm is a streamed run's end, given how it ended otherwise (§9.10): the node's own reason to
// kill it first, a film whose disk failed or did not answer being the disk's failure, as a run
// reading a file that stalls on its disk is, and any other the run's (a changed film among them);
// then FFmpeg's account of its input, which a run that exited 0 must close with DONE OK for its own
// run. A run that read part of its film never passes as complete.
func judgeFilm(tool Tool, account ffstreamer.Account, run int64, stderr *tailBuffer, ended error) error {
	var stuck *diskwatch.StalledError
	isDisks := errors.As(account.Failure, &stuck) || diskwatch.IsDiskFailure(account.Failure)
	switch {
	case errors.Is(account.Failure, ffstreamer.ErrReadFailed) && isDisks:
		return &StalledError{
			Tool: tool, Cause: StallDiskFailed, Condition: ConditionWaitingOnDisk, StderrTail: stderr.String(),
		}
	case account.Failure != nil:
		return &RunError{Tool: tool, ExitStatus: -1, StderrTail: stderr.String(), Err: account.Failure}
	case ended != nil:
		return ended
	case account.Done == nil:
		return &RunError{
			Tool: tool, StderrTail: stderr.String(),
			Err: errors.New("it exited 0 without accounting for its input (no DONE)"),
		}
	case !account.Done.IsOK:
		return &RunError{
			Tool: tool, StderrTail: stderr.String(),
			Err: fmt.Errorf("its input failed: %s, code %d: %s", account.Done.Kind, account.Done.Code,
				account.Done.Message),
		}
	case account.Done.Run != run:
		return &RunError{
			Tool: tool, StderrTail: stderr.String(),
			Err: fmt.Errorf("%w: its DONE names run %d, not %d", ffstreamer.ErrViolation, account.Done.Run, run),
		}
	}
	return nil
}

// protocolsOutputBytes bounds what ffmpeg -protocols may write: about a kilobyte in practice.
const protocolsOutputBytes = 64 << 10

// HasFFstreamer is whether the runner's FFmpeg can read a film through the node (D330): a build with
// the ffstreamer protocol among its input protocols, as orpheum-ffmpeg's patches add it.
func (runner *Runner) HasFFstreamer(ctx context.Context) (bool, error) {
	output, err := runner.Run(ctx, FFmpeg, confine.Access{}, []string{"-protocols"}, protocolsOutputBytes)
	if err != nil {
		return false, fmt.Errorf("list FFmpeg's protocols: %w", err)
	}
	return listsInputProtocol(string(output), "ffstreamer"), nil
}

// listsInputProtocol is whether ffmpeg -protocols' output lists name among its input protocols: the
// lines after "Input:" and before "Output:", one name each.
func listsInputProtocol(output, name string) bool {
	isInput := false
	for line := range strings.Lines(output) {
		switch word := strings.TrimSpace(line); {
		case word == "Input:":
			isInput = true
		case word == "Output:":
			isInput = false
		case isInput && word == name:
			return true
		}
	}
	return false
}
