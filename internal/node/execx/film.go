// SPDX-License-Identifier: AGPL-3.0-or-later

package execx

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
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
// the descriptors FFmpeg inherits have no gap (ffstreamer's specification, Descriptors).
const (
	filmReadFD  = progressFD + 1
	filmWriteFD = progressFD + 2
)

// StreamedInput names a run's input as its film served through the node (WatchedRun.Film, D330),
// FFmpeg keeping cacheBlocks blocks of it (filmCacheBlocks): only the ffstreamer protocol is
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
	// CacheBlocks is how many blocks of a streamed film FFmpeg keeps (ffstreamer's specification,
	// Reading in Blocks): filmCacheBlocks, for every film alike.
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

// The terms on which the node streams a film to the run that reads it (D330, ffstreamer's
// specification).

// filmBudgetFloorBytes is the least a run may be served of its film, so a small film, its index or a
// block's rounding never passes the budget (§9.6).
const filmBudgetFloorBytes = 64 << 20

// filmDataPipeBytes is the size asked for the pipe that carries a film's bytes: at the default 64 KiB,
// throughput halved in measurement (§4.3).
const filmDataPipeBytes = 1 << 20

// filmMostOutstanding is how many READs FFmpeg may have unanswered: the block it waits for, and the
// next, prefetched as it reads straight through.
const filmMostOutstanding = 2

// filmCacheBlocks is how many of a streamed film's blocks FFmpeg keeps (Reading in Blocks): about one
// for each place in the film a run reads from at once, and a few more. FFmpeg reads every track as it
// opens a film, and a prepare reads each track it maps, so a film whose tracks are stored one after
// another needs a block a track: a prepare of all 25 tracks of one read it 1.4 times at 32 blocks of
// 1 MiB and 1.3 at 48, and thrashed at 24 (ffmpeg-ffstreamer's bench/RESULTS.md, 2026-10-23). The
// library measured has at most 10 tracks in an MP4, the one container that stores them so.
const filmCacheBlocks = 48

// FilmReading is how much of its film a run reads, which sets how much it may be served (Budget).
type FilmReading int

const (
	// ReadsWholeFilm is a run that reads its film through: a read-through, a prepare, chunks, a
	// subtitle track.
	ReadsWholeFilm FilmReading = iota
	// ReadsIndex is a probe: the film's index and a few seconds of each track.
	ReadsIndex
)

// budget is the most a run reading a film of sizeBytes as reading may be served. A whole read: the
// film once, a quarter more for blocks read twice (an interleaved film measured 1.04 to 1.12 times its
// size), and the floor. A probe: a sixteenth of the film over the floor, which a probe passed only by
// thrashing (bench/interleave.py measured at most 70 MiB).
func (reading FilmReading) budget(sizeBytes int64) int64 {
	if reading == ReadsIndex {
		return sizeBytes/16 + filmBudgetFloorBytes
	}
	return sizeBytes + sizeBytes/4 + filmBudgetFloorBytes
}

// OpenedFilm is a film the node has opened and checked: the file, its facts as opened, and the gate of
// its disk, under Root, through which each of its reads goes, given Window.
type OpenedFilm struct {
	File   *os.File
	Facts  fs.FileInfo
	Disks  *diskwatch.Gate
	Root   string
	Window time.Duration
}

// StreamFilms has the runner stream every film to the run that reads it from now on (D330): its
// FFmpeg has the ffstreamer protocol (HasFFstreamer). It is called once, before any run.
func (runner *Runner) StreamFilms() { runner.streamsFilms.Store(true) }

// FilmOf is the film at path, an absolute path, opened and checked as opened, for a run that reads it
// as reading: served from the opened file through its disk's gate when the runner streams films, the
// run given no path to it; by its path otherwise. The caller closes the file once the run has ended.
func (runner *Runner) FilmOf(path string, opened OpenedFilm, reading FilmReading) Film {
	film := Film{Path: path, Size: opened.Facts.Size()}
	if !runner.streamsFilms.Load() {
		return film
	}
	film.Stream = &ffstreamer.Config{
		Run: runner.filmRuns.Add(1),
		Source: ffstreamer.NewFileSource(
			opened.File, opened.Facts, opened.Disks, opened.Root, path, opened.Window,
		),
		MostOutstanding: filmMostOutstanding,
		BudgetBytes:     reading.budget(film.Size),
		DataPipeBytes:   filmDataPipeBytes,
	}
	film.CacheBlocks = filmCacheBlocks
	return film
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
// the ffstreamer protocol among its input protocols, as ffmpeg-ffstreamer's patches add it.
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
