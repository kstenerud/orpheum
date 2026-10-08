// SPDX-License-Identifier: GPL-3.0-or-later

package prepare

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/diskspace"
)

// Runner runs a prepare's FFmpeg, watched for a stall: execx.Runner.
type Runner interface {
	RunWatched(ctx context.Context, run execx.WatchedRun) (execx.Ran, error)
}

// prepareStartGrace is how long a prepare's position may take to first move (execx.Watch):
// opening the source on a network mount, probing it and starting a hardware encoder, each of
// which can take tens of seconds.
const prepareStartGrace = 2 * time.Minute

// prepareStallWindow is how long a prepare's position may stand still. It moves with every packet
// written, several times a second, and stands still only while FFmpeg reads nothing. Three
// minutes outlasts an NFS client's retries (a 60-second timeout, retransmitted twice by default),
// and a stalled prepare is still answered before the core gives up on five minutes without a word
// (cmd/orpheum's prepareMaxSilence).
const prepareStallWindow = 3 * time.Minute

// unmeasuredRealtimeFactor is the speed a prepare's deadline assumes before this node has measured
// one of its profile: half of real time, a slow x264 encode of a 4K picture.
const unmeasuredRealtimeFactor = 0.5

// renditionDirPattern is the only shape a rendition's directory may have: "<file_id>/<profile>/<run_id>".
var renditionDirPattern = regexp.MustCompile(`^[1-9]\d{0,18}/(remux|h264)/[0-9a-f]{16}$`)

// ErrBadRenditionDir is a rendition directory that is not "<file_id>/<profile>/<run_id>".
var ErrBadRenditionDir = errors.New("a rendition's directory must be <file_id>/<profile>/<run_id>")

// Keeper keeps the prepared directory within its budget: prepared.Shelf (D217 §5).
type Keeper interface {
	// MakeRoom makes room for a prepare, or says why there is none.
	MakeRoom(ctx context.Context) error
	// MarkWriting marks a rendition being written, which eviction leaves alone, until finishWriting is called.
	MarkWriting(renditionDir string) (finishWriting func())
	// Shelve writes a made rendition's manifest beside it (D238 §2).
	Shelve(manifest nodeproto.RenditionManifest) error
}

// Preparer makes renditions under the node's prepared directory.
type Preparer struct {
	runner    Runner
	directory string
	keeper    Keeper
	encoder   Encoder

	mutex sync.Mutex
	// measuredRealtimeFactors are how fast the last prepare of each profile ran here: what the next
	// one's deadline expects. Guarded by mutex.
	measuredRealtimeFactors map[nodeproto.PrepareProfile]float64
}

// New returns a preparer writing under directory, which must be absolute, within the budget its
// keeper keeps, encoding with encoder (ChooseEncoder).
func New(runner Runner, directory string, keeper Keeper, encoder Encoder) (*Preparer, error) {
	if !filepath.IsAbs(directory) {
		return nil, fmt.Errorf("the prepared directory must be an absolute path, not %q", directory)
	}
	return &Preparer{
		runner: runner, directory: directory, keeper: keeper, encoder: encoder,
		measuredRealtimeFactors: map[nodeproto.PrepareProfile]float64{},
	}, nil
}

// Directory is where renditions are written, for the byte socket to serve them from.
func (preparer *Preparer) Directory() string { return preparer.directory }

// Made is what a prepare made, and how (D225 §3).
type Made struct {
	SizeBytes int64
	// VideoEncoder is the encoder the video was made with, or "copy" for a remux.
	VideoEncoder string
	// RealtimeFactor is how many seconds of the source FFmpeg prepared per second it ran, as it last said;
	// zero when it never said.
	RealtimeFactor float64
}

// Prepare runs one prepare to its end: FFmpeg reads sourcePath, an absolute path, and writes the
// rendition into its directory, reporting how much of the source is done as it goes. It returns
// what it made.
func (preparer *Preparer) Prepare(
	ctx context.Context, sourcePath string, command nodeproto.Prepare, onProgress func(preparedThroughMs int64),
) (Made, error) {
	if !renditionDirPattern.MatchString(command.RenditionDir) {
		return Made{}, fmt.Errorf("%w, not %q", ErrBadRenditionDir, command.RenditionDir)
	}
	// A prepare the disk has no room for is refused rather than fill it (D217 §5).
	if err := preparer.keeper.MakeRoom(ctx); err != nil {
		return Made{}, fmt.Errorf("prepare %s: %w", command.RenditionDir, err)
	}
	finishWriting := preparer.keeper.MarkWriting(command.RenditionDir)
	defer finishWriting()
	renditionPath := filepath.Join(preparer.directory, filepath.FromSlash(command.RenditionDir))
	if err := os.MkdirAll(renditionPath, 0o750); err != nil {
		return Made{}, fmt.Errorf("make the rendition's directory: %w", err)
	}
	// It reads the source and writes the rendition, and an encode on the GPU opens it (cut-a §7).
	isOnGPU := command.Profile == nodeproto.ProfileH264 && preparer.encoder.UsesGPU()
	access := confine.Access{Read: []string{sourcePath}, Write: []string{renditionPath}, UsesGPU: isOnGPU}
	var realtimeFactor float64
	watch := execx.Watch{
		StartGrace: prepareStartGrace, StallWindow: prepareStallWindow,
		Deadline: execx.DeadlineFor(preparer.expectedLength(command)),
		OnProgress: func(progress execx.Progress) {
			onProgress(progress.Position.Milliseconds())
			if progress.RealtimeFactor > 0 {
				realtimeFactor = progress.RealtimeFactor
			}
		},
	}
	arguments := Arguments(sourcePath, renditionPath, command, preparer.encoder)
	_, err := preparer.runner.RunWatched(ctx, execx.WatchedRun{Access: access, Arguments: arguments, Watch: watch})
	if err != nil {
		return Made{}, fmt.Errorf("prepare %s: %w", command.RenditionDir, err)
	}
	if realtimeFactor > 0 {
		preparer.mutex.Lock()
		preparer.measuredRealtimeFactors[command.Profile] = realtimeFactor
		preparer.mutex.Unlock()
	}
	sizeBytes, err := diskspace.TreeBytes(renditionPath)
	if err != nil {
		return Made{}, err
	}
	videoEncoder := videoCopied
	if command.Profile == nodeproto.ProfileH264 {
		videoEncoder = string(preparer.encoder)
	}
	made := Made{SizeBytes: sizeBytes, VideoEncoder: videoEncoder, RealtimeFactor: realtimeFactor}
	// The manifest is written while the rendition is still marked being written, so no eviction
	// finds it without one and takes it for a making that never finished.
	err = preparer.keeper.Shelve(nodeproto.RenditionManifest{
		Prepare: command, SizeBytes: made.SizeBytes, VideoEncoder: made.VideoEncoder,
		RealtimeFactor: made.RealtimeFactor,
	})
	if err != nil {
		return Made{}, fmt.Errorf("prepare %s: %w", command.RenditionDir, err)
	}
	return made, nil
}

// expectedLength is how long a prepare is expected to run: its source's length at the speed the
// last prepare of its profile ran here, or at unmeasuredRealtimeFactor before one has; zero when
// the source's length is unknown.
func (preparer *Preparer) expectedLength(command nodeproto.Prepare) time.Duration {
	preparer.mutex.Lock()
	realtimeFactor, isMeasured := preparer.measuredRealtimeFactors[command.Profile]
	preparer.mutex.Unlock()
	if !isMeasured {
		realtimeFactor = unmeasuredRealtimeFactor
	}
	return time.Duration(float64(command.SourceDurationMs)/realtimeFactor) * time.Millisecond
}
