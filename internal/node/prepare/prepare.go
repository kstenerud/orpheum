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

	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// Runner runs FFmpeg to its end, streaming its stdout: execx.Runner.
type Runner interface {
	RunStreaming(ctx context.Context, tool execx.Tool, arguments []string, stdout io.Writer) error
}

// renditionDir is the only shape a rendition's directory may have: "<file_id>/<profile>/<run_id>".
var renditionDir = regexp.MustCompile(`^[1-9]\d{0,18}/(remux|h264-8m)/[0-9a-f]{16}$`)

// ErrBadDir is a rendition directory that is not "<file_id>/<profile>/<run_id>".
var ErrBadDir = errors.New("a rendition's directory must be <file_id>/<profile>/<run_id>")

// Preparer makes renditions under the node's prepared directory.
type Preparer struct {
	runner    Runner
	directory string
}

// New returns a preparer writing under directory, which must be absolute.
func New(runner Runner, directory string) (*Preparer, error) {
	if !filepath.IsAbs(directory) {
		return nil, fmt.Errorf("the prepared directory must be an absolute path, not %q", directory)
	}
	return &Preparer{runner: runner, directory: directory}, nil
}

// Directory is where renditions are written, for the byte socket to serve them from.
func (preparer *Preparer) Directory() string { return preparer.directory }

// Prepare runs one prepare to its end: FFmpeg reads input, an absolute path, and writes the
// rendition into its directory, reporting how much of the source is done as it goes. It returns
// the rendition's size.
func (preparer *Preparer) Prepare(
	ctx context.Context, sourcePath string, command nodeproto.Prepare, progress func(preparedMs int64),
) (int64, error) {
	if !renditionDir.MatchString(command.RenditionDir) {
		return 0, fmt.Errorf("%w, not %q", ErrBadDir, command.RenditionDir)
	}
	renditionPath := filepath.Join(preparer.directory, filepath.FromSlash(command.RenditionDir))
	if err := os.MkdirAll(renditionPath, 0o750); err != nil {
		return 0, fmt.Errorf("make the rendition's directory: %w", err)
	}
	reader, writer := io.Pipe()
	done := make(chan struct{})
	go func() {
		defer close(done)
		readProgress(reader, progress)
	}()
	err := preparer.runner.RunStreaming(ctx, execx.FFmpeg, Arguments(sourcePath, renditionPath, command), writer)
	_ = writer.Close() //nolint:errcheck // a pipe's writer closes without error
	<-done
	if err != nil {
		return 0, fmt.Errorf("prepare %s: %w", command.RenditionDir, err)
	}
	return size(renditionPath)
}

// readProgress reads FFmpeg's -progress lines and reports out_time_us, in milliseconds, each time
// it moves.
func readProgress(reader io.Reader, progress func(preparedMs int64)) {
	lines := bufio.NewScanner(reader)
	for lines.Scan() {
		value, found := strings.CutPrefix(lines.Text(), "out_time_us=")
		if !found {
			continue
		}
		microseconds, err := strconv.ParseInt(value, 10, 64)
		if err == nil && microseconds >= 0 {
			progress(microseconds / 1000)
		}
	}
	// A reader that ends early leaves FFmpeg's pipe unread; drain it so the run is never blocked.
	_, _ = io.Copy(io.Discard, reader) //nolint:errcheck // draining
}

// size is the bytes of every file under directory.
func size(directory string) (int64, error) {
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
