// SPDX-License-Identifier: GPL-3.0-or-later

package library

import (
	"context"
	"fmt"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// Unavailable is a root, or a disk mounted inside one, that the last scan found missing: the
// files under it are not there to read, and work that reads them waits for a scan that finds it
// (D208, D231 §5).
type Unavailable struct {
	// Path is the root's or the mount's path on its node.
	Path string
	// Since is when the scan that found it missing ran.
	Since time.Time
}

// UnavailableUnder is the missing root or mount that holds a file on a node, by the path its node
// reported: the deepest, since a mount inside a root is what went; false when all are there.
func UnavailableUnder(
	ctx context.Context, queries *sqlcgen.Queries, nodeID int64, reportedPath []byte,
) (Unavailable, bool, error) {
	roots, err := queries.ListUnavailableRootsOfNode(ctx, nodeID)
	if err != nil {
		return Unavailable{}, false, fmt.Errorf("read node %d's unavailable roots: %w", nodeID, err)
	}
	mounts, err := queries.ListMissingMountsOfNode(ctx, nodeID)
	if err != nil {
		return Unavailable{}, false, fmt.Errorf("read node %d's missing mounts: %w", nodeID, err)
	}
	candidates := make([]Unavailable, 0, len(roots)+len(mounts))
	for _, root := range roots {
		candidates = append(candidates, Unavailable{Path: root.RootPath, Since: time.UnixMilli(root.LastScanAt)})
	}
	for _, mount := range mounts {
		candidates = append(candidates, Unavailable{
			Path: mount.MountPath, Since: time.UnixMilli(ptr.ValueOrZero(mount.LastScanAt)),
		})
	}
	var found Unavailable
	isFound := false
	for _, candidate := range candidates {
		if isAtOrUnder(reportedPath, candidate.Path) && (!isFound || len(candidate.Path) > len(found.Path)) {
			found, isFound = candidate, true
		}
	}
	return found, isFound, nil
}
