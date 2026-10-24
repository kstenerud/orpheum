// SPDX-License-Identifier: AGPL-3.0-or-later

package fdproxy

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"time"

	"github.com/kstenerud/orpheum/internal/node/diskwatch"
)

// ErrFilmChanged is a film that changed while a run read it: its size or modification time is not
// what it was when the run began, so what the run made would be of two films (READ).
var ErrFilmChanged = errors.New("the film changed while it was read")

// FileSource serves a film the node has opened, through its disk's gate: each read waits on the
// film's place at most its window, as every reading of a root does (D233 §10), and a film that
// changes under the run fails it.
type FileSource struct {
	file   *os.File
	facts  fs.FileInfo
	gate   *diskwatch.Gate
	root   string
	path   string
	window time.Duration
}

// NewFileSource serves file, opened under root at path with facts, through gate, each read given
// window. The caller closes file once the run has ended.
func NewFileSource(
	file *os.File, facts fs.FileInfo, gate *diskwatch.Gate, root, path string, window time.Duration,
) *FileSource {
	return &FileSource{file: file, facts: facts, gate: gate, root: root, path: path, window: window}
}

// Size is the film's size when it was opened.
func (source *FileSource) Size() int64 { return source.facts.Size() }

// ReadAt reads buffer whole from offset through the gate, then checks the film is the one opened.
// A read that does not return within the window is a *diskwatch.StalledError, its place stuck until
// it returns.
func (source *FileSource) ReadAt(ctx context.Context, buffer []byte, offset int64) error {
	operation := diskwatch.Operation{Root: source.root, Path: source.path, Window: source.window}
	_, err := diskwatch.Run(ctx, source.gate, operation, func() (struct{}, error) {
		if _, err := source.file.ReadAt(buffer, offset); err != nil {
			return struct{}{}, fmt.Errorf("read %d bytes at %d of %s: %w", len(buffer), offset, source.path, err)
		}
		facts, err := source.file.Stat()
		if err != nil {
			return struct{}{}, fmt.Errorf("stat %s: %w", source.path, err)
		}
		if facts.Size() != source.facts.Size() || !facts.ModTime().Equal(source.facts.ModTime()) {
			return struct{}{}, fmt.Errorf("%w: %s", ErrFilmChanged, source.path)
		}
		return struct{}{}, nil
	}, func(struct{}) {})
	if err != nil {
		return fmt.Errorf("read the film: %w", err)
	}
	return nil
}
