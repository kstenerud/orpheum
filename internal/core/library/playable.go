// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"cmp"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// PlayableFile is a live video a person may play, probed and hashed: what a play, a prepare, an
// extraction and a measurement read. A file that is not one is ErrNoFile.
func PlayableFile(ctx context.Context, queries *sqlcgen.Queries, fileID int64) (sqlcgen.GetPlayableFileRow, error) {
	file, err := store.OrNotFound(queries.GetPlayableFile(ctx, fileID))
	if errors.Is(err, store.ErrNotFound) {
		return file, fmt.Errorf("file %d: %w", fileID, ErrNoFile)
	}
	if err != nil {
		return file, fmt.Errorf("read file %d: %w", fileID, err)
	}
	if file.ProbeJSON == nil || file.PayloadHash == nil {
		return file, fmt.Errorf("%w: file %d was never probed and hashed", ErrNoFile, fileID)
	}
	return file, nil
}

// ProbeOf is a file's stored probe; a file never probed is ErrNoFile.
func ProbeOf(stored *string) (nodeproto.Probe, error) {
	if stored == nil {
		return nodeproto.Probe{}, ErrNoFile
	}
	return DecodeProbe(*stored)
}

// Located is where a file's bytes are: its library, its node, the path its node reported, and its
// payload hash. Subject names what was located, "file 12" or "stream 40", for an error.
type Located struct {
	Subject         string
	LibraryID       int64
	NodeID          int64
	ReportedPathRaw []byte
	PayloadHash     []byte
}

// LocatedOf is where a playable file's bytes are.
func LocatedOf(file sqlcgen.GetPlayableFileRow) Located {
	return Located{
		Subject: fmt.Sprintf("file %d", file.ID), LibraryID: file.LibraryID, NodeID: file.NodeID,
		ReportedPathRaw: file.ReportedPathRaw, PayloadHash: file.PayloadHash,
	}
}

// BytesFileOf names a file on its node's byte socket: the library root it is under, its path
// relative to that root, and its payload hash.
func BytesFileOf(ctx context.Context, queries *sqlcgen.Queries, file Located) (nodeproto.BytesFile, error) {
	roots, err := queries.ListLibraryRootsOnNode(ctx, sqlcgen.ListLibraryRootsOnNodeParams{
		LibraryID: file.LibraryID, NodeID: file.NodeID,
	})
	if err != nil {
		return nodeproto.BytesFile{}, fmt.Errorf("read the library's roots: %w", err)
	}
	root, relative, found := RootHolding(file.ReportedPathRaw, roots)
	if !found {
		return nodeproto.BytesFile{}, fmt.Errorf("%w: %s is under none of its library's roots", ErrNoFile, file.Subject)
	}
	return nodeproto.BytesFile{RootPath: root, Path: relative, PayloadHash: hex.EncodeToString(file.PayloadHash)}, nil
}

// UnreadableError is a play of a file that cannot be read now: its node away, or its root or a disk
// inside it missing (D241 §4).
type UnreadableError struct {
	Unreadable
}

func (unreadable *UnreadableError) Error() string {
	return fmt.Sprintf("node %d's file cannot be read: %s %s, since %s", unreadable.NodeID, unreadable.Reason,
		cmp.Or(unreadable.Path, "-"), unreadable.Since.UTC().Format(time.RFC3339))
}

// RefuseUnreadable refuses a play of the file a node reported at reportedPath when it cannot be read
// now (UnreadableError); nil when it can.
func RefuseUnreadable(
	ctx context.Context, queries *sqlcgen.Queries, connections NodeConnections, nodeID int64, reportedPath []byte,
) error {
	unreadable, isUnreadable, err := UnreadableFile(ctx, queries, connections, nodeID, reportedPath)
	if err != nil {
		return fmt.Errorf("ask whether the file can be read: %w", err)
	}
	if isUnreadable {
		return &UnreadableError{unreadable}
	}
	return nil
}

// ParkIfUnavailable answers that a job reading file waits, at no cost of an attempt, while the root
// or the mounted disk that holds it is missing, until a scan finds it (D231 §5); nil when it is
// there. A play-driven job whose file cannot be read ends instead, its node away included: no one can
// watch it, so no one waits for its work (D241 §4).
func ParkIfUnavailable(
	ctx context.Context, queries *sqlcgen.Queries, connections NodeConnections, job jobs.Job, file Located,
) error {
	if job.Priority >= jobs.PriorityPlayNow {
		err := RefuseUnreadable(ctx, queries, connections, file.NodeID, file.ReportedPathRaw)
		if errors.As(err, new(*UnreadableError)) {
			return jobs.MarkPermanent(fmt.Errorf("no one can watch %s: %w", file.Subject, err))
		}
		return err
	}
	return ParkIfUnder(ctx, queries, file.NodeID, file.ReportedPathRaw, file.Subject)
}

// ParkIfUnder parks the background work on a file, named subject, that is under a root or disk
// that is unavailable, until a scan finds it again (D231 §5); nil when all are there. Its wait is
// not the file's own failure, which would be kept for good.
func ParkIfUnder(
	ctx context.Context, queries *sqlcgen.Queries, nodeID int64, reportedPath []byte, subject string,
) error {
	missing, isMissing, err := UnavailableUnder(ctx, queries, nodeID, reportedPath)
	if err != nil || !isMissing {
		return err
	}
	return jobs.ParkOnMissingRoot(nodeID, missing.Path, missing.Since, fmt.Errorf("%s is under it", subject))
}
