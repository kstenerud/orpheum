// SPDX-License-Identifier: AGPL-3.0-or-later

package execx

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"sync"

	"github.com/kstenerud/orpheum/internal/node/container"
	"github.com/kstenerud/orpheum/internal/node/fdproxy"
)

// Stretch is the stretch of its film a run of chunks reads, from FFmpeg's seek to where it stops
// reading, in the film's ms (OpenedFilm.Stretch, D512).
type Stretch struct{ FromMs, ToMs int64 }

// cutDownMinMoovBytes is the least movie box a film's index is cut down from (D512): FFmpeg parses
// the whole of it at every open, 4 to 5 s for 179 MB on production's Celeron, 0.05 s for a
// two-hour film's. The lead's default, awaiting the owner.
const cutDownMinMoovBytes = 16 << 20

// cutDownMarginMs is how much of the film either side of its run's stretch a cut-down index keeps,
// so the run seeks, and reads past its end, within it whatever its tracks' edits and offsets.
const cutDownMarginMs = 60_000

// maxCutDownIndexes bounds the films whose indexes a runner remembers laid out for cutting down.
const maxCutDownIndexes = 16

// errNoLongIndex is a film with no movie box long enough to cut down: shorter than the least, or
// no ISO BMFF file to find one in.
var errNoLongIndex = errors.New("no movie box long enough to cut down")

// isCutDownExtension says whether the path's extension is one of the ISO BMFF files whose indexes
// are cut down.
func isCutDownExtension(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mp4", ".m4v", ".mov":
		return true
	default:
		return false
	}
}

// cutDownIndexes remembers each long-indexed film's index, laid out for cutting down, or why it is
// not, by the film's path, size and modification time, so the runs of one film read its index once
// and a film refused is logged once.
type cutDownIndexes struct {
	mutex  sync.Mutex
	byFile map[discKey]*cutDownIndex
}

// cutDownIndex is one film's: its index, or why it is not cut down; nil both for a film whose index
// is too short to cut.
type cutDownIndex struct {
	mutex   sync.Mutex
	isRead  bool
	index   *container.Index
	refusal error
}

// cutDown is the film's source served with its index cut down to stretch, once FFmpeg would spend
// long parsing it (D512), and the source as it is otherwise. A film whose index this does not cut
// down is logged once, with why; a wrong cut never makes a chunk, so in any doubt the film is served
// as it is.
func (runner *Runner) cutDown(
	ctx context.Context, path string, facts fs.FileInfo, source fdproxy.Source, stretch Stretch,
) (fdproxy.Source, bool) {
	if !isCutDownExtension(path) {
		return source, false
	}
	entry := runner.cutDownIndexes.entryOf(discKey{path: path, sizeBytes: facts.Size(), modifiedAt: facts.ModTime()})
	entry.mutex.Lock()
	defer entry.mutex.Unlock()
	reader := sourceReader{ctx: ctx, source: source}
	if !entry.isRead {
		index, err := readCutDownIndex(reader, source.Size(), runner.cutDownFromBytes)
		if readErr, failed := errors.AsType[*container.ReadError](err); failed {
			// The run reads the film as it is, and finds the disk as it is.
			if runner.logger != nil {
				runner.logger.DebugContext(ctx, "a film's index could not be read to cut it down (D512)",
					"path", path, "why", readErr.Error())
			}
			return source, false
		}
		entry.isRead, entry.index = true, index
		if err != nil && !errors.Is(err, errNoLongIndex) {
			entry.refusal = err
			runner.tellNotCutDown(ctx, path, err)
		}
	}
	if entry.index == nil {
		return source, false
	}
	trimmed, err := entry.index.Trim(reader, stretch.FromMs-cutDownMarginMs, stretch.ToMs+cutDownMarginMs)
	if err != nil {
		if entry.refusal == nil {
			entry.refusal = err
			runner.tellNotCutDown(ctx, path, err)
		}
		return source, false
	}
	return cutDownSource{film: source, trimmed: trimmed}, true
}

// readCutDownIndex is the film's index laid out for cutting down: errNoLongIndex for a film whose
// movie box is shorter than fromBytes, or that is no ISO BMFF file or has none; a ReadError for a
// failed read; ErrCannotTrim, saying why, for an index it does not follow.
func readCutDownIndex(reader sourceReader, sizeBytes, fromBytes int64) (*container.Index, error) {
	moovAt, moov, err := container.FindMoov(reader, sizeBytes)
	switch {
	case errors.Is(err, container.ErrNotThisContainer), errors.Is(err, container.ErrNoIndex):
		return nil, fmt.Errorf("%w: %w", errNoLongIndex, err)
	case err != nil:
		return nil, fmt.Errorf("finding the movie box: %w", err)
	case moov.End-moovAt < fromBytes:
		return nil, fmt.Errorf("%w: a moov of %d bytes", errNoLongIndex, moov.End-moovAt)
	}
	index, err := container.ReadIndex(reader, sizeBytes, moovAt, moov)
	if err != nil {
		return nil, fmt.Errorf("a moov of %d bytes: %w", moov.End-moovAt, err)
	}
	return index, nil
}

// entryOf is the film's entry, made when it has none.
func (indexes *cutDownIndexes) entryOf(key discKey) *cutDownIndex {
	indexes.mutex.Lock()
	defer indexes.mutex.Unlock()
	if entry, found := indexes.byFile[key]; found {
		return entry
	}
	if indexes.byFile == nil || len(indexes.byFile) >= maxCutDownIndexes {
		indexes.byFile = map[discKey]*cutDownIndex{}
	}
	entry := &cutDownIndex{}
	indexes.byFile[key] = entry
	return entry
}

// tellNotCutDown logs that a long-indexed film is read with its index whole, and why.
func (runner *Runner) tellNotCutDown(ctx context.Context, path string, refusal error) {
	if runner.logger != nil {
		runner.logger.InfoContext(ctx, "a film's long index is read whole, so each run of it opens slowly (D512)",
			"path", path, "why", refusal.Error())
	}
}

// cutDownSource serves a film with its movie box cut down (container.Trimmed): every byte the film's
// but the movie box's, whose cut-down head is served from memory and its padding as zeros, read from
// no disk.
type cutDownSource struct {
	film    fdproxy.Source
	trimmed container.Trimmed
}

// Size is the film's: the cut-down index fills the movie box's place exactly.
func (source cutDownSource) Size() int64 { return source.film.Size() }

// ReadAt reads buffer whole from offset: the film's bytes either side of the movie box, read from
// it, and the cut-down index's.
func (source cutDownSource) ReadAt(ctx context.Context, buffer []byte, offset int64) error {
	before, after := source.trimmed.ReadAt(buffer, offset)
	if before > 0 {
		if err := source.film.ReadAt(ctx, buffer[:before], offset); err != nil {
			return fmt.Errorf("read the film: %w", err)
		}
	}
	if after > 0 {
		at := len(buffer) - after
		if err := source.film.ReadAt(ctx, buffer[at:], offset+int64(at)); err != nil {
			return fmt.Errorf("read the film: %w", err)
		}
	}
	return nil
}
