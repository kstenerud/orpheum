// SPDX-License-Identifier: GPL-3.0-or-later

package scan

import (
	"context"
	"errors"
	"fmt"
	"io/fs"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// errNotAFile is a path the core asked to examine that is not a regular file now.
var errNotAFile = errors.New("not a regular file")

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

// examine reads one file, emitting its record or its error.
func (walk *walk) examine(root *confine.MediaRoot, file nodeproto.ExamineFile) error {
	reported := string(file.Path.Bytes())
	place := location{mediaRoot: root, relativePath: reported, reportedPath: reported}
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

// errUnknownKind is an examining of a kind this node does not know.
var errUnknownKind = errors.New("an unknown kind of examining")

// statFile describes the file at place as opening it finds it, links inside the root followed, and
// refuses what is not a regular file.
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
