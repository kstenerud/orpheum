// SPDX-License-Identifier: GPL-3.0-or-later

package scan

import (
	"context"
	"encoding/hex"
	"fmt"
	"io/fs"
	"path"
	"strings"
	"unicode/utf8"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/hash"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// Prober looks inside one file; probe.Prober is the real one.
type Prober interface {
	Probe(ctx context.Context, absolutePath string) (nodeproto.Probe, error)
}

// Emit sends one record on. An error stops the scan: nobody is listening.
type Emit func(record nodeproto.Record) error

// Options are the scanner's settings, from the node's config file.
type Options struct {
	// VideoExtensions are the extensions, lower case and without the dot, of files worth
	// probing. Anything else is skipped with the rule "extension".
	VideoExtensions []string
	// MaxErrors is how many errors one root may produce before the scanner stops enumerating
	// it and reports the root as unreadable (D40: a dead share must not flood the report).
	MaxErrors int
	// UID and GID are the node's effective ids, reported so an error can be read against them.
	UID, GID int
}

// Scanner walks media roots and reports what it finds (scanner.md). It reports; the core
// decides.
type Scanner struct {
	roots      []*confine.MediaRoot
	prober     Prober
	extensions map[string]bool
	options    Options
}

// NewScanner returns a scanner over roots, every configured media root: a symlink is followed
// only into one of them (D206).
func NewScanner(roots []*confine.MediaRoot, prober Prober, options Options) *Scanner {
	extensions := make(map[string]bool, len(options.VideoExtensions))
	for _, extension := range options.VideoExtensions {
		extensions[strings.ToLower(extension)] = true
	}
	return &Scanner{roots: roots, prober: prober, extensions: extensions, options: options}
}

// location is where an entry is read from — a media root and a path inside it — and the path
// it is reported under, relative to the root being scanned. They differ below a followed link.
type location struct {
	media    *confine.MediaRoot
	rel      string
	reported string
}

func (place location) child(name string) location {
	return location{media: place.media, rel: path.Join(place.rel, name), reported: path.Join(place.reported, name)}
}

func (place location) absolute() string { return path.Join(place.media.Path(), place.rel) }

// Scan walks root and emits scan_begin, then a record for everything it finds, then
// scan_complete. A cancelled scan ends with a run-scoped error and no scan_complete, so the
// core applies no removals from it (D39).
func (scanner *Scanner) Scan(ctx context.Context, root *confine.MediaRoot, emit Emit) error {
	walk := &walk{
		scanner: scanner,
		ctx:     ctx,
		emit:    emit,
		visited: map[string]bool{},
		errors:  newErrorLog(emit, scanner.options.MaxErrors),
	}
	err := emit(nodeproto.Record{Type: nodeproto.RecordScanBegin, ScanBegin: &nodeproto.ScanBegin{
		Root: root.Path(), UID: scanner.options.UID, GID: scanner.options.GID,
	}})
	if err != nil {
		return err
	}
	err = walk.directory(location{media: root, rel: ".", reported: "."})
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
		return fmt.Errorf("scan %s: %w", root.Path(), ctx.Err())
	}
	return emit(nodeproto.Record{Type: nodeproto.RecordScanComplete, ScanComplete: &nodeproto.ScanComplete{
		Root:     root.Path(),
		Observed: walk.observed,
		Skipped:  walk.skipped,
		Errors:   walk.errors.total,
		Stopped:  walk.errors.stopped,
	}})
}

// walk is one scan in progress.
type walk struct {
	scanner  *Scanner
	ctx      context.Context //nolint:containedctx // one walk is one call's context
	emit     Emit
	visited  map[string]bool
	errors   *errorLog
	observed int
	skipped  int
}

// directory reports everything below place, once per real directory however many links reach it.
func (walk *walk) directory(place location) error {
	if walk.visited[place.absolute()] {
		return nil
	}
	walk.visited[place.absolute()] = true
	entries, err := place.media.ReadDir(place.rel)
	if err != nil {
		scope := nodeproto.ScopeSubtree
		if place.reported == "." {
			scope = nodeproto.ScopeRoot
		}
		return walk.fail(place.reported, scope, err, stat(place))
	}
	for _, entry := range entries {
		if walk.ctx.Err() != nil || walk.errors.stopped {
			return walk.errors.err
		}
		if err := walk.entry(place, entry.Name()); err != nil {
			return err
		}
	}
	// Errors collapse within a directory, not across one.
	if err := walk.errors.flush(); err != nil {
		return err
	}
	return walk.errors.err
}

// entry reports one directory entry.
func (walk *walk) entry(parent location, name string) error {
	place := parent.child(name)
	if parent.reported == "." && name == ".trash" {
		return walk.skip(place, nodeproto.SkipTrash)
	}
	info, err := place.media.Lstat(place.rel)
	if err != nil {
		return walk.fail(place.reported, nodeproto.ScopeEntry, err, nil)
	}
	switch mode := info.Mode(); {
	case mode&fs.ModeSymlink != 0:
		return walk.symlink(place)
	case mode.IsDir():
		if isSampleDirectory(name) {
			return walk.skip(place, nodeproto.SkipSample)
		}
		return walk.directory(place)
	case mode.IsRegular():
		return walk.file(place, info)
	default:
		return walk.skip(place, nodeproto.SkipNotRegular)
	}
}

// symlink follows a link whose target lies inside a media root, and skips one that leaves them
// all (D206). Below a followed link, entries are read from the target and reported under the
// link's path.
func (walk *walk) symlink(place location) error {
	resolved, err := place.media.Resolve(place.rel)
	if err != nil {
		return walk.fail(place.reported, nodeproto.ScopeEntry, err, nil)
	}
	for _, media := range walk.scanner.roots {
		rel, inside := media.Contains(resolved)
		if !inside {
			continue
		}
		target := location{media: media, rel: rel, reported: place.reported}
		info, err := media.Lstat(rel)
		if err != nil {
			return walk.fail(place.reported, nodeproto.ScopeEntry, err, nil)
		}
		switch {
		case info.IsDir():
			return walk.directory(target)
		case info.Mode().IsRegular():
			return walk.file(target, info)
		default:
			return walk.skip(place, nodeproto.SkipNotRegular)
		}
	}
	return walk.skip(place, nodeproto.SkipSymlinkOutsideRoots)
}

// file probes and hashes a video file, or skips anything else.
func (walk *walk) file(place location, info fs.FileInfo) error {
	name := path.Base(place.reported)
	extension := strings.ToLower(strings.TrimPrefix(path.Ext(name), "."))
	if !walk.scanner.extensions[extension] {
		return walk.skip(place, nodeproto.SkipExtension)
	}
	if isSampleFile(name) {
		return walk.skip(place, nodeproto.SkipSample)
	}
	identity, err := walk.identity(place, info.Size())
	if err != nil {
		return walk.fail(place.reported, nodeproto.ScopeEntry, err, info)
	}
	probed, err := walk.scanner.prober.Probe(walk.ctx, place.absolute())
	if err != nil {
		return walk.fail(place.reported, nodeproto.ScopeEntry, fmt.Errorf("%w: %w", errProbe, err), info)
	}
	walk.observed++
	return walk.send(nodeproto.Record{Type: nodeproto.RecordObserved, Observed: &nodeproto.Observed{
		Path:     reportedPath(place.reported),
		Size:     info.Size(),
		MtimeMs:  info.ModTime().UnixMilli(),
		Identity: identity,
		Probe:    probed,
	}})
}

func (walk *walk) identity(place location, size int64) (nodeproto.Identity, error) {
	file, err := place.media.Open(place.rel)
	if err != nil {
		return nodeproto.Identity{}, fmt.Errorf("open for the payload hash: %w", err)
	}
	identity, hashErr := hash.Payload(file, size)
	closeErr := file.Close()
	if hashErr != nil {
		return nodeproto.Identity{}, fmt.Errorf("payload hash: %w", hashErr)
	}
	if closeErr != nil {
		return nodeproto.Identity{}, fmt.Errorf("close after the payload hash: %w", closeErr)
	}
	return nodeproto.Identity{
		Scheme:        hash.Scheme,
		Anchor:        string(identity.Anchor),
		Hash:          hex.EncodeToString(identity.Hash[:]),
		PayloadLength: identity.PayloadLength,
	}, nil
}

// fail records an error and returns what stops the walk, if anything does.
func (walk *walk) fail(reported string, scope nodeproto.ErrorScope, cause error, info fs.FileInfo) error {
	walk.errors.record(reported, scope, cause, info)
	return walk.errors.err
}

// send emits a record after any error still pending, so every record keeps its place in the
// enumeration: an error's position says what was and was not covered around it (D40).
func (walk *walk) send(record nodeproto.Record) error {
	if err := walk.errors.flush(); err != nil {
		return err
	}
	return walk.emit(record)
}

func (walk *walk) skip(place location, rule nodeproto.SkipRule) error {
	walk.skipped++
	return walk.send(nodeproto.Record{Type: nodeproto.RecordSkip, Skip: &nodeproto.Skip{
		Path: reportedPath(place.reported), Rule: rule,
	}})
}

// reportedPath is a path as the report carries it: exact when it is UTF-8, and with its bytes
// alongside when it is not (D49).
func reportedPath(reported string) nodeproto.Path {
	if utf8.ValidString(reported) {
		return nodeproto.Path{Text: reported}
	}
	return nodeproto.Path{Text: strings.ToValidUTF8(reported, "�"), Raw: []byte(reported)}
}

// isSampleDirectory is a directory named sample, in any case (D162).
func isSampleDirectory(name string) bool {
	return strings.EqualFold(name, "sample")
}

// isSampleFile is sample.mkv, x.sample.mkv or x-sample.mkv, in any case (D162).
func isSampleFile(name string) bool {
	stem := strings.ToLower(strings.TrimSuffix(name, path.Ext(name)))
	return stem == "sample" || strings.HasSuffix(stem, ".sample") || strings.HasSuffix(stem, "-sample")
}

// stat describes a directory for an error record, when it can be described place all.
func stat(place location) fs.FileInfo {
	info, err := place.media.Lstat(place.rel)
	if err != nil {
		return nil
	}
	return info
}
