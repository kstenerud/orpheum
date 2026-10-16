// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"path"
	"sync"

	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// notIdentifiedYet is the reason on a file the node has read and identification has not seen.
const notIdentifiedYet = "not identified yet"

// Ingester stores what nodes report (D163, D173, D179): a row for every path a scan enumerated,
// with its filing and reason; a file's identity matched by its payload hash; and tombstones only
// from a scan that completed, outside anything that scan could not read.
type Ingester struct {
	database *store.Database
	clock    clock.Clock
	settings IngestSettings
	logger   *slog.Logger

	mutex        sync.Mutex
	scans        map[scanKey]*scanState // by node and root: guarded by mutex
	onScanEnded  func(ctx context.Context, libraryID int64)
	onPathsFound func(ctx context.Context, nodeID int64, paths []string)
	onNewFiles   func(ctx context.Context)
	// onUnreadRetired is told each row a listing stored unread whose read found a file stored
	// before (SetOnUnreadRetired).
	onUnreadRetired UnreadRetirer
}

// UnreadRetirer retires what the pass filed for a row a listing stored unread, unreadID, whose read
// found it to be fileID, a file stored before that moved or came back: its version and the title
// made from its path, which another package writes. It runs in the ingest's transaction.
type UnreadRetirer func(ctx context.Context, queries *sqlcgen.Queries, unreadID, fileID, nowMs int64) error

// SetOnUnreadRetired sets what retires an unread row's version and title (identify's
// RetireUnreadFile); with none set, only the row is tombstoned.
func (ingester *Ingester) SetOnUnreadRetired(retire UnreadRetirer) {
	ingester.onUnreadRetired = retire
}

// SetOnNewFiles sets what runs once a batch that stored a new file is committed, while its scan
// may still be going: a file read again unchanged is not new. It asks for an identification pass,
// so a film found early in a long scan can be played before the scan ends (the owner, 2026-10-01).
// It must return at once; the node's records wait behind it.
func (ingester *Ingester) SetOnNewFiles(onNewFiles func(ctx context.Context)) {
	ingester.onNewFiles = onNewFiles
}

// SetOnPathsFound sets what runs once a complete scan is committed, with the node and the paths
// it found there: its root and each disk mounted inside it. Work parked on one that was missing
// resumes (D231 §6).
func (ingester *Ingester) SetOnPathsFound(onPathsFound func(ctx context.Context, nodeID int64, paths []string)) {
	ingester.onPathsFound = onPathsFound
}

// SetOnScanEnded sets what runs once a scan's end is committed, with the scanned library's id:
// identification, which reads what the scan stored.
func (ingester *Ingester) SetOnScanEnded(onScanEnded func(ctx context.Context, libraryID int64)) {
	ingester.onScanEnded = onScanEnded
}

// scanKey names a scan in progress: its node, and the root it scans.
type scanKey struct {
	nodeID int64
	root   string
}

// scanState is one scan in progress: which root and library, when it began, the moves it has
// not been able to conclude yet, and the regions it could not read.
type scanState struct {
	root string
	// scope is what the scan lists, relative to root: wholeRoot, or "" for a scan whose records came
	// whole (IngestScan), or a folder a watcher saw change (D272 §9).
	scope              string
	facts              rootFacts
	libraryID          int64 // 0 when no library covers the root: its records are not stored
	startedAt          int64
	pendingMoves       []pendingMove
	mounts             map[string]uint64 // mount boundaries this scan crossed, by path, with their device
	unreadPaths        []string          // paths, as the node reported them, under which nothing may be removed
	isEverythingUnread bool              // the root itself could not be read, or the scan stopped
}

// rootFacts are the root's filesystem facts as its scan began (D42, D208).
type rootFacts struct {
	FilesystemDevice uint64
	IsMountPoint     bool
}

// pendingMove is a new path whose payload hash matches a file this scan has not seen yet. It
// is a move if that file is still unseen when the scan completes, and a duplicate if not (D179).
type pendingMove struct {
	seenFile   observation
	fromFileID int64
	// unreadID is the row its listing stored unread at the new path, or 0 (placeRead).
	unreadID int64
}

// IngestSettings is what the ingest reads from the settings.
type IngestSettings interface {
	Int(ctx context.Context, key settings.Key) (int64, error)
}

// NewIngester returns the ingest.
func NewIngester(
	database *store.Database, clockSource clock.Clock, settingsService IngestSettings, logger *slog.Logger,
) *Ingester {
	return &Ingester{
		database: database, clock: clockSource, settings: settingsService, logger: logger,
		scans: map[scanKey]*scanState{},
	}
}

// IngestScan stores records of the scan of a node's root in one transaction: scan_begin starts it,
// and scan_complete ends it, applying its removals (D39).
func (ingester *Ingester) IngestScan(
	ctx context.Context, nodeID int64, root string, records []nodeproto.ScanRecord,
) error {
	return ingester.write(ctx, scanKey{nodeID: nodeID, root: root}, func(batch *ingestBatch) error {
		for index, record := range records {
			if err := batch.ingestRecord(ctx, cleanRecord(record)); err != nil {
				return fmt.Errorf("record %d (%s): %w", index, record.Type, err)
			}
		}
		return nil
	})
}

// AbandonScan forgets the scan of a node's root, which will not complete: nothing it did not see is
// removed.
func (ingester *Ingester) AbandonScan(nodeID int64, root string) {
	ingester.mutex.Lock()
	defer ingester.mutex.Unlock()
	delete(ingester.scans, scanKey{nodeID: nodeID, root: root})
}

// write runs ingest in one transaction for the scan of key, then says what it stored to those who
// asked: new files, paths found, scans ended.
func (ingester *Ingester) write(ctx context.Context, key scanKey, ingest func(batch *ingestBatch) error) error {
	ingester.mutex.Lock()
	defer ingester.mutex.Unlock()
	nowMs := ingester.clock.Now().UnixMilli()
	var endedLibraryIDs []int64
	var foundPaths []string
	hasNewFiles := false
	err := ingester.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		batch := &ingestBatch{ingester: ingester, queries: queries, key: key, nodeID: key.nodeID, nowMs: nowMs}
		if err := ingest(batch); err != nil {
			return err
		}
		endedLibraryIDs, foundPaths, hasNewFiles = batch.endedLibraryIDs, batch.foundPaths, batch.hasNewFiles
		return nil
	})
	if err != nil {
		return err
	}
	nodeID := key.nodeID
	if ingester.onNewFiles != nil && hasNewFiles {
		ingester.onNewFiles(ctx)
	}
	if ingester.onPathsFound != nil && len(foundPaths) > 0 {
		ingester.onPathsFound(ctx, nodeID, foundPaths)
	}
	if ingester.onScanEnded != nil {
		for _, libraryID := range endedLibraryIDs {
			ingester.onScanEnded(ctx, libraryID)
		}
	}
	return nil
}

// ingestBatch is one transaction's worth of records of one scan.
type ingestBatch struct {
	ingester        *Ingester
	queries         *sqlcgen.Queries
	key             scanKey
	nodeID          int64
	nowMs           int64
	endedLibraryIDs []int64 // libraries whose scan ended in this batch
	// foundPaths are the roots, and the disks mounted inside them, that a complete scan in this
	// batch found.
	foundPaths []string
	// hasNewFiles is whether the batch added a file or brought one back from its tombstone.
	hasNewFiles bool
}

func (batch *ingestBatch) ingestRecord(ctx context.Context, record nodeproto.ScanRecord) error {
	switch record.Type {
	case nodeproto.RecordScanBegin:
		return batch.beginScan(ctx, record.ScanBegin)
	case nodeproto.RecordObserved:
		if scan := batch.scanInProgress(); scan != nil {
			return batch.ingestObserved(ctx, scan, record.Observed)
		}
	case nodeproto.RecordSkip:
		if scan := batch.scanInProgress(); scan != nil {
			return batch.recordFiling(ctx, scan, record.Skip.Path, FilingSkipped, string(record.Skip.Rule), nil)
		}
	case nodeproto.RecordError:
		if scan := batch.scanInProgress(); scan != nil {
			return batch.recordError(ctx, scan, record.Error)
		}
	case nodeproto.RecordScanComplete:
		if scan := batch.scanInProgress(); scan != nil {
			return batch.completeScan(ctx, scan, record.ScanComplete)
		}
	case nodeproto.RecordSidecar:
		if scan := batch.scanInProgress(); scan != nil {
			return batch.recordSidecar(ctx, scan, record.Sidecar)
		}
	case nodeproto.RecordMount:
		if scan := batch.scanInProgress(); scan != nil {
			mountPath := resolveReportedPath(scan.root, record.Mount.Path).text
			scan.mounts[mountPath] = record.Mount.FilesystemDevice
		}
	case nodeproto.RecordListed:
		// A listing is compared, not ingested (TakeListing).
	}
	return nil
}

// scanInProgress is the batch's scan in progress, or nil for a record outside one or under a root no
// library covers.
func (batch *ingestBatch) scanInProgress() *scanState {
	scan := batch.ingester.scans[batch.key]
	if scan == nil || scan.libraryID == 0 {
		return nil
	}
	return scan
}

// beginScan starts a scan of a root. A root no library covers is noted once and its records are
// not stored: a file row belongs to a library (schema.md).
func (batch *ingestBatch) beginScan(ctx context.Context, begin *nodeproto.ScanBegin) error {
	if begin.RootPath != batch.key.root {
		return fmt.Errorf("scan_begin names %s, but the scan is of %s", begin.RootPath, batch.key.root)
	}
	libraryID, err := store.OrNotFound(batch.queries.GetLibraryForRoot(ctx, sqlcgen.GetLibraryForRootParams{
		NodeID: batch.nodeID, RootPath: begin.RootPath,
	}))
	if errors.Is(err, store.ErrNotFound) {
		batch.ingester.logger.WarnContext(ctx, "a node scanned a root no library covers; what it saw is not stored",
			"nodeId", batch.nodeID, "rootPath", begin.RootPath)
		batch.ingester.scans[batch.key] = &scanState{root: begin.RootPath}
		return nil
	}
	if err != nil {
		return err
	}
	batch.ingester.scans[batch.key] = &scanState{
		root: begin.RootPath, libraryID: libraryID, startedAt: batch.nowMs,
		facts:  rootFacts{FilesystemDevice: begin.FilesystemDevice, IsMountPoint: begin.IsMountPoint},
		mounts: map[string]uint64{},
	}
	return nil
}

// reportedPath is a reported path as the file table keeps it: absolute on the node, in NFC for
// display, search and D49's cross-node equality, and the reported bytes, always, as the key the
// file is known by (D211).
type reportedPath struct {
	text          string
	reportedBytes []byte
}

// resolveReportedPath turns a reported path into a reportedPath under the scan's root.
func resolveReportedPath(root string, reported nodeproto.Path) reportedPath {
	raw := reported.Text
	if reported.Raw != nil {
		raw = string(reported.Raw)
	}
	absolute := root
	if reported.Text != "." {
		absolute = path.Join(root, raw)
	}
	// NFC for D49's equality, then the path cleaner, so what is related and shown is safe
	// (principles §22); the bytes are kept too, as bytes, to open the file.
	return reportedPath{text: PathText([]byte(absolute)), reportedBytes: []byte(absolute)}
}

// ErrorDetail is an errored file's platform account, as its row keeps it in error_detail_json
// (D163): the platform's words, the owner, group and mode, and how often the node met it.
type ErrorDetail struct {
	nodeproto.PlatformError
	OccurrenceCount int `json:"occurrenceCount"`
}

// DecodeProbe reads a file row's probe_json: the probe its node reported, as the ingest keeps it.
func DecodeProbe(text string) (nodeproto.Probe, error) {
	var probe nodeproto.Probe
	if err := json.Unmarshal([]byte(text), &probe); err != nil {
		return nodeproto.Probe{}, fmt.Errorf("decode a file's probe: %w", err)
	}
	return probe, nil
}

// DecodeErrorDetail reads a file row's error_detail_json.
func DecodeErrorDetail(text string) (ErrorDetail, error) {
	var detail ErrorDetail
	if err := json.Unmarshal([]byte(text), &detail); err != nil {
		return ErrorDetail{}, fmt.Errorf("decode a file's error detail: %w", err)
	}
	return detail, nil
}

// errorDetail is the error record's platform account, as the file row keeps it (D163).
func errorDetail(failure *nodeproto.ScanError) (*string, error) {
	encoded, err := json.Marshal(ErrorDetail{failure.PlatformError, failure.OccurrenceCount})
	if err != nil {
		return nil, fmt.Errorf("encode the error's detail: %w", err)
	}
	text := string(encoded)
	return &text, nil
}

// decodePayloadHash decodes the node's hex hash for the BLOB column.
func decodePayloadHash(identity nodeproto.Identity) ([]byte, error) {
	decoded, err := hex.DecodeString(identity.PayloadHash)
	if err != nil || len(decoded) == 0 {
		return nil, fmt.Errorf("a payload hash that is not hex: %q", identity.PayloadHash)
	}
	return decoded, nil
}
