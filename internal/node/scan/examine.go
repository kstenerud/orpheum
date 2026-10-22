// SPDX-License-Identifier: AGPL-3.0-or-later

package scan

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/node/hash"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/bounded"
	"github.com/kstenerud/orpheum/internal/platform/readonly"
)

// maxNFOBytes bounds the NFO a sidecar record carries. Real ones are a few kilobytes; a larger
// one is refused with an error record rather than sent.
const maxNFOBytes = 1 << 20

// errNFOTooLarge is an NFO past maxNFOBytes.
var errNFOTooLarge = errors.New("an NFO file larger than the node sends")

// errNotAFile is a path the core asked to examine that is not a regular file now.
var errNotAFile = errors.New("not a regular file")

// errUnknownKind is an examining of a kind this node does not know.
var errUnknownKind = errors.New("an unknown kind of examining")

// errOutsideRoots is a path whose links lead out of every media root (D206).
var errOutsideRoots = errors.New("its links lead outside every media root")

// Examine reads each of files under root as its kind says (D240 §4) and returns, for each in turn,
// its observed or sidecar record, or the error that kept it from being read. Before each file it
// calls between, which may wait — for play-driven work to have the device (D240 §6) — and whose
// error, or ctx's end, stops it with the records so far.
func (scanner *Scanner) Examine(
	ctx context.Context, root *confine.MediaRoot, files []nodeproto.ExamineFile, between func() error,
) ([]nodeproto.ScanRecord, error) {
	var records []nodeproto.ScanRecord
	emit := func(record nodeproto.ScanRecord) error {
		records = append(records, record)
		return nil
	}
	walk := &walk{scanner: scanner, ctx: ctx, emit: emit, errors: newErrorLog(emit, 0)}
	for _, file := range files {
		if err := between(); err != nil {
			return records, err
		}
		if ctx.Err() != nil {
			return records, fmt.Errorf("examine under %s: %w", root.Path(), ctx.Err())
		}
		if err := walk.examine(root, file); err != nil {
			return records, err
		}
		// Each file's error is its own: one is never folded into the next's.
		if err := walk.errors.flush(); err != nil {
			return records, err
		}
	}
	return records, nil
}

// examine reads one file, emitting its record or its error. A file reached through a link is read
// where the link leads, within the media roots, as a listing finds it (D206).
func (walk *walk) examine(root *confine.MediaRoot, file nodeproto.ExamineFile) error {
	reported := string(file.Path.Bytes())
	start := location{mediaRoot: root, relativePath: reported, reportedPath: reported}
	place, isInside, err := walk.scanner.resolve(start)
	if err == nil && !isInside {
		err = errOutsideRoots
	}
	if err != nil {
		return walk.fail(reported, nodeproto.ScopeEntry, err, nil)
	}
	info, err := statFile(place)
	if err != nil {
		return walk.fail(reported, nodeproto.ScopeEntry, err, info)
	}
	switch file.Kind {
	case nodeproto.ExamineVideo:
		return walk.observe(place, info)
	case nodeproto.ExamineNFO:
		return walk.reportSidecar(place, info, nodeproto.SidecarNFO)
	case nodeproto.ExamineSubtitle:
		return walk.reportSidecar(place, info, nodeproto.SidecarSubtitle)
	}
	return walk.fail(reported, nodeproto.ScopeEntry, fmt.Errorf("%w: %q", errUnknownKind, file.Kind), info)
}

// statFile describes the file at place as opening it finds it, and refuses what is not a regular
// file.
func statFile(place location) (fs.FileInfo, error) {
	opened, err := place.mediaRoot.Open(place.relativePath)
	if err != nil {
		return nil, fmt.Errorf("examine: %w", err)
	}
	info, statErr := opened.Stat()
	closeErr := opened.Close()
	if statErr != nil {
		return nil, fmt.Errorf("stat %s: %w", place.relativePath, statErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close %s: %w", place.relativePath, closeErr)
	}
	if !info.Mode().IsRegular() {
		return info, fmt.Errorf("%s: %w", place.relativePath, errNotAFile)
	}
	return info, nil
}

// observe probes and hashes a video.
func (walk *walk) observe(place location, info fs.FileInfo) error {
	identity, err := walk.payloadIdentity(place, info.Size())
	if err != nil {
		return walk.fail(place.reportedPath, nodeproto.ScopeEntry, err, info)
	}
	moviehash, err := walk.openSubtitlesHash(place, info.Size())
	if err != nil {
		return walk.fail(place.reportedPath, nodeproto.ScopeEntry, err, info)
	}
	probed, err := walk.probe(place)
	if err != nil {
		return walk.fail(place.reportedPath, nodeproto.ScopeEntry, err, info)
	}
	device, _ := confine.DeviceOf(info)
	inode, _ := confine.InodeOf(info)
	return walk.send(nodeproto.ScanRecord{Type: nodeproto.RecordObserved, Observed: &nodeproto.Observed{
		Path:       toNodePath(place.reportedPath),
		SizeBytes:  info.Size(),
		ModifiedAt: info.ModTime().UnixMilli(),
		Identity:   identity,
		Probe:      probed,

		OpensubtitlesHash: moviehash,
		FilesystemDevice:  device,
		Inode:             inode,
	}})
}

// probe probes the video at place: streamed from the file the scanner opens, when its options make
// films (Options.Films), or by its path. Its failure is errProbe's.
func (walk *walk) probe(place location) (nodeproto.Probe, error) {
	film := execx.Film{Path: place.absolute()}
	if options := walk.scanner.options; options.Films != nil {
		file, err := place.mediaRoot.Open(place.relativePath)
		if err != nil {
			return nodeproto.Probe{}, fmt.Errorf("%w: open %s: %w", errProbe, place.relativePath, err)
		}
		defer readonly.Close(file)
		facts, err := file.Stat()
		if err != nil {
			return nodeproto.Probe{}, fmt.Errorf("%w: stat %s: %w", errProbe, place.relativePath, err)
		}
		opened := execx.OpenedFilm{File: file, Facts: facts, Disks: options.Disks, Root: place.mediaRoot.Path()}
		opened.Window = options.FilmReadWindow
		film = options.Films.FilmOf(film.Path, opened, execx.ReadsIndex)
	}
	probed, err := walk.scanner.prober.Probe(walk.ctx, film)
	if err != nil {
		return nodeproto.Probe{}, fmt.Errorf("%w: %w", errProbe, err)
	}
	return probed, nil
}

// openSubtitlesHash is OpenSubtitles' moviehash of a video, from its first and last 64 KiB
// (D224 §3).
func (walk *walk) openSubtitlesHash(place location, size int64) (string, error) {
	file, err := place.mediaRoot.Open(place.relativePath)
	if err != nil {
		return "", fmt.Errorf("open for the OpenSubtitles hash: %w", err)
	}
	hashed, hashErr := hash.OpenSubtitlesHash(movingReader{file, walk.ctx}, size)
	closeErr := file.Close()
	if hashErr != nil {
		return "", fmt.Errorf("hash for OpenSubtitles: %w", hashErr)
	}
	if closeErr != nil {
		return "", fmt.Errorf("close after the OpenSubtitles hash: %w", closeErr)
	}
	return hashed, nil
}

func (walk *walk) payloadIdentity(place location, size int64) (nodeproto.Identity, error) {
	file, err := place.mediaRoot.Open(place.relativePath)
	if err != nil {
		return nodeproto.Identity{}, fmt.Errorf("open for the payload hash: %w", err)
	}
	identity, hashErr := hash.PayloadIdentity(movingReader{file, walk.ctx}, size)
	closeErr := file.Close()
	if hashErr != nil {
		return nodeproto.Identity{}, fmt.Errorf("payload hash: %w", hashErr)
	}
	if closeErr != nil {
		return nodeproto.Identity{}, fmt.Errorf("close after the payload hash: %w", closeErr)
	}
	return nodeproto.Identity{
		Scheme:       hash.Scheme,
		Anchor:       string(identity.Anchor),
		PayloadHash:  hex.EncodeToString(identity.PayloadHash[:]),
		PayloadBytes: identity.PayloadBytes,
	}, nil
}

// movingReader reads a file for a hash, and each read is movement (execx.Moved): a hash reading
// through a large file moves while no record is sent.
type movingReader struct {
	file io.ReaderAt
	ctx  context.Context //nolint:containedctx // one hash's reads, on its examining's context
}

// ReadAt reads the file, and is movement when it read anything.
func (reader movingReader) ReadAt(buffer []byte, offset int64) (int, error) {
	read, err := reader.file.ReadAt(buffer, offset)
	if read > 0 {
		execx.Moved(reader.ctx)
	}
	return read, err //nolint:wrapcheck // the file's own error, which the hash wraps
}

// reportSidecar reports a metadata or subtitle file, with an NFO's bytes.
func (walk *walk) reportSidecar(place location, info fs.FileInfo, kind nodeproto.SidecarKind) error {
	record := &nodeproto.Sidecar{
		Path: toNodePath(place.reportedPath), Kind: kind, SizeBytes: info.Size(),
		ModifiedAt: info.ModTime().UnixMilli(),
	}
	switch kind {
	case nodeproto.SidecarNFO:
		body, err := readNFO(place, info.Size())
		if err != nil {
			return walk.fail(place.reportedPath, nodeproto.ScopeEntry, err, info)
		}
		record.NFOBody = body
	case nodeproto.SidecarSubtitle:
		identity, err := walk.payloadIdentity(place, info.Size())
		if err != nil {
			return walk.fail(place.reportedPath, nodeproto.ScopeEntry, err, info)
		}
		record.Identity = &identity
	case nodeproto.SidecarImage:
	}
	return walk.send(nodeproto.ScanRecord{Type: nodeproto.RecordSidecar, Sidecar: record})
}

func readNFO(place location, size int64) ([]byte, error) {
	if size > maxNFOBytes {
		return nil, fmt.Errorf("%w: %d bytes, at most %d", errNFOTooLarge, size, maxNFOBytes)
	}
	body, err := place.mediaRoot.ReadFile(place.relativePath, maxNFOBytes)
	if errors.Is(err, bounded.ErrTooLarge) {
		return nil, fmt.Errorf("%w: it grew past %d bytes while read", errNFOTooLarge, maxNFOBytes)
	}
	if err != nil {
		return nil, fmt.Errorf("read the NFO: %w", err)
	}
	return body, nil
}
