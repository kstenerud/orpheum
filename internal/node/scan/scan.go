// SPDX-License-Identifier: GPL-3.0-or-later

package scan

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
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
	// probing. Anything that is neither a video nor a sidecar is skipped with the rule
	// "extension".
	VideoExtensions []string
	// SubtitleExtensions are the extensions of text subtitle files, reported as sidecars.
	SubtitleExtensions []string
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
	subtitles  map[string]bool
	options    Options
}

// maxNFOBytes bounds the NFO a sidecar record carries. Real ones are a few kilobytes; a larger
// one is refused with an error record rather than sent.
const maxNFOBytes = 1 << 20

// errNFOTooLarge is an NFO past maxNFOBytes.
var errNFOTooLarge = errors.New("an NFO file larger than the node sends")

// NewScanner returns a scanner over roots, every configured media root: a symlink is followed
// only into one of them (D206).
func NewScanner(roots []*confine.MediaRoot, prober Prober, options Options) *Scanner {
	return &Scanner{
		roots: roots, prober: prober, extensions: lowerSet(options.VideoExtensions),
		subtitles: lowerSet(options.SubtitleExtensions), options: options,
	}
}

func lowerSet(values []string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, value := range values {
		set[strings.ToLower(value)] = true
	}
	return set
}

// location is where an entry is read from — a media root and a path inside it — and the path
// it is reported under, relative to the root being scanned. They differ below a followed link.
type location struct {
	media    *confine.MediaRoot
	rel      string
	reported string
	device   uint64 // the directory's device, for spotting a mount boundary below it
}

func (place location) child(name string) location {
	return location{
		media: place.media, rel: path.Join(place.rel, name), reported: path.Join(place.reported, name),
		device: place.device,
	}
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
	facts, factsErr := root.Facts()
	err := emit(nodeproto.Record{Type: nodeproto.RecordScanBegin, ScanBegin: &nodeproto.ScanBegin{
		RootPath: root.Path(), UID: scanner.options.UID, GID: scanner.options.GID,
		FilesystemDevice: facts.Device, IsMountPoint: facts.MountPoint, IsReadOnly: facts.ReadOnly,
	}})
	if err != nil {
		return err
	}
	if factsErr != nil {
		// Without the facts the core cannot tell an unmounted disk from an empty one, so the
		// root is not enumerated: a root error withholds every removal (D40, D41).
		walk.errors.record(".", nodeproto.ScopeRoot, factsErr, nil)
	} else {
		err = walk.directory(location{media: root, rel: ".", reported: ".", device: facts.Device})
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
		return fmt.Errorf("scan %s: %w", root.Path(), ctx.Err())
	}
	return emit(nodeproto.Record{Type: nodeproto.RecordScanComplete, ScanComplete: &nodeproto.ScanComplete{
		RootPath:          root.Path(),
		ObservedCount:     walk.observed,
		SidecarCount:      walk.sidecars,
		SkippedCount:      walk.skipped,
		ErrorCount:        walk.errors.total,
		IsStoppedByErrors: walk.errors.stopped,
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
	sidecars int
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
		return walk.enter(place, info)
	case mode.IsRegular():
		return walk.file(place, info)
	default:
		return walk.skip(place, nodeproto.SkipNotRegular)
	}
}

// enter walks a directory, first reporting it as a mount boundary when it is on another device
// than its parent (D42, D208).
func (walk *walk) enter(place location, info fs.FileInfo) error {
	if device, known := confine.DeviceOf(info); known && device != place.device {
		place.device = device
		err := walk.send(nodeproto.Record{Type: nodeproto.RecordMount, Mount: &nodeproto.Mount{
			Path: reportedPath(place.reported), FilesystemDevice: device,
		}})
		if err != nil {
			return err
		}
	}
	return walk.directory(place)
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
		target := location{media: media, rel: rel, reported: place.reported, device: place.device}
		info, err := media.Lstat(rel)
		if err != nil {
			return walk.fail(place.reported, nodeproto.ScopeEntry, err, nil)
		}
		switch {
		case info.IsDir():
			return walk.enter(target, info)
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
	if strings.HasPrefix(name, "._") {
		return walk.skip(place, nodeproto.SkipAppleDouble)
	}
	extension := strings.ToLower(strings.TrimPrefix(path.Ext(name), "."))
	switch {
	case extension == "nfo":
		return walk.sidecar(place, info, nodeproto.SidecarNFO)
	case walk.scanner.subtitles[extension]:
		return walk.sidecar(place, info, nodeproto.SidecarSubtitle)
	case isImage(extension):
		return walk.sidecar(place, info, nodeproto.SidecarImage)
	case !walk.scanner.extensions[extension]:
		return walk.skip(place, nodeproto.SkipExtension)
	}
	if isSampleFile(name) {
		return walk.skip(place, nodeproto.SkipSample)
	}
	identity, err := walk.identity(place, info.Size())
	if err != nil {
		return walk.fail(place.reported, nodeproto.ScopeEntry, err, info)
	}
	providerHash, err := walk.openSubtitlesHash(place, info.Size())
	if err != nil {
		return walk.fail(place.reported, nodeproto.ScopeEntry, err, info)
	}
	probed, err := walk.scanner.prober.Probe(walk.ctx, place.absolute())
	if err != nil {
		return walk.fail(place.reported, nodeproto.ScopeEntry, fmt.Errorf("%w: %w", errProbe, err), info)
	}
	walk.observed++
	return walk.send(nodeproto.Record{Type: nodeproto.RecordObserved, Observed: &nodeproto.Observed{
		Path:       reportedPath(place.reported),
		SizeBytes:  info.Size(),
		ModifiedAt: info.ModTime().UnixMilli(),
		Identity:   identity,
		Probe:      probed,

		OpensubtitlesHash: providerHash,
	}})
}

// openSubtitlesHash is OpenSubtitles' moviehash of a video, from its first and last 64 KiB
// (D224 §3).
func (walk *walk) openSubtitlesHash(place location, size int64) (string, error) {
	file, err := place.media.Open(place.rel)
	if err != nil {
		return "", fmt.Errorf("open for the OpenSubtitles hash: %w", err)
	}
	hashed, hashErr := hash.OpenSubtitles(file, size)
	closeErr := file.Close()
	if hashErr != nil {
		return "", fmt.Errorf("hash for OpenSubtitles: %w", hashErr)
	}
	if closeErr != nil {
		return "", fmt.Errorf("close after the OpenSubtitles hash: %w", closeErr)
	}
	return hashed, nil
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
		Scheme:       hash.Scheme,
		Anchor:       string(identity.Anchor),
		PayloadHash:  hex.EncodeToString(identity.Hash[:]),
		PayloadBytes: identity.PayloadLength,
	}, nil
}

// fail records an error and returns what stops the walk, if anything does.
func (walk *walk) fail(reported string, scope nodeproto.ErrorScope, cause error, info fs.FileInfo) error {
	walk.errors.record(reported, scope, cause, info)
	return walk.errors.err
}

// sidecar reports a metadata or subtitle file, with an NFO's bytes.
func (walk *walk) sidecar(place location, info fs.FileInfo, kind nodeproto.SidecarKind) error {
	record := &nodeproto.Sidecar{
		Path: reportedPath(place.reported), Kind: kind, SizeBytes: info.Size(), ModifiedAt: info.ModTime().UnixMilli(),
	}
	switch kind {
	case nodeproto.SidecarNFO:
		body, err := walk.readNFO(place, info.Size())
		if err != nil {
			return walk.fail(place.reported, nodeproto.ScopeEntry, err, info)
		}
		record.NFOBody = body
	case nodeproto.SidecarSubtitle:
		identity, err := walk.identity(place, info.Size())
		if err != nil {
			return walk.fail(place.reported, nodeproto.ScopeEntry, err, info)
		}
		record.Identity = &identity
	case nodeproto.SidecarImage:
	}
	walk.sidecars++
	return walk.send(nodeproto.Record{Type: nodeproto.RecordSidecar, Sidecar: record})
}

func (walk *walk) readNFO(place location, size int64) ([]byte, error) {
	if size > maxNFOBytes {
		return nil, fmt.Errorf("%w: %d bytes, at most %d", errNFOTooLarge, size, maxNFOBytes)
	}
	file, err := place.media.Open(place.rel)
	if err != nil {
		return nil, fmt.Errorf("open the NFO: %w", err)
	}
	body, readErr := io.ReadAll(io.LimitReader(file, maxNFOBytes+1))
	closeErr := file.Close()
	switch {
	case readErr != nil:
		return nil, fmt.Errorf("read the NFO: %w", readErr)
	case closeErr != nil:
		return nil, fmt.Errorf("close the NFO: %w", closeErr)
	case len(body) > maxNFOBytes:
		return nil, fmt.Errorf("%w: it grew past %d bytes while read", errNFOTooLarge, maxNFOBytes)
	}
	return body, nil
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
	return nodeproto.Path{Text: strings.ToValidUTF8(reported, "�"), Bytes: []byte(reported)}
}

// isImage is an image extension a poster or backdrop beside the media is saved with (D215).
func isImage(extension string) bool {
	switch extension {
	case "jpg", "jpeg", "png", "webp", "tbn":
		return true
	}
	return false
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
