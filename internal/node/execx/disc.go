// SPDX-License-Identifier: AGPL-3.0-or-later

package execx

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"sync"
	"time"

	"github.com/kstenerud/orpheum/internal/node/disc"
	"github.com/kstenerud/orpheum/internal/node/fdproxy"
)

// ErrDiscNeedsStreaming is a disc image for a node whose FFmpeg reads its film by path: its main
// title is the image's runs joined, which only the node can serve (D350 §3).
var ErrDiscNeedsStreaming = errors.New(
	"a disc image is read only by an FFmpeg the node streams its film to (fdproxy), which this node's cannot")

// maxDiscTitles bounds the disc titles a runner remembers, each a few runs of an image.
const maxDiscTitles = 64

// discTitles remembers each disc image's main title by the image's path, size and modification
// time, so the runs of one film (its probe, read-through and every run of chunks) read its structure
// once.
type discTitles struct {
	mutex  sync.Mutex
	byFile map[discKey]disc.Title
}

// discKey is an image as it was opened.
type discKey struct {
	path       string
	sizeBytes  int64
	modifiedAt time.Time
}

// titleOf is the main title of the image at path, opened with facts and read through image:
// remembered, or read from its structure.
func (titles *discTitles) titleOf(
	ctx context.Context, path string, facts fs.FileInfo, image fdproxy.Source,
) (disc.Title, error) {
	key := discKey{path: path, sizeBytes: facts.Size(), modifiedAt: facts.ModTime()}
	titles.mutex.Lock()
	title, found := titles.byFile[key]
	titles.mutex.Unlock()
	if found {
		return title, nil
	}
	title, err := disc.MainTitle(sourceReader{ctx: ctx, source: image}, image.Size())
	if err != nil {
		return disc.Title{}, fmt.Errorf("read the disc image %s: %w", path, err)
	}
	titles.mutex.Lock()
	defer titles.mutex.Unlock()
	if titles.byFile == nil || len(titles.byFile) >= maxDiscTitles {
		titles.byFile = map[discKey]disc.Title{}
	}
	titles.byFile[key] = title
	return title, nil
}

// sourceReader reads a film's source as an io.ReaderAt, on ctx: each read through the disk's gate,
// as the run's own reads are.
type sourceReader struct {
	ctx    context.Context //nolint:containedctx // the reads of one film's structure, on its run's context
	source fdproxy.Source
}

// ReadAt reads buffer whole from offset, or fails.
func (reader sourceReader) ReadAt(buffer []byte, offset int64) (int, error) {
	if err := reader.source.ReadAt(reader.ctx, buffer, offset); err != nil {
		return 0, fmt.Errorf("read %d bytes at %d: %w", len(buffer), offset, err)
	}
	return len(buffer), nil
}

// titleSource serves a disc's main title from its image: each read of the title is the reads of the
// image's runs that hold it (D350 §3).
type titleSource struct {
	image fdproxy.Source
	title disc.Title
}

// Size is the title's length.
func (source titleSource) Size() int64 { return source.title.SizeBytes() }

// ReadAt reads buffer whole from offset of the title, from the image's runs that hold it.
func (source titleSource) ReadAt(ctx context.Context, buffer []byte, offset int64) error {
	pieces, err := source.title.Locate(offset, int64(len(buffer)))
	if err != nil {
		return fmt.Errorf("read the disc's title: %w", err)
	}
	for _, piece := range pieces {
		if err := source.image.ReadAt(ctx, buffer[:piece.Length], piece.Offset); err != nil {
			return fmt.Errorf("read the disc's title: %w", err)
		}
		buffer = buffer[piece.Length:]
	}
	return nil
}
