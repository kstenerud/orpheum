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

// observed matches a file the node read against what the core knows, by D179's table:
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
func (work *batch) observed(ctx context.Context, scan *scanState, observed *nodeproto.Observed) error {
	seen, err := observationOf(scan.root, observed)
	if err != nil {
		return err
	}
	existing, err := store.One(work.queries.GetLiveFileByKey(ctx, sqlcgen.GetLiveFileByKeyParams{
		NodeID: work.nodeID, ReportedPathBytes: seen.key,
	}))
	switch {
	case err == nil && bytes.Equal(existing.PayloadHash, seen.hash):
		return work.update(ctx, scan, existing.ID, seen)
	case err == nil:
		// A different file at the path: the old row's history stays with it (D173).
		tombstone := sqlcgen.TombstoneFileParams{RemovedAt: &work.now, ID: existing.ID}
		tombstoneErr := work.queries.TombstoneFile(ctx, tombstone)
		if tombstoneErr != nil {
			return fmt.Errorf("tombstone the file replaced at %s: %w", seen.path, tombstoneErr)
		}
	case !errors.Is(err, store.ErrNotFound):
		return err
	default:
		elsewhere, listErr := work.queries.ListLiveFilesByHash(ctx, sqlcgen.ListLiveFilesByHashParams{
			NodeID: work.nodeID, PayloadHash: seen.hash,
		})
		if listErr != nil {
			return fmt.Errorf("look for %s's payload elsewhere: %w", seen.path, listErr)
		}
		// Only a file under this scan's root can be judged absent by it. One in another root was
		// simply not looked at, so a match there is a duplicate, never a move.
		unseen := slices.IndexFunc(elsewhere, func(row sqlcgen.ListLiveFilesByHashRow) bool {
			return row.LastSeenAt < scan.startedAt && under(row.Path, scan.root)
		})
		if unseen >= 0 && len(elsewhere) == 1 {
			scan.pending = append(scan.pending, pendingMove{seen: seen, from: elsewhere[unseen].ID})
			return nil
		}
		if len(elsewhere) > 0 {
			return work.insert(ctx, scan, seen) // a duplicate of a live file
		}
	}
	return work.reviveOrInsert(ctx, scan, seen)
}

// reviveOrInsert brings back the latest tombstone in the library with this payload, with its
// history, wherever the payload now is — a film deleted by mistake and copied back, a disk that
// returned after its files were removed — or else adds a new file (D208 §4, D41).
func (work *batch) reviveOrInsert(ctx context.Context, scan *scanState, seen observation) error {
	tombstone, err := store.One(work.queries.GetLatestTombstoneByHash(ctx, sqlcgen.GetLatestTombstoneByHashParams{
		LibraryID: scan.libraryID, PayloadHash: seen.hash,
	}))
	if errors.Is(err, store.ErrNotFound) {
		return work.insert(ctx, scan, seen)
	}
	if err != nil {
		return err
	}
	if err := work.queries.ReviveFile(ctx, tombstone); err != nil {
		return fmt.Errorf("revive file %d at %s: %w", tombstone, seen.path, err)
	}
	return work.update(ctx, scan, tombstone, seen)
}

// observation is an observed record resolved to the file table's terms.
type observation struct {
	path     string
	key      []byte
	hash     []byte
	probe    string
	observed *nodeproto.Observed
	// openSubtitlesHash is the file's OpenSubtitles moviehash, or nil when the node gave none a
	// provider would accept (D224 §3).
	openSubtitlesHash *string
}

// openSubtitlesShape is the only moviehash kept: 16 lowercase hex digits, as OpenSubtitles writes it.
var openSubtitlesShape = regexp.MustCompile(`^[0-9a-f]{16}$`)

// openSubtitlesHash is the moviehash a node reported, cleaned where it enters: anything else is
// none, so nothing a node says reaches the provider's query unchecked.
func openSubtitlesHash(reported string) *string {
	if !openSubtitlesShape.MatchString(reported) {
		return nil
	}
	return &reported
}

func observationOf(root string, observed *nodeproto.Observed) (observation, error) {
	reported := nodePath(root, observed.Path)
	hash, err := payloadHash(observed.Identity)
	if err != nil {
		return observation{}, err
	}
	probe, err := json.Marshal(observed.Probe)
	if err != nil {
		return observation{}, fmt.Errorf("encode the probe of %s: %w", reported.text, err)
	}
	return observation{
		path: reported.text, key: reported.key, hash: hash, probe: string(probe), observed: observed,
		openSubtitlesHash: openSubtitlesHash(observed.OpenSubtitlesHash),
	}, nil
}

// insert adds a new file row, unidentified until identification sees it, and its streams.
func (work *batch) insert(ctx context.Context, scan *scanState, seen observation) error {
	observed, reason := seen.observed, notIdentifiedYet
	id, err := work.queries.InsertFile(ctx, sqlcgen.InsertFileParams{
		LibraryID: scan.libraryID, NodeID: work.nodeID, Path: seen.path, ReportedPathBytes: seen.key,
		SizeBytes: &observed.SizeBytes, MtimeMs: &observed.MtimeMs, PayloadHash: seen.hash,
		PayloadBytes: &observed.Identity.PayloadBytes, Filing: filingUnidentified, FilingReason: &reason,
		ProbeJson: &seen.probe, SeenAt: work.now, OpensubtitlesHash: seen.openSubtitlesHash,
	})
	if err != nil {
		return fmt.Errorf("insert %s: %w", seen.path, err)
	}
	return work.streams(ctx, id, observed.Probe)
}

// update records a file observed again, perhaps at a new path (a move), and re-derives its
// streams from the new probe.
func (work *batch) update(ctx context.Context, scan *scanState, id int64, seen observation) error {
	observed, reason := seen.observed, notIdentifiedYet
	err := work.queries.UpdateObservedFile(ctx, sqlcgen.UpdateObservedFileParams{
		LibraryID: scan.libraryID, Path: seen.path, ReportedPathBytes: seen.key,
		SizeBytes: &observed.SizeBytes,
		MtimeMs:   &observed.MtimeMs, PayloadHash: seen.hash, PayloadBytes: &observed.Identity.PayloadBytes,
		UnidentifiedReason: &reason, ProbeJson: &seen.probe, OpensubtitlesHash: seen.openSubtitlesHash,
		SeenAt: work.now,
		ID:     id,
	})
	if err != nil {
		return fmt.Errorf("update %s: %w", seen.path, err)
	}
	return work.streams(ctx, id, observed.Probe)
}

// filed records a path the node skipped or could not read: an existing row keeps its identity
// and takes the filing; a new path gets a row with no payload.
func (work *batch) filed(
	ctx context.Context, scan *scanState, reported nodeproto.Path, filing, reason string, detail *string,
) error {
	located := nodePath(scan.root, reported)
	existing, err := store.One(work.queries.GetLiveFileByKey(ctx, sqlcgen.GetLiveFileByKeyParams{
		NodeID: work.nodeID, ReportedPathBytes: located.key,
	}))
	if err == nil {
		err = work.queries.UpdateFileFiling(ctx, sqlcgen.UpdateFileFilingParams{
			Filing: filing, FilingReason: &reason, ErrorDetail: detail, SeenAt: work.now, ID: existing.ID,
		})
		if err != nil {
			return fmt.Errorf("file %s as %s: %w", located.text, filing, err)
		}
		return nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return err
	}
	_, err = work.queries.InsertFile(ctx, sqlcgen.InsertFileParams{
		LibraryID: scan.libraryID, NodeID: work.nodeID, Path: located.text, ReportedPathBytes: located.key,
		Filing: filing, FilingReason: &reason, ErrorDetail: detail, SeenAt: work.now,
	})
	if err != nil {
		return fmt.Errorf("insert %s as %s: %w", located.text, filing, err)
	}
	return nil
}

// errored records what the node could not read, and marks the region no removal may touch:
// an entry is unknown, not absent; a subtree hides everything below it; a root or a stopped
// scan hides everything (D40).
func (work *batch) errored(ctx context.Context, scan *scanState, failure *nodeproto.Error) error {
	switch failure.Scope {
	case nodeproto.ScopeRoot, nodeproto.ScopeRun:
		scan.unreadAll = true
	case nodeproto.ScopeEntry, nodeproto.ScopeSubtree:
		scan.unread = append(scan.unread, nodePath(scan.root, failure.Path).text)
	}
	if failure.Path.Text == "." {
		return nil // the root itself is not a file
	}
	detail, err := errorDetail(failure)
	if err != nil {
		return err
	}
	return work.filed(ctx, scan, failure.Path, filingErrored, string(failure.Code), detail)
}
