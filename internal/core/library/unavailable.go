// SPDX-License-Identifier: GPL-3.0-or-later

package library

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store"
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
	candidates, err := unavailableOfNode(ctx, queries, nodeID)
	if err != nil {
		return Unavailable{}, false, err
	}
	found, isFound := deepestUnder(candidates, reportedPath)
	return found, isFound, nil
}

// unavailableOfNode is every root and mount of a node that is missing.
func unavailableOfNode(ctx context.Context, queries *sqlcgen.Queries, nodeID int64) ([]Unavailable, error) {
	roots, err := queries.ListUnavailableRootsOfNode(ctx, nodeID)
	if err != nil {
		return nil, fmt.Errorf("read node %d's unavailable roots: %w", nodeID, err)
	}
	mounts, err := queries.ListMissingMountsOfNode(ctx, nodeID)
	if err != nil {
		return nil, fmt.Errorf("read node %d's missing mounts: %w", nodeID, err)
	}
	candidates := make([]Unavailable, 0, len(roots)+len(mounts))
	for _, root := range roots {
		candidates = append(candidates, Unavailable{Path: root.RootPath, Since: time.UnixMilli(root.UnavailableSince)})
	}
	for _, mount := range mounts {
		candidates = append(candidates, Unavailable{
			Path: mount.MountPath, Since: time.UnixMilli(ptr.ValueOrZero(mount.LastScanAt)), IsMount: true,
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

// MarkRootUnavailable makes a root unavailable from now, unless it already was, because work
// stalled waiting on its disk (D233 §10): what is under it is not there to read until a scan reads
// it again (RecordRootComplete). It returns since when; a root no scan has read has no state, and is
// unavailable from now without being kept so.
func (ingester *Ingester) MarkRootUnavailable(ctx context.Context, nodeID int64, rootPath string) (time.Time, error) {
	now := ingester.clock.Now()
	var since int64
	err := ingester.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		var markErr error
		since, markErr = store.OrNotFound(queries.MarkRootUnavailable(ctx, sqlcgen.MarkRootUnavailableParams{
			Now: new(now.UnixMilli()), NodeID: nodeID, RootPath: rootPath,
		}))
		if errors.Is(markErr, store.ErrNotFound) {
			since, markErr = now.UnixMilli(), nil
		}
		return markErr
	})
	if err != nil {
		return time.Time{}, fmt.Errorf("make root %s on node %d unavailable: %w", rootPath, nodeID, err)
	}
	return time.UnixMilli(since), nil
}
