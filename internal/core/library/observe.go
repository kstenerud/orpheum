// SPDX-License-Identifier: GPL-3.0-or-later

package library

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"

	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// ingestObserved matches a file the node read against what the core knows, by D179's table:
//
//	same path, same hash      → the same file
//	same path, other hash     → a different file at that path; the old row is tombstoned (D173)
//	new path, a hash this scan has already seen elsewhere → a duplicate
//	new path, a hash seen before under this root but not yet in this scan → held until the
//	                            scan completes: a move if that file stays unseen, else a duplicate
//	new path, a hash only in another root → a duplicate: that root was not looked at
//	new path, a hash nobody has → a new file
//
// A file moved from one root to another is therefore recorded as a new file, and the old row is
// tombstoned when its root's next scan completes: its history does not follow it.
func (batch *ingestBatch) ingestObserved(ctx context.Context, scan *scanState, observed *nodeproto.Observed) error {
	seenFile, err := observationOf(scan.root, observed)
	if err != nil {
		return err
	}
	existing, err := store.OrNotFound(batch.queries.GetLiveFileByReportedPathRaw(ctx,
		sqlcgen.GetLiveFileByReportedPathRawParams{
			NodeID: batch.nodeID, ReportedPathRaw: seenFile.reportedBytes,
		}))
	switch {
	case err == nil && bytes.Equal(existing.PayloadHash, seenFile.hash):
		return batch.updateObservedFile(ctx, scan, existing.ID, seenFile)
	case err == nil:
		// A different file at the path: the old row's history stays with it (D173).
		tombstone := sqlcgen.TombstoneFileParams{RemovedAt: &batch.nowMs, ID: existing.ID}
		tombstoneErr := batch.queries.TombstoneFile(ctx, tombstone)
		if tombstoneErr != nil {
			return fmt.Errorf("tombstone the file replaced at %s: %w", seenFile.path, tombstoneErr)
		}
	case !errors.Is(err, store.ErrNotFound):
		return err
	default:
		elsewhere, listErr := batch.queries.ListLiveFilesByPayloadHash(ctx, sqlcgen.ListLiveFilesByPayloadHashParams{
			NodeID: batch.nodeID, PayloadHash: seenFile.hash,
		})
		if listErr != nil {
			return fmt.Errorf("look for %s's payload elsewhere: %w", seenFile.path, listErr)
		}
		// Only a file under this scan's root can be judged absent by it. One in another root was
		// simply not looked at, so a match there is a duplicate, never a move.
		unseenIndex := slices.IndexFunc(elsewhere, func(row sqlcgen.ListLiveFilesByPayloadHashRow) bool {
			_, under := RelativeToRoot(row.ReportedPathRaw, scan.root)
			return row.LastSeenAt < scan.startedAt && under
		})
		if unseenIndex >= 0 && len(elsewhere) == 1 {
			move := pendingMove{seenFile: seenFile, fromFileID: elsewhere[unseenIndex].ID}
			scan.pendingMoves = append(scan.pendingMoves, move)
			return nil
		}
		if len(elsewhere) > 0 {
			return batch.insertFile(ctx, scan, seenFile) // a duplicate of a live file
		}
	}
	return batch.reviveOrInsert(ctx, scan, seenFile)
}

// reviveOrInsert brings back the latest tombstone in the library with this payload, with its
// history, wherever the payload now is — a film deleted by mistake and copied back, a disk that
// returned after its files were removed — or else adds a new file (D208 §4, D41).
func (batch *ingestBatch) reviveOrInsert(ctx context.Context, scan *scanState, seenFile observation) error {
	tombstone, err := store.OrNotFound(batch.queries.GetLatestTombstoneByPayloadHash(ctx,
		sqlcgen.GetLatestTombstoneByPayloadHashParams{
			LibraryID: scan.libraryID, PayloadHash: seenFile.hash,
		}))
	if errors.Is(err, store.ErrNotFound) {
		return batch.insertFile(ctx, scan, seenFile)
	}
	if err != nil {
		return err
	}
	if err := batch.queries.ReviveFile(ctx, tombstone); err != nil {
		return fmt.Errorf("revive file %d at %s: %w", tombstone, seenFile.path, err)
	}
	return batch.updateObservedFile(ctx, scan, tombstone, seenFile)
}

// observation is an observed record resolved to the file table's terms.
type observation struct {
	path          string
	reportedBytes []byte
	hash          []byte
	probeJSON     string
	observed      *nodeproto.Observed
	// openSubtitlesHash is the file's OpenSubtitles moviehash, or nil when the node gave none a
	// provider would accept (D224 §3).
	openSubtitlesHash *string
}

// openSubtitlesShape is the only moviehash kept: 16 lowercase hex digits, as OpenSubtitles writes it.
var openSubtitlesShape = regexp.MustCompile(`^[0-9a-f]{16}$`)

// cleanOpenSubtitlesHash is the moviehash a node reported, cleaned where it enters: anything else is
// none, so nothing a node says reaches the provider's query unchecked.
func cleanOpenSubtitlesHash(reported string) *string {
	if !openSubtitlesShape.MatchString(reported) {
		return nil
	}
	return &reported
}

func observationOf(root string, observed *nodeproto.Observed) (observation, error) {
	reported := resolveReportedPath(root, observed.Path)
	hash, err := decodePayloadHash(observed.Identity)
	if err != nil {
		return observation{}, err
	}
	probe, err := json.Marshal(observed.Probe)
	if err != nil {
		return observation{}, fmt.Errorf("encode the probe of %s: %w", reported.text, err)
	}
	return observation{
		path: reported.text, reportedBytes: reported.reportedBytes, hash: hash, probeJSON: string(probe),
		observed:          observed,
		openSubtitlesHash: cleanOpenSubtitlesHash(observed.OpensubtitlesHash),
	}, nil
}

// insertFile adds a new file row, unidentified until identification sees it, and its streams.
func (batch *ingestBatch) insertFile(ctx context.Context, scan *scanState, seenFile observation) error {
	observed, reason := seenFile.observed, notIdentifiedYet
	id, err := batch.queries.InsertFile(ctx, sqlcgen.InsertFileParams{
		LibraryID: scan.libraryID, NodeID: batch.nodeID, Path: seenFile.path, ReportedPathRaw: seenFile.reportedBytes,
		SizeBytes: &observed.SizeBytes, ModifiedAt: &observed.ModifiedAt, PayloadHash: seenFile.hash,
		PayloadBytes: &observed.Identity.PayloadBytes, Filing: FilingUnidentified, FilingReason: &reason,
		ProbeJSON: &seenFile.probeJSON, SeenAt: batch.nowMs, OpensubtitlesHash: seenFile.openSubtitlesHash,
	})
	if err != nil {
		return fmt.Errorf("insert %s: %w", seenFile.path, err)
	}
	return batch.syncStreams(ctx, id, observed.Probe)
}

// updateObservedFile records a file observed again, perhaps at a new path (a move), and re-derives its
// streams from the new probe.
func (batch *ingestBatch) updateObservedFile(
	ctx context.Context, scan *scanState, id int64, seenFile observation,
) error {
	observed, reason := seenFile.observed, notIdentifiedYet
	err := batch.queries.UpdateObservedFile(ctx, sqlcgen.UpdateObservedFileParams{
		LibraryID: scan.libraryID, Path: seenFile.path, ReportedPathRaw: seenFile.reportedBytes,
		SizeBytes:  &observed.SizeBytes,
		ModifiedAt: &observed.ModifiedAt, PayloadHash: seenFile.hash, PayloadBytes: &observed.Identity.PayloadBytes,
		UnidentifiedReason: &reason, ProbeJSON: &seenFile.probeJSON, OpensubtitlesHash: seenFile.openSubtitlesHash,
		SeenAt: batch.nowMs,
		ID:     id,
	})
	if err != nil {
		return fmt.Errorf("update %s: %w", seenFile.path, err)
	}
	return batch.syncStreams(ctx, id, observed.Probe)
}

// recordFiling records a path the node skipped or could not read: an existing row keeps its identity
// and takes the filing; a new path gets a row with no payload.
func (batch *ingestBatch) recordFiling(
	ctx context.Context, scan *scanState, reported nodeproto.Path, filing, reason string, detail *string,
) error {
	located := resolveReportedPath(scan.root, reported)
	existing, err := store.OrNotFound(batch.queries.GetLiveFileByReportedPathRaw(ctx,
		sqlcgen.GetLiveFileByReportedPathRawParams{
			NodeID: batch.nodeID, ReportedPathRaw: located.reportedBytes,
		}))
	if err == nil {
		err = batch.queries.UpdateFileFilingAndLastSeen(ctx, sqlcgen.UpdateFileFilingAndLastSeenParams{
			Filing: filing, FilingReason: &reason, ErrorDetailJSON: detail, SeenAt: batch.nowMs, ID: existing.ID,
		})
		if err != nil {
			return fmt.Errorf("file %s as %s: %w", located.text, filing, err)
		}
		return nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return err
	}
	_, err = batch.queries.InsertFile(ctx, sqlcgen.InsertFileParams{
		LibraryID: scan.libraryID, NodeID: batch.nodeID, Path: located.text, ReportedPathRaw: located.reportedBytes,
		Filing: filing, FilingReason: &reason, ErrorDetailJSON: detail, SeenAt: batch.nowMs,
	})
	if err != nil {
		return fmt.Errorf("insert %s as %s: %w", located.text, filing, err)
	}
	return nil
}

// recordError records what the node could not read, and marks the region no removal may touch:
// an entry is unknown, not absent; a subtree hides everything below it; a root or a stopped
// scan hides everything (D40).
func (batch *ingestBatch) recordError(ctx context.Context, scan *scanState, failure *nodeproto.ScanError) error {
	switch failure.Scope {
	case nodeproto.ScopeRoot, nodeproto.ScopeRun:
		scan.isEverythingUnread = true
	case nodeproto.ScopeEntry, nodeproto.ScopeSubtree:
		scan.unreadPaths = append(scan.unreadPaths, string(resolveReportedPath(scan.root, failure.Path).reportedBytes))
	}
	if failure.Path.Text == "." {
		return nil // the root itself is not a file
	}
	detail, err := errorDetail(failure)
	if err != nil {
		return err
	}
	return batch.recordFiling(ctx, scan, failure.Path, FilingErrored, string(failure.Code), detail)
}
