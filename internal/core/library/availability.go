// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"context"
	"errors"
	"fmt"

	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// Why a place cannot be read now (D328 §2), as unavailable_place keeps it.
const (
	causeNotAnswering = "not_answering" // its disk did not answer: work stalled, or its status or probe said so
	causeUnread       = "unread"        // a scan could not read the root, or its held-back scan saw none of it
	causeMissing      = "missing"       // a scan did not find the disk on its mount point inside a root (D208)
	causeNotNamed     = "not_named"     // its node's hello no longer names the root (D241 §2)
)

// The one writer of a place's availability (D328 §3): every change of it, from any cause, is one of
// these, in the caller's transaction. A place that comes back is handed to the ingester's
// onPathsFound by its caller once that commits, which unparks the work waiting on it.

// placeUnavailable is a place found unavailable, and why.
type placeUnavailable struct {
	nodeID         int64
	path, rootPath string
	cause          string
}

// markPlaceUnavailable makes a place unavailable for its cause from now, unless it already was, and
// returns since when, in milliseconds.
func markPlaceUnavailable(
	ctx context.Context, queries *sqlcgen.Queries, place placeUnavailable, nowMs int64,
) (int64, error) {
	err := queries.MarkPlaceUnavailable(ctx, sqlcgen.MarkPlaceUnavailableParams{
		NodeID: place.nodeID, Path: place.path, RootPath: place.rootPath, Now: nowMs, Cause: place.cause,
	})
	if err != nil {
		return 0, fmt.Errorf("make %s on node %d unavailable: %w", place.path, place.nodeID, err)
	}
	since, err := queries.GetUnavailablePlaceSince(ctx, sqlcgen.GetUnavailablePlaceSinceParams{
		NodeID: place.nodeID, Path: place.path,
	})
	if err != nil {
		return 0, fmt.Errorf("read since when %s on node %d is unavailable: %w", place.path, place.nodeID, err)
	}
	return since, nil
}

// clearPlace makes a place a scan read, or found its disk where it was, available.
func clearPlace(ctx context.Context, queries *sqlcgen.Queries, nodeID int64, path string) error {
	_, err := queries.ClearUnavailablePlace(ctx, sqlcgen.ClearUnavailablePlaceParams{NodeID: nodeID, Path: path})
	if err != nil {
		return fmt.Errorf("make %s on node %d available: %w", path, nodeID, err)
	}
	return nil
}

// bringBackPlace makes a place available when its disk answers with the facts its last complete scan
// recorded, and says whether it did (D326, D328 §6).
func bringBackPlace(
	ctx context.Context, queries *sqlcgen.Queries, nodeID int64, path string, device uint64, isMountPoint bool,
) (bool, error) {
	broughtBack, err := queries.BringBackPlace(ctx, sqlcgen.BringBackPlaceParams{
		NodeID: nodeID, Path: path,
		FilesystemDevice: new(int64(device)), //nolint:gosec // a device id fits
		IsMountPoint:     new(boolInt(isMountPoint)),
	})
	if err != nil {
		return false, fmt.Errorf("bring back %s on node %d: %w", path, nodeID, err)
	}
	return broughtBack > 0, nil
}

// forgetRootPlaces forgets a retired or repointed root's places at its old place.
func forgetRootPlaces(ctx context.Context, queries *sqlcgen.Queries, nodeID int64, rootPath string) error {
	err := queries.ForgetRootPlaces(ctx, sqlcgen.ForgetRootPlacesParams{NodeID: nodeID, RootPath: rootPath})
	if err != nil {
		return fmt.Errorf("forget the places of %s on node %d: %w", rootPath, nodeID, err)
	}
	return nil
}

// isPlaceUnavailable is whether a place cannot be read now.
func isPlaceUnavailable(ctx context.Context, queries *sqlcgen.Queries, nodeID int64, path string) (bool, error) {
	_, err := store.OrNotFound(queries.GetUnavailablePlaceSince(ctx, sqlcgen.GetUnavailablePlaceSinceParams{
		NodeID: nodeID, Path: path,
	}))
	if errors.Is(err, store.ErrNotFound) {
		return false, nil
	}
	return err == nil, err
}
