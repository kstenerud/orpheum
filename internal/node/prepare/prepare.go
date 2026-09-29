// SPDX-License-Identifier: GPL-3.0-or-later

package prepare

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// Runner runs FFmpeg to its end, streaming its stdout: execx.Runner.
type Runner interface {
	RunStreaming(
		ctx context.Context, tool execx.Tool, access confine.Access, arguments []string, stdout io.Writer,
	) error
}

// renditionDirPattern is the only shape a rendition's directory may have: "<file_id>/<profile>/<run_id>".
var renditionDirPattern = regexp.MustCompile(`^[1-9]\d{0,18}/(remux|h264-8m)/[0-9a-f]{16}$`)

// ErrBadRenditionDir is a rendition directory that is not "<file_id>/<profile>/<run_id>".
var ErrBadRenditionDir = errors.New("a rendition's directory must be <file_id>/<profile>/<run_id>")

// Keeper keeps the prepared directory within its budget: prepared.Shelf (D217 §5).
type Keeper interface {
	// MakeRoom makes room for a prepare, or says why there is none.
	MakeRoom() error
	// MarkWriting marks a rendition being written, which eviction leaves alone, until finishWriting is called.
	MarkWriting(renditionDir string) (finishWriting func())
}

// Preparer makes renditions under the node's prepared directory.
type Preparer struct {
	runner    Runner
	directory string
	keeper    Keeper
	encoder   Encoder
}

// New returns a preparer writing under directory, which must be absolute, within the budget its
// keeper keeps, encoding with encoder (ChooseEncoder).
func New(runner Runner, directory string, keeper Keeper, encoder Encoder) (*Preparer, error) {
	if !filepath.IsAbs(directory) {
		return nil, fmt.Errorf("the prepared directory must be an absolute path, not %q", directory)
	}
	return &Preparer{runner: runner, directory: directory, keeper: keeper, encoder: encoder}, nil
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
	ctx context.Context, sourcePath string, command nodeproto.Prepare, onProgress func(preparedMs int64),
) (Made, error) {
	if !renditionDirPattern.MatchString(command.RenditionDir) {
		return Made{}, fmt.Errorf("%w, not %q", ErrBadRenditionDir, command.RenditionDir)
	}
	// A prepare the disk has no room for is refused rather than fill it (D217 §5).
	if err := preparer.keeper.MakeRoom(); err != nil {
		return Made{}, fmt.Errorf("prepare %s: %w", command.RenditionDir, err)
	}
	finishWriting := preparer.keeper.MarkWriting(command.RenditionDir)
	defer finishWriting()
	renditionPath := filepath.Join(preparer.directory, filepath.FromSlash(command.RenditionDir))
	if err := os.MkdirAll(renditionPath, 0o750); err != nil {
		return Made{}, fmt.Errorf("make the rendition's directory: %w", err)
	}
	reader, writer := io.Pipe()
	progressDrained := make(chan struct{})
	var realtimeFactor float64
	go func() {
		defer close(progressDrained)
		realtimeFactor = readProgress(reader, onProgress)
	}()
	// It reads the source and writes the rendition, and an encode on the GPU opens it (cut-a §7).
	isOnGPU := command.Profile == nodeproto.ProfileH264At8M && preparer.encoder.UsesGPU()
	access := confine.Access{Read: []string{sourcePath}, Write: []string{renditionPath}, UsesGPU: isOnGPU}
	arguments := Arguments(sourcePath, renditionPath, command, preparer.encoder)
	err := preparer.runner.RunStreaming(ctx, execx.FFmpeg, access, arguments, writer)
	_ = writer.Close() //nolint:errcheck // a pipe's writer closes without error
	<-progressDrained
	if err != nil {
		return Made{}, fmt.Errorf("prepare %s: %w", command.RenditionDir, err)
	}
	sizeBytes, err := directoryBytes(renditionPath)
	if err != nil {
		return Made{}, err
	}
	videoEncoder := videoCopied
	if command.Profile == nodeproto.ProfileH264At8M {
		videoEncoder = string(preparer.encoder)
	}
	return Made{SizeBytes: sizeBytes, VideoEncoder: videoEncoder, RealtimeFactor: realtimeFactor}, nil
}

// readProgress reads FFmpeg's -progress lines and reports out_time_us, in milliseconds, each time
// it moves. It returns the last speed FFmpeg gave ("speed=6.2x"), or zero when it gave none.
func readProgress(reader io.Reader, onProgress func(preparedMs int64)) (realtimeFactor float64) {
	lines := bufio.NewScanner(reader)
	for lines.Scan() {
		key, value, _ := strings.Cut(lines.Text(), "=")
		switch key {
		case "out_time_us":
			microseconds, err := strconv.ParseInt(value, 10, 64)
			if err == nil && microseconds >= 0 {
				onProgress(microseconds / 1000)
			}
		case "speed":
			// "N/A" until FFmpeg has timed something; a speed it cannot say leaves the last one.
			if said, err := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(value), "x"), 64); err == nil {
				realtimeFactor = said
			}
		}
	}
	// A reader that ends early leaves FFmpeg's pipe unread; drain it so the run is never blocked.
	_, _ = io.Copy(io.Discard, reader) //nolint:errcheck // draining
	return realtimeFactor
}

// directoryBytes is the bytes of every file under directory.
func directoryBytes(directory string) (int64, error) {
	var total int64
	err := filepath.WalkDir(directory, func(_ string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		total += info.Size()
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("measure the rendition: %w", err)
	}
	return total, nil
}
