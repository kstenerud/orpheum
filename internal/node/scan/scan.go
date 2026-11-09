// SPDX-License-Identifier: AGPL-3.0-or-later

package scan

import (
	"context"
	"fmt"
	"io/fs"
	"path"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/diskwatch"
	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// Prober looks inside one file; probe.Prober is the real one.
type Prober interface {
	Probe(ctx context.Context, film execx.Film) (nodeproto.Probe, error)
}

// Films makes the film a probe reads from the file the scanner opened: execx.Runner's FilmOf, which
// streams it when the node's FFmpeg can be streamed one (D330).
type Films interface {
	FilmOf(
		ctx context.Context, absolutePath string, opened execx.OpenedFilm, reading execx.FilmReading,
	) (execx.Film, error)
}

// emitFunc takes one record of a listing or an examining.
type emitFunc func(record nodeproto.ScanRecord) error

// Options are the scanner's settings, from the node's config file.
type Options struct {
	// UID and GID are the node's effective ids, reported so an error can be read against them.
	UID, GID int
	// Films makes the film each probe reads from the file the scanner opens; nil probes each file by
	// its path. Disks is the gate of its disks, through which a streamed film's reads go, each given
	// FilmReadWindow.
	Films          Films
	Disks          *diskwatch.Gate
	FilmReadWindow time.Duration
}

// Scanner lists under media roots and examines the files the core names (D240). It reports; the
// core decides.
type Scanner struct {
	roots   []*confine.MediaRoot
	prober  Prober
	options Options
}

// NewScanner returns a scanner over roots, every configured media root: a symlink is followed
// only into one of them (D206).
func NewScanner(roots []*confine.MediaRoot, prober Prober, options Options) *Scanner {
	return &Scanner{roots: roots, prober: prober, options: options}
}

// FolderNotOpened is the listing of a library folder inside a root that could not be opened, for
// cause: gone, or not readable. It begins and completes as the folder's listing, with an error of
// the root's scope between, as a root whose facts cannot be read is listed: that withholds every
// removal, and the core marks the folder unavailable until a listing reads it (D40, D241 §2). A
// vanished folder's listing was refused instead, and the folder left as complete (staging,
// 2026-10-27, VQ81). Its uid and gid are the node's effective ids, as Options holds them.
func FolderNotOpened(rootPath string, uid, gid int, cause error) []nodeproto.ScanRecord {
	return []nodeproto.ScanRecord{
		{Type: nodeproto.RecordScanBegin, ScanBegin: &nodeproto.ScanBegin{RootPath: rootPath, UID: uid, GID: gid}},
		{Type: nodeproto.RecordError, Error: &nodeproto.ScanError{
			Path: toNodePath("."), Scope: nodeproto.ScopeRoot, Code: classify(cause), OccurrenceCount: 1,
			PlatformError: platformError(cause, nil),
		}},
		{Type: nodeproto.RecordScanComplete, ScanComplete: &nodeproto.ScanComplete{RootPath: rootPath, ErrorCount: 1}},
	}
}

// location is where an entry is read from — a media root and a path inside it — and the path
// it is reported under, relative to the root being listed. They differ below a followed link.
type location struct {
	mediaRoot    *confine.MediaRoot
	relativePath string
	reportedPath string
	device       uint64 // the directory's device, for spotting a mount boundary below it
	level        int    // how far below where the listing started: its entries are 1
}

func (place location) child(name string) location {
	return location{
		mediaRoot: place.mediaRoot, relativePath: path.Join(place.relativePath, name),
		reportedPath: path.Join(place.reportedPath, name),
		device:       place.device, level: place.level + 1,
	}
}

func (place location) absolute() string { return path.Join(place.mediaRoot.Path(), place.relativePath) }

// List lists what is under the command's path, relative to root, to its depth below it, 0 for the
// whole tree, stopping at its error limit, and returns its records (nodeproto.Listing says which): it
// reads no file (D240 §2). A cancelled listing ends with a run-scoped error and no scanComplete, so
// the core removes nothing for it (D39).
func (scanner *Scanner) List(ctx context.Context, root *confine.MediaRoot, command nodeproto.List) (
	[]nodeproto.ScanRecord, error,
) {
	var records []nodeproto.ScanRecord
	emit := func(record nodeproto.ScanRecord) error {
		records = append(records, record)
		return nil
	}
	walk := &walk{
		scanner: scanner, ctx: ctx, emit: emit, visited: map[string]bool{},
		errors: newErrorLog(emit, command.ErrorLimit), depth: command.Depth,
	}
	err := walk.run(root, path.Clean(command.Path))
	return records, err
}

// walk is one listing in progress.
type walk struct {
	scanner      *Scanner
	ctx          context.Context //nolint:containedctx // one walk is one call's context
	emit         emitFunc
	visited      map[string]bool
	errors       *errorLog
	skippedCount int
	listedCount  int
	// depth is how many levels below its start the listing goes, 0 for all of them.
	depth int
}

// run walks start, under root, between scanBegin and scanComplete.
func (walk *walk) run(root *confine.MediaRoot, start string) error {
	ctx, emit, scanner := walk.ctx, walk.emit, walk.scanner
	facts, factsErr := root.Facts()
	err := emit(nodeproto.ScanRecord{Type: nodeproto.RecordScanBegin, ScanBegin: &nodeproto.ScanBegin{
		RootPath: root.Path(), UID: scanner.options.UID, GID: scanner.options.GID,
		FilesystemDevice: facts.FilesystemDevice, IsMountPoint: facts.IsMountPoint, IsReadOnly: facts.IsReadOnly,
	}})
	if err != nil {
		return err
	}
	if factsErr != nil {
		// Without the facts the core cannot tell an unmounted disk from an empty one, so the
		// root is not listed: a root error withholds every removal (D40, D41).
		walk.errors.record(".", nodeproto.ScopeRoot, factsErr, nil)
	} else {
		err = walk.walkStart(location{
			mediaRoot: root, relativePath: start, reportedPath: start, device: facts.FilesystemDevice,
		})
	}
	if flushErr := walk.errors.flush(); err == nil {
		err = flushErr
	}
	if err != nil {
		return err
	}
	if ctx.Err() != nil {
		walk.errors.record(".", nodeproto.ScopeRun, ctx.Err(), nil)
		if flushErr := walk.errors.flush(); flushErr != nil {
			return flushErr
		}
		return fmt.Errorf("list %s: %w", root.Path(), ctx.Err())
	}
	return emit(nodeproto.ScanRecord{Type: nodeproto.RecordScanComplete, ScanComplete: &nodeproto.ScanComplete{
		RootPath:          root.Path(),
		SkippedCount:      walk.skippedCount,
		ListedCount:       walk.listedCount,
		ErrorCount:        walk.errors.errorCount,
		IsStoppedByErrors: walk.errors.isStopped,
	}})
}

// walkStart walks where the listing starts: the root, or a directory under it, whose device a mount
// below it is told from.
func (walk *walk) walkStart(start location) error {
	if start.relativePath == "." {
		return walk.walkDirectory(start)
	}
	info, err := start.mediaRoot.Lstat(start.relativePath)
	if err != nil {
		return walk.fail(start.reportedPath, nodeproto.ScopeRoot, err, nil)
	}
	if device, known := confine.DeviceOf(info); known {
		start.device = device
	}
	return walk.walkDirectory(start)
}

// walkDirectory lists everything below place, once per real directory however many links reach it.
func (walk *walk) walkDirectory(place location) error {
	if walk.visited[place.absolute()] {
		return nil
	}
	walk.visited[place.absolute()] = true
	entries, err := place.mediaRoot.ReadDir(place.relativePath)
	if err != nil {
		scope := nodeproto.ScopeSubtree
		if place.reportedPath == "." {
			scope = nodeproto.ScopeRoot
		}
		return walk.fail(place.reportedPath, scope, err, stat(place))
	}
	for _, entry := range entries {
		if walk.ctx.Err() != nil || walk.errors.isStopped {
			return walk.errors.emitErr
		}
		if err := walk.listEntry(place, entry.Name()); err != nil {
			return err
		}
	}
	// Errors collapse within a directory, not across one.
	if err := walk.errors.flush(); err != nil {
		return err
	}
	return walk.errors.emitErr
}

// listEntry lists one directory entry.
func (walk *walk) listEntry(parent location, name string) error {
	place := parent.child(name)
	info, err := place.mediaRoot.Lstat(place.relativePath)
	if err != nil {
		return walk.fail(place.reportedPath, nodeproto.ScopeEntry, err, nil)
	}
	switch mode := info.Mode(); {
	case mode&fs.ModeSymlink != 0:
		return walk.followSymlink(place)
	case mode.IsDir():
		return walk.enter(place, info)
	case mode.IsRegular():
		return walk.list(place, info)
	default:
		return walk.skip(place, nodeproto.SkipNotRegular)
	}
}

// enter lists a directory, then reports it as a mount boundary when it is on another device than
// its parent (D42, D208), then lists what is in it, as deep as the listing goes.
func (walk *walk) enter(place location, info fs.FileInfo) error {
	if err := walk.list(place, info); err != nil {
		return err
	}
	if device, known := confine.DeviceOf(info); known && device != place.device {
		place.device = device
		err := walk.send(nodeproto.ScanRecord{Type: nodeproto.RecordMount, Mount: &nodeproto.Mount{
			Path: toNodePath(place.reportedPath), FilesystemDevice: device,
		}})
		if err != nil {
			return err
		}
	}
	if walk.depth > 0 && place.level >= walk.depth {
		return nil
	}
	return walk.walkDirectory(place)
}

// list reports an entry with its stat.
func (walk *walk) list(place location, info fs.FileInfo) error {
	device, _ := confine.DeviceOf(info)
	inode, _ := confine.InodeOf(info)
	walk.listedCount++
	return walk.send(nodeproto.ScanRecord{Type: nodeproto.RecordListed, Listed: &nodeproto.Listed{
		Path: toNodePath(place.reportedPath), IsDirectory: info.IsDir(), SizeBytes: info.Size(),
		ModifiedAt: info.ModTime().UnixMilli(), FilesystemDevice: device, Inode: inode,
	}})
}

// followSymlink follows a link whose target lies inside a media root, and skips one that leaves them
// all (D206). Below a followed link, entries are read from the target and reported under the
// link's path.
func (walk *walk) followSymlink(place location) error {
	target, isInside, err := walk.scanner.resolve(place)
	if err != nil {
		return walk.fail(place.reportedPath, nodeproto.ScopeEntry, err, nil)
	}
	if !isInside {
		return walk.skip(place, nodeproto.SkipSymlinkOutsideRoots)
	}
	info, err := target.mediaRoot.Lstat(target.relativePath)
	if err != nil {
		return walk.fail(place.reportedPath, nodeproto.ScopeEntry, err, nil)
	}
	switch {
	case info.IsDir():
		return walk.enter(target, info)
	case info.Mode().IsRegular():
		return walk.list(place, info)
	default:
		return walk.skip(place, nodeproto.SkipNotRegular)
	}
}

// resolve is where place's links lead, reported under place's path, and false when that is inside no
// media root (D206).
func (scanner *Scanner) resolve(place location) (location, bool, error) {
	resolved, err := place.mediaRoot.Resolve(place.relativePath)
	if err != nil {
		return location{}, false, fmt.Errorf("follow %s: %w", place.reportedPath, err)
	}
	for _, media := range append([]*confine.MediaRoot{place.mediaRoot}, scanner.roots...) {
		if relativePath, inside := media.RelativeIfInside(resolved); inside {
			return location{
				mediaRoot: media, relativePath: relativePath, reportedPath: place.reportedPath,
				device: place.device, level: place.level,
			}, true, nil
		}
	}
	return location{}, false, nil
}

// fail records an error and returns what stops the walk, if anything does.
func (walk *walk) fail(reported string, scope nodeproto.ErrorScope, cause error, info fs.FileInfo) error {
	walk.errors.record(reported, scope, cause, info)
	return walk.errors.emitErr
}

// send emits a record after any error still pending, so every record keeps its place in the
// listing: an error's position says what was and was not covered around it (D40).
// send emits a record, after the errors before it. Each record is movement: a listing or an
// examining that sends none for its window has stalled on its disk (D233 §5).
func (walk *walk) send(record nodeproto.ScanRecord) error {
	if err := walk.errors.flush(); err != nil {
		return err
	}
	execx.Moved(walk.ctx)
	return walk.emit(record)
}

func (walk *walk) skip(place location, rule nodeproto.SkipRule) error {
	walk.skippedCount++
	return walk.send(nodeproto.ScanRecord{Type: nodeproto.RecordSkip, Skip: &nodeproto.Skip{
		Path: toNodePath(place.reportedPath), Rule: rule,
	}})
}

// toNodePath is a path as a record carries it: exact when it is UTF-8, and with its bytes
// alongside when it is not (D49).
func toNodePath(reported string) nodeproto.Path {
	if utf8.ValidString(reported) {
		return nodeproto.Path{Text: reported}
	}
	return nodeproto.Path{Text: strings.ToValidUTF8(reported, "�"), Raw: []byte(reported)}
}

// stat describes a directory for an error record, when it can be described at all.
func stat(place location) fs.FileInfo {
	info, err := place.mediaRoot.Lstat(place.relativePath)
	if err != nil {
		return nil
	}
	return info
}
