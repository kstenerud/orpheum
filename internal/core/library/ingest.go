// SPDX-License-Identifier: GPL-3.0-or-later

package library

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"path"
	"strings"
	"sync"

	"golang.org/x/text/unicode/norm"

	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/textindex"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// The filings the ingest gives; identification gives listed and extra later (D163).
const (
	filingUnidentified = "unidentified"
	filingSkipped      = "skipped"
	filingErrored      = "errored"
)

// notIdentifiedYet is the reason on a file the node has read and identification has not seen.
const notIdentifiedYet = "not identified yet"

// Ingester stores what nodes report (D163, D173, D179): a row for every path a scan enumerated,
// with its filing and reason; a file's identity matched by its payload hash; and tombstones only
// from a scan that completed, outside anything that scan could not read.
type Ingester struct {
	database *store.Database
	clock    clock.Clock
	settings Settings
	logger   *slog.Logger

	mutex     sync.Mutex
	scans     map[int64]*scanState // by node: a node scans one root at a time
	afterScan func(ctx context.Context, libraryID int64)
}

// AfterScan sets what runs once a scan's end is committed, with the scanned library's id:
// identification, which reads what the scan stored.
func (ingester *Ingester) AfterScan(run func(ctx context.Context, libraryID int64)) {
	ingester.afterScan = run
}

// scanState is one scan in progress: which root and library, when it began, the moves it has
// not been able to conclude yet, and the regions it could not read.
type scanState struct {
	root      string
	facts     rootFacts
	libraryID int64 // 0 when no library covers the root: its records are not stored
	startedAt int64
	pending   []pendingMove
	mounts    map[string]uint64 // mount boundaries this scan crossed, by path, with their device
	unread    []string          // paths under which nothing may be removed
	unreadAll bool              // the root itself could not be read, or the scan stopped
}

// rootFacts are the root's filesystem facts as its scan began (D42, D208).
type rootFacts struct {
	Device     uint64
	MountPoint bool
}

// pendingMove is a new path whose payload hash matches a file this scan has not seen yet. It
// is a move if that file is still unseen when the scan completes, and a duplicate if not (D179).
type pendingMove struct {
	seen observation
	from int64
}

// Settings is what the ingest reads from the settings.
type Settings interface {
	Int(ctx context.Context, key settings.Key) (int64, error)
}

// NewIngester returns the ingest.
func NewIngester(database *store.Database, now clock.Clock, settingsService Settings, logger *slog.Logger) *Ingester {
	return &Ingester{
		database: database, clock: now, settings: settingsService, logger: logger, scans: map[int64]*scanState{},
	}
}

// Records stores one batch of a node's report in one transaction.
func (ingester *Ingester) Records(ctx context.Context, nodeID int64, records []nodeproto.Record) error {
	ingester.mutex.Lock()
	defer ingester.mutex.Unlock()
	now := ingester.clock.Now().UnixMilli()
	var ended []int64
	err := ingester.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		batch := &batch{ingester: ingester, queries: queries, nodeID: nodeID, now: now}
		for _, record := range records {
			if err := batch.record(ctx, cleanRecord(record)); err != nil {
				return fmt.Errorf("record %d (%s): %w", record.Seq, record.Type, err)
			}
		}
		ended = batch.ended
		return nil
	})
	if err != nil {
		return err
	}
	if ingester.afterScan != nil {
		for _, libraryID := range ended {
			ingester.afterScan(ctx, libraryID)
		}
	}
	return nil
}

// batch is one transaction's worth of records from one node.
type batch struct {
	ingester *Ingester
	queries  *sqlcgen.Queries
	nodeID   int64
	now      int64
	ended    []int64 // libraries whose scan ended in this batch
}

func (work *batch) record(ctx context.Context, record nodeproto.Record) error {
	switch record.Type {
	case nodeproto.RecordScanBegin:
		return work.begin(ctx, record.ScanBegin)
	case nodeproto.RecordObserved:
		if scan := work.scan(); scan != nil {
			return work.observed(ctx, scan, record.Observed)
		}
	case nodeproto.RecordSkip:
		if scan := work.scan(); scan != nil {
			return work.filed(ctx, scan, record.Skip.Path, filingSkipped, string(record.Skip.Rule), nil)
		}
	case nodeproto.RecordError:
		if scan := work.scan(); scan != nil {
			return work.errored(ctx, scan, record.Error)
		}
	case nodeproto.RecordScanComplete:
		if scan := work.scan(); scan != nil {
			return work.complete(ctx, scan, record.ScanComplete)
		}
	case nodeproto.RecordSidecar:
		if scan := work.scan(); scan != nil {
			return work.sidecar(ctx, scan, record.Sidecar)
		}
	case nodeproto.RecordMount:
		if scan := work.scan(); scan != nil {
			mountPath := nodePath(scan.root, record.Mount.Path).text
			scan.mounts[mountPath] = record.Mount.Device
		}
	case nodeproto.RecordStatus:
	}
	return nil
}

// scan is the node's scan in progress, or nil for a record outside one or under a root no
// library covers.
func (work *batch) scan() *scanState {
	scan := work.ingester.scans[work.nodeID]
	if scan == nil || scan.libraryID == 0 {
		return nil
	}
	return scan
}

// begin starts a scan of a root. A root no library covers is noted once and its records are
// not stored: a file row belongs to a library (schema.md).
func (work *batch) begin(ctx context.Context, begin *nodeproto.ScanBegin) error {
	libraryID, err := store.One(work.queries.GetLibraryForRoot(ctx, sqlcgen.GetLibraryForRootParams{
		NodeID: work.nodeID, Path: begin.Root,
	}))
	if errors.Is(err, store.ErrNotFound) {
		work.ingester.logger.WarnContext(ctx, "a node scanned a root no library covers; what it saw is not stored",
			"nodeId", work.nodeID, "root", begin.Root)
		work.ingester.scans[work.nodeID] = &scanState{root: begin.Root}
		return nil
	}
	if err != nil {
		return err
	}
	work.ingester.scans[work.nodeID] = &scanState{
		root: begin.Root, libraryID: libraryID, startedAt: work.now,
		facts:  rootFacts{Device: begin.Device, MountPoint: begin.MountPoint},
		mounts: map[string]uint64{},
	}
	return nil
}

// filePath is a reported path as the file table keeps it: absolute on the node, in NFC for
// display, search and D49's cross-node equality, with the bytes beside it when they differ, and
// the reported bytes, always, as the key the file is known by (D211).
type filePath struct {
	text string
	raw  []byte // nil when the text is the bytes
	key  []byte
}

// nodePath turns a reported path into a filePath under the scan's root.
func nodePath(root string, reported nodeproto.Path) filePath {
	raw := reported.Text
	if reported.Raw != nil {
		raw = string(reported.Raw)
	}
	absolute := root
	if reported.Text != "." {
		absolute = path.Join(root, raw)
	}
	// NFC for D49's equality, then the path cleaner, so what is related and shown is safe
	// (principles §22); the bytes stay beside it, as bytes, to open the file.
	normal := textindex.Path(norm.NFC.String(strings.ToValidUTF8(absolute, "\uFFFD")))
	result := filePath{text: normal, key: []byte(absolute)}
	if normal != absolute {
		result.raw = []byte(absolute)
	}
	return result
}

// errorDetail is the error record's platform account, as the file row keeps it (D163).
func errorDetail(failure *nodeproto.Error) (*string, error) {
	encoded, err := json.Marshal(struct {
		nodeproto.ErrorRaw
		Count int `json:"count"`
	}{failure.Raw, failure.Count})
	if err != nil {
		return nil, fmt.Errorf("encode the error's detail: %w", err)
	}
	text := string(encoded)
	return &text, nil
}

// payloadHash decodes the node's hex hash for the BLOB column.
func payloadHash(identity nodeproto.Identity) ([]byte, error) {
	decoded, err := hex.DecodeString(identity.Hash)
	if err != nil || len(decoded) == 0 {
		return nil, fmt.Errorf("a payload hash that is not hex: %q", identity.Hash)
	}
	return decoded, nil
}
