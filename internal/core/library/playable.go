// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"cmp"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// PlayableFile is a live video a person may play, probed and hashed: what a play, a run of chunks, an
// extraction and a measurement read. A file that is not one is ErrNoFile.
func PlayableFile(ctx context.Context, queries *sqlcgen.Queries, fileID int64) (sqlcgen.GetPlayableFileRow, error) {
	file, err := store.OrNotFound(queries.GetPlayableFile(ctx, fileID))
	if errors.Is(err, store.ErrNotFound) {
		return file, fmt.Errorf("file %d: %w", fileID, ErrNoFile)
	}
	if err != nil {
		return file, fmt.Errorf("read file %d: %w", fileID, err)
	}
	if file.PayloadHash == nil {
		// Stored by its listing, not read yet (D304 §4): it cannot be played until it is.
		return file, &UnreadableError{Unreadable{Reason: StillReading, NodeID: file.NodeID}}
	}
	if file.ProbeJSON == nil {
		return file, fmt.Errorf("%w: file %d was never probed", ErrNoFile, fileID)
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

// Located is where a file's bytes are read from: a place of it (D363 §2), its node and the path its
// node reported, and its payload hash. Subject names what was located, "file 12" or "stream 40", for
// an error.
type Located struct {
	Subject         string
	NodeID          int64
	ReportedPathRaw []byte
	PayloadHash     []byte
	// LibraryIDs are the live libraries that cover the place, of those its reader reaches: who may go
	// on reading it (D363 §8). PlaceToRead's alone; empty from LocatedOf.
	LibraryIDs []int64
}

// LocatedOf is where a playable file's bytes are at its first place.
func LocatedOf(file sqlcgen.GetPlayableFileRow) Located {
	return Located{
		Subject: fmt.Sprintf("file %d", file.ID), NodeID: file.NodeID,
		ReportedPathRaw: file.ReportedPathRaw, PayloadHash: file.PayloadHash,
	}
}

// PlaceToRead is where a playable file is read from (D363 §8): of its live places that a library reach
// reaches covers, every one for a reach of every library, the first that can be read now, its first
// place before its copies; else the first of them, whose being unreadable says why. A disk that stops
// answering so falls back to another of the places reach reaches, and to none other; so does a place
// at which a read found the file not there, among absences, until a scan sees it there again (nil
// knows of none). A file at no place reach reaches is ErrNoFile. Work no one person asked for, a run
// of chunks or a measurement, reads with every library's reach: the bytes of a copy are the file's,
// and whether a person may play is judged by their own places.
func PlaceToRead(
	ctx context.Context, queries *sqlcgen.Queries, connections NodeConnections, absences *Absences,
	reach auth.Reach, file sqlcgen.GetPlayableFileRow,
) (Located, error) {
	return placeToRead(ctx, queries, connections, absences, file, placesOf{reach: reach})
}

// PlaceOnNodeToRead is PlaceToRead among the file's places on one node: work whose output stays on its
// node, a run of chunks in that node's store, falls back only to a copy beside it.
func PlaceOnNodeToRead(
	ctx context.Context, queries *sqlcgen.Queries, connections NodeConnections, absences *Absences,
	file sqlcgen.GetPlayableFileRow, nodeID int64,
) (Located, error) {
	return placeToRead(ctx, queries, connections, absences, file, placesOf{reach: auth.EveryLibrary(), nodeID: nodeID})
}

// placesOf are the places of a file a read may be from: those reach reaches, on nodeID alone when it
// is not 0.
type placesOf struct {
	reach  auth.Reach
	nodeID int64
}

// placeToRead is PlaceToRead, of the places among.
func placeToRead(
	ctx context.Context, queries *sqlcgen.Queries, connections NodeConnections, absences *Absences,
	file sqlcgen.GetPlayableFileRow, among placesOf,
) (Located, error) {
	reach, nodeID := among.reach, among.nodeID
	rows, err := queries.ListPlacesOfFile(ctx, file.ID)
	if err != nil {
		return Located{}, fmt.Errorf("list the places of file %d: %w", file.ID, err)
	}
	var reached []Located
	for _, row := range rows {
		var covering []int64
		if err := json.Unmarshal([]byte(row.LibraryIdsJSON), &covering); err != nil {
			return Located{}, fmt.Errorf("read the libraries covering place %d: %w", row.PlaceID, err)
		}
		reachedThere := slices.DeleteFunc(covering, func(libraryID int64) bool { return !reach.Reaches(&libraryID) })
		if (!reach.IsEveryLibrary() && len(reachedThere) == 0) || (nodeID != 0 && row.NodeID != nodeID) {
			continue
		}
		reached = append(reached, Located{
			Subject: fmt.Sprintf("file %d", file.ID), NodeID: row.NodeID, ReportedPathRaw: row.ReportedPathRaw,
			PayloadHash: file.PayloadHash, LibraryIDs: reachedThere,
		})
	}
	if len(reached) == 0 {
		return Located{}, fmt.Errorf("file %d at no place its reader reaches: %w", file.ID, ErrNoFile)
	}
	// A place a read found the file not at is the last tried: its read says the file is not there.
	present := slices.DeleteFunc(slices.Clone(reached), func(place Located) bool {
		return absences.IsAbsent(place.NodeID, place.ReportedPathRaw)
	})
	if len(present) == 0 {
		return reached[0], nil
	}
	reached = present
	if len(reached) == 1 || connections == nil {
		return reached[0], nil
	}
	for _, place := range reached {
		_, isUnreadable, readErr := UnreadableFile(ctx, queries, connections, place.NodeID, place.ReportedPathRaw)
		if readErr != nil {
			return Located{}, readErr
		}
		if !isUnreadable {
			return place, nil
		}
	}
	return reached[0], nil
}

// BytesFileOf names a file on its node's byte socket: the deepest live library folder on the node it
// is under, its path relative to that folder, and its payload hash.
func BytesFileOf(ctx context.Context, queries *sqlcgen.Queries, file Located) (nodeproto.BytesFile, error) {
	roots, err := queries.ListLiveRootPathsOnNode(ctx, file.NodeID)
	if err != nil {
		return nodeproto.BytesFile{}, fmt.Errorf("read node %d's library folders: %w", file.NodeID, err)
	}
	root, relative, found := RootHolding(file.ReportedPathRaw, roots)
	if !found {
		return nodeproto.BytesFile{}, fmt.Errorf("%w: %s is under no library's folder", ErrNoFile, file.Subject)
	}
	return nodeproto.BytesFile{RootPath: root, Path: relative, PayloadHash: hex.EncodeToString(file.PayloadHash)}, nil
}

// UnreadableError is a play of a file that cannot be read now: its node away, or its root or a disk
// inside it missing (D241 §4).
type UnreadableError struct {
	Unreadable
}

// Error says since when only when that is known: a node not heard since the core started has no time,
// which read "since 0001-01-01T00:00:00Z" (the production NAS, 2026-10-18).
func (unreadable *UnreadableError) Error() string {
	text := fmt.Sprintf("node %d's file cannot be read: %s %s", unreadable.NodeID, unreadable.Reason,
		cmp.Or(unreadable.Path, "-"))
	if unreadable.Since.IsZero() {
		return text
	}
	return text + ", since " + unreadable.Since.UTC().Format(time.RFC3339)
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
