// SPDX-License-Identifier: AGPL-3.0-or-later

package chunks

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/encode"
	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// InitName is a store's initialisation segment, which every chunk of it follows.
const InitName = "init.mp4"

// ChunkName is chunk n's file in its store.
func ChunkName(n int) string { return fmt.Sprintf("%05d.m4s", n) }

// TimelineOffsetMs is where the film's start is on every stream's timeline: every chunk's time is
// its time in the film plus this. A sound's first frame, the encoder's priming, comes before the
// film's start, and a picture's first decode before its keyframe is shown; neither may be before
// zero, and moving every stream alike keeps them in step.
const TimelineOffsetMs = 10_000

// aacPriming is how many samples FFmpeg's AAC encoder puts before the sound it was given: a sound
// run's first raw file starts that much before the run's start (slow-links.md §10).
const aacPriming = 1024

// How a run is watched: a grace before its first chunk, and a window it may go without one.
const (
	runStartGrace  = time.Minute
	runStallWindow = 2 * time.Minute
	// The speed a run is assumed to make chunks at: half of real time, before one has said.
	unmeasuredRealtimeFactor = 0.5
)

// storeDirPattern is a store's directory: <file_id>/chunks-<stream>/<grid_id>.
var storeDirPattern = regexp.MustCompile(`^\d+/chunks-[a-z0-9-]+/[0-9a-f]{16,64}$`)

// rawNamePattern is the shape of a raw file's name the run's FFmpeg is told to write: %05d.mp4.
var rawNamePattern = regexp.MustCompile(`^\d{5,9}\.mp4$`)

// ErrBadRun is a run whose store or chunks are not of a run's shape.
var ErrBadRun = errors.New("not a run of chunks")

// Keeper keeps the prepared directory, where the stores are: prepared.Shelf.
type Keeper interface {
	MakeRoom(ctx context.Context) error
	MarkWriting(dir string) (finishWriting func())
	// OpenStore makes the store, if it is not there, with its manifest, and a raw directory of its
	// own for this run's FFmpeg; it returns the raw directory's absolute path.
	OpenStore(manifest nodeproto.StoreManifest) (rawPath string, err error)
	// PutFile writes name into the store whole, so no reader sees part of it.
	PutFile(storeDir, name string, contents []byte) error
	// DiscardRaw removes one of a run's raw files once it is made into a chunk, or dropped.
	DiscardRaw(rawPath, name string) error
	// CloseRun removes the run's raw directory once it has ended.
	CloseRun(rawPath string) error
}

// Runner runs FFmpeg watched: execx.Runner.
type Runner interface {
	RunWatched(ctx context.Context, run execx.WatchedRun) (execx.Ran, error)
}

// Maker makes runs of chunks in the node's prepared directory.
type Maker struct {
	runner    Runner
	keeper    Keeper
	directory string
	encoder   encode.Encoder
}

// NewMaker returns a maker writing under directory, the prepared directory, encoding with encoder.
func NewMaker(runner Runner, keeper Keeper, directory string, encoder encode.Encoder) *Maker {
	return &Maker{runner: runner, keeper: keeper, directory: directory, encoder: encoder}
}

// check refuses a run that is not of a run's shape before anything is made.
func check(command nodeproto.MakeChunks) error {
	if !storeDirPattern.MatchString(command.StoreDir) {
		return fmt.Errorf("%w: the store %q", ErrBadRun, command.StoreDir)
	}
	if command.FirstChunk < 0 || len(command.BoundsMs) < 2 {
		return fmt.Errorf("%w: chunk %d with %d bounds", ErrBadRun, command.FirstChunk, len(command.BoundsMs))
	}
	for index := 1; index < len(command.BoundsMs); index++ {
		if command.BoundsMs[index] <= command.BoundsMs[index-1] {
			return fmt.Errorf("%w: bounds not rising at %d", ErrBadRun, index)
		}
	}
	isSound := command.Stream.Kind.IsSound()
	if isSound != (command.Stream.Sound != nil) {
		return fmt.Errorf("%w: a %s stream with a sound of %v", ErrBadRun,
			command.Stream.Kind, command.Stream.Sound)
	}
	if err := checkSeek(command); err != nil {
		return err
	}
	isLeadInWrong := !isSound || command.FirstChunk == 0 || command.SoundLeadInMs >= command.BoundsMs[0]
	if command.SoundLeadInMs != 0 && isLeadInWrong {
		return fmt.Errorf("%w: a lead-in at %d ms", ErrBadRun, command.SoundLeadInMs)
	}
	stream := command.Stream
	if stream.Kind == nodeproto.StreamQuality && (stream.BitRateKbps <= 0 || stream.Height <= 0) {
		return fmt.Errorf("%w: a quality of %d kb/s at %dp", ErrBadRun, stream.BitRateKbps, stream.Height)
	}
	return nil
}

// checkSeek refuses a copied picture whose seek is not past its first keyframe and before its
// second chunk, or, when it seeks before the keyframe (MPEG-TS), not at or before it; and any other
// stream with a picture's seek.
func checkSeek(command nodeproto.MakeChunks) error {
	isPicture := command.Stream.Kind == nodeproto.StreamPicture
	seekMs := command.PictureSeekMs
	isSeekRight := seekMs > command.BoundsMs[0] && seekMs < command.BoundsMs[1]
	if command.IsPictureSeekBeforeKeyframe {
		isSeekRight = seekMs >= 0 && seekMs <= command.BoundsMs[0]
	}
	if isPicture != isSeekRight || !isPicture && command.IsPictureSeekBeforeKeyframe {
		return fmt.Errorf("%w: a %s seeking to %d ms", ErrBadRun, command.Stream.Kind, seekMs)
	}
	return nil
}

// Make makes the run's chunks of source, a film the caller has checked, in order, telling onChunk
// each one as it is in its store, and returns the last it made, or -1 for none. A chunk is in the
// store whole and at its time in the film once told; a run that fails or is stopped keeps the chunks
// it made.
func (maker *Maker) Make(
	ctx context.Context, source execx.Film, command nodeproto.MakeChunks, onChunk func(n int),
) (int, error) {
	if err := check(command); err != nil {
		return -1, err
	}
	if err := maker.keeper.MakeRoom(ctx); err != nil {
		return -1, fmt.Errorf("make chunks of %s: %w", command.StoreDir, err)
	}
	finishWriting := maker.keeper.MarkWriting(command.StoreDir)
	defer finishWriting()
	rawPath, err := maker.keeper.OpenStore(nodeproto.StoreManifest{
		StoreDir: command.StoreDir, Video: command.Video, Stream: command.Stream,
	})
	if err != nil {
		return -1, err
	}
	defer func() { _ = maker.keeper.CloseRun(rawPath) }() //nolint:errcheck // eviction takes what is left
	taker := &rawTaker{command: command, keeper: maker.keeper, rawPath: rawPath, onChunk: onChunk, lastChunk: -1}
	storePath := filepath.Join(maker.directory, filepath.FromSlash(command.StoreDir))
	isOnGPU := command.Stream.Kind == nodeproto.StreamQuality && maker.encoder.UsesGPU()
	span := time.Duration(command.BoundsMs[len(command.BoundsMs)-1]-runStartMs(command)) * time.Millisecond
	watch := execx.Watch{
		StartGrace: runStartGrace, StallWindow: runStallWindow,
		Deadline: execx.DeadlineFor(time.Duration(float64(span) / unmeasuredRealtimeFactor)),
		// A failure taking a raw file is kept, and said when the run ends.
		OnProgress: func(execx.Progress) { _ = taker.takeFinished() }, //nolint:errcheck // see above
	}
	_, runErr := maker.runner.RunWatched(ctx, execx.WatchedRun{
		Access:    confine.Access{Read: source.Readable(), Write: []string{storePath}, UsesGPU: isOnGPU},
		Arguments: Arguments(source, rawPath, command, maker.encoder),
		Watch:     watch,
		Film:      source.Stream,
	})
	// What FFmpeg finished before it ended or was stopped is made into chunks either way.
	taker.end(runErr == nil)
	takeErr := taker.takeFinished()
	if runErr != nil {
		return taker.last(), fmt.Errorf("make chunks of %s: %w", command.StoreDir, runErr)
	}
	return taker.last(), takeErr
}

// rawTaker makes the raw files FFmpeg has finished into chunks, in order, as its list names them.
type rawTaker struct {
	command nodeproto.MakeChunks
	keeper  Keeper
	rawPath string
	onChunk func(n int)

	mutex sync.Mutex
	taken int // guarded by mutex: how many of the list's lines are taken
	// soundSamples is how many samples the run's sound has before the next raw file; guarded.
	soundSamples int64
	lastChunk    int // guarded by mutex
	// hasInit is whether the run has put the store's init; guarded by mutex.
	hasInit bool
	// hasEnded is whether FFmpeg has ended, and hasEndedCleanly whether it ended of itself, at the
	// run's end, and not stopped or failing; guarded by mutex.
	hasEnded, hasEndedCleanly bool
	failed                    error // guarded by mutex: the first failure, after which nothing more is taken
}

// end says FFmpeg has ended, cleanly or not: the run's last chunk is taken only then.
func (taker *rawTaker) end(isClean bool) {
	taker.mutex.Lock()
	defer taker.mutex.Unlock()
	taker.hasEnded, taker.hasEndedCleanly = true, isClean
}

func (taker *rawTaker) last() int {
	taker.mutex.Lock()
	defer taker.mutex.Unlock()
	return taker.lastChunk
}

// takeFinished takes every raw file the list names that is not taken yet. It is called from
// FFmpeg's progress, on another goroutine, and once more after FFmpeg has ended.
func (taker *rawTaker) takeFinished() error {
	taker.mutex.Lock()
	defer taker.mutex.Unlock()
	if taker.failed != nil {
		return taker.failed
	}
	listed, err := os.ReadFile(filepath.Join(taker.rawPath, rawListName))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		taker.failed = fmt.Errorf("read the run's list: %w", err)
		return taker.failed
	}
	lines := strings.Split(string(listed), "\n")
	// The last line is whole only once it ends with a newline.
	for ; taker.taken < len(lines)-1; taker.taken++ {
		err := taker.take(lines[taker.taken])
		if errors.Is(err, errNotYet) {
			return nil
		}
		if err != nil {
			taker.failed = err
			return err
		}
	}
	return nil
}

// errNotYet is the run's last chunk listed before FFmpeg has ended: whether a cut closed it or
// FFmpeg's end did is not known yet.
var errNotYet = errors.New("the run's last chunk waits for FFmpeg to end")

// take makes one listed raw file into its chunk, or drops it: a sound's lead-in, or what FFmpeg
// wrote past the run's last chunk. A file whose span is not its chunk's fails the run, and is never
// made a chunk: FFmpeg cut it elsewhere, or was stopped in it and listed it (idiosyncrasies.md).
func (taker *rawTaker) take(line string) error {
	entry, err := parseListed(line)
	if err != nil {
		return err
	}
	name, number := entry.name, entry.number
	command := taker.command
	lastOfRun := command.FirstChunk + len(command.BoundsMs) - 2
	if number == lastOfRun && !taker.hasEnded {
		return errNotYet
	}
	defer func() { _ = taker.keeper.DiscardRaw(taker.rawPath, name) }() //nolint:errcheck // eviction takes what is left
	if number > lastOfRun {
		return nil
	}
	if !rawNamePattern.MatchString(name) {
		return fmt.Errorf("the run's list names %q, not a raw file of its own", name)
	}
	if spanErr := checkSpan(command, entry, number == lastOfRun && taker.hasEndedCleanly); spanErr != nil {
		return spanErr
	}
	return taker.makeChunk(entry)
}

// makeChunk makes a listed raw file into its chunk, at its time in the film, putting the store's
// init first if the run has not; a sound's lead-in is counted and dropped.
func (taker *rawTaker) makeChunk(entry listed) error {
	name, number := entry.name, entry.number
	command := taker.command
	raw, err := os.ReadFile(filepath.Join(taker.rawPath, name)) //nolint:gosec // a raw file's name, checked above
	if err != nil {
		return fmt.Errorf("read raw %s: %w", name, err)
	}
	init, fragments, err := Split(raw)
	if err != nil {
		return fmt.Errorf("raw %s: %w", name, err)
	}
	scale, err := Timescale(init)
	if err != nil {
		return fmt.Errorf("raw %s: %w", name, err)
	}
	start, err := taker.startTicks(entry, init, fragments, int64(scale))
	if err != nil {
		return fmt.Errorf("raw %s: %w", name, err)
	}
	if number < command.FirstChunk {
		return nil // the sound's lead-in, counted and dropped
	}
	if !taker.hasInit {
		if putErr := taker.keeper.PutFile(command.StoreDir, InitName, init); putErr != nil {
			return putErr
		}
		taker.hasInit = true
	}
	retimed, err := Retime(fragments, uint64(start)) //nolint:gosec // a time in the film, never negative
	if err != nil {
		return fmt.Errorf("raw %s: %w", name, err)
	}
	if putErr := taker.keeper.PutFile(command.StoreDir, ChunkName(number), retimed); putErr != nil {
		return putErr
	}
	taker.lastChunk = number
	if taker.onChunk != nil {
		taker.onChunk(number)
	}
	return nil
}

// startTicks is where a listed raw file's first sample is decoded in the film, in its track's ticks.
// A picture's chunk starts with its keyframe, shown at its bound: it is decoded that much earlier
// as its composition offset says. A sound's starts after the run's samples before it, from the
// run's start less the encoder's priming; it counts this file's samples for the next. A sound
// copied ends where the muxer lists it ending, at the source's own time in the film, the next
// packet's or its last's end (copiedSoundTimes), and starts as long before that as its samples last:
// the list's start of a run's first file is not a packet's, and its first packet's time is moved.
func (taker *rawTaker) startTicks(entry listed, init, fragments []byte, scale int64) (int64, error) {
	command, number := taker.command, entry.number
	if command.Stream.Kind == nodeproto.StreamSoundCopied {
		lasting, err := Duration(init, fragments)
		if err != nil {
			return 0, err
		}
		endTicks := int64(math.Round((entry.endSeconds + TimelineOffsetMs/1000.0) * float64(scale)))
		return endTicks - lasting, nil
	}
	if command.Stream.Kind != nodeproto.StreamSound {
		shownAfter, err := FirstShownAfterDecode(fragments)
		if err != nil {
			return 0, err
		}
		return (command.BoundsMs[number-command.FirstChunk]+TimelineOffsetMs)*scale/1000 - shownAfter, nil
	}
	samples, err := Samples(fragments)
	if err != nil {
		return 0, err
	}
	start := (runStartMs(command)+TimelineOffsetMs)*scale/1000 - aacPriming + taker.soundSamples*aacFrameSamples
	taker.soundSamples += samples
	return start, nil
}

// aacFrameSamples is how many samples one AAC frame, one MP4 sample of it, holds.
const aacFrameSamples = 1024
