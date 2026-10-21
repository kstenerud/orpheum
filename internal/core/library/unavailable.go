// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"context"
	"fmt"
	"time"

	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// Unavailable is a root, or a disk mounted inside one, that the last scan found missing, or whose
// disk work stalled on: the files under it are not there to read, and work that reads them waits for
// a scan that finds it (D208, D231 §5, D233 §10).
type Unavailable struct {
	// Path is the root's or the mount's path on its node.
	Path string
	// Since is when it went: the first scan in a row that found it missing, or the stall.
	Since time.Time
	// IsMount is a disk mounted inside a root, rather than the root.
	IsMount bool
}

// UnavailableUnder is the missing root or mount that holds a file on a node, by the path its node
// reported: the deepest, since a mount inside a root is what went; false when all are there.
func UnavailableUnder(
	ctx context.Context, queries *sqlcgen.Queries, nodeID int64, reportedPath []byte,
) (Unavailable, bool, error) {
	return NewUnavailability(queries).Under(ctx, nodeID, reportedPath)
}

// Unavailability answers UnavailableUnder for many files, reading each node's missing roots and
// mounts once: a sweep over every file asks it of each (D231 §5, D233 §10).
type Unavailability struct {
	queries *sqlcgen.Queries
	// byNode is each node's missing roots and mounts, read when first asked.
	byNode map[int64][]Unavailable
}

// NewUnavailability answers from what queries read now.
func NewUnavailability(queries *sqlcgen.Queries) *Unavailability {
	return &Unavailability{queries: queries, byNode: map[int64][]Unavailable{}}
}

// Under is UnavailableUnder for one file.
func (unavailability *Unavailability) Under(
	ctx context.Context, nodeID int64, reportedPath []byte,
) (Unavailable, bool, error) {
	candidates, isRead := unavailability.byNode[nodeID]
	if !isRead {
		var err error
		if candidates, err = unavailableOfNode(ctx, unavailability.queries, nodeID); err != nil {
			return Unavailable{}, false, err
		}
		unavailability.byNode[nodeID] = candidates
	}
	found, isFound := deepestUnder(candidates, reportedPath)
	return found, isFound, nil
}

// unavailableOfNode is every place of a node that cannot be read now: the one reader of them (D328
// §4).
func unavailableOfNode(ctx context.Context, queries *sqlcgen.Queries, nodeID int64) ([]Unavailable, error) {
	places, err := queries.ListUnavailablePlacesOfNode(ctx, nodeID)
	if err != nil {
		return nil, fmt.Errorf("read node %d's unavailable places: %w", nodeID, err)
	}
	candidates := make([]Unavailable, 0, len(places))
	for _, place := range places {
		candidates = append(candidates, Unavailable{
			Path: place.Path, Since: time.UnixMilli(place.Since), IsMount: place.Path != place.RootPath,
		})
	}
	return candidates, nil
}

// deepestUnder is the deepest of candidates that reportedPath is at or under, and whether there is
// one.
func deepestUnder(candidates []Unavailable, reportedPath []byte) (Unavailable, bool) {
	var found Unavailable
	isFound := false
	for _, candidate := range candidates {
		if isAtOrUnder(reportedPath, candidate.Path) && (!isFound || len(candidate.Path) > len(found.Path)) {
			found, isFound = candidate, true
		}
	}
	return found, isFound
}

// MarkPlaceUnavailable makes a place, a root or a disk mounted inside rootPath, unavailable for cause
// from now, unless it already was (D328): what is under it is not there to read until it comes back.
// It returns since when.
func (ingester *Ingester) MarkPlaceUnavailable(
	ctx context.Context, nodeID int64, path, rootPath, cause string,
) (time.Time, error) {
	now := ingester.clock.Now()
	var since int64
	err := ingester.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		var markErr error
		place := placeUnavailable{nodeID: nodeID, path: path, rootPath: rootPath, cause: cause}
		since, markErr = markPlaceUnavailable(ctx, queries, place, now.UnixMilli())
		return markErr
	})
	if err != nil {
		return time.Time{}, err
	}
	return time.UnixMilli(since), nil
}

// BringBackRoot makes an unavailable root available again before a scan reads it, when its disk
// answers with the facts of the root's last complete scan, and says whether it did: the work parked
// on it resumes as a complete scan's finding it would make it (D208, D233 §10). A share back took a
// minute to bring a film back, behind the listing of every root on its disk (staging, 2026-10-21).
func (ingester *Ingester) BringBackRoot(
	ctx context.Context, nodeID int64, rootPath string, device uint64, isMountPoint bool,
) (bool, error) {
	var isBack bool
	err := ingester.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		var bringErr error
		isBack, bringErr = bringBackPlace(ctx, queries, nodeID, rootPath, device, isMountPoint)
		return bringErr
	})
	if err != nil || !isBack {
		return false, err
	}
	if ingester.onPathsFound != nil {
		ingester.onPathsFound(ctx, nodeID, []string{rootPath})
	}
	return true, nil
}
