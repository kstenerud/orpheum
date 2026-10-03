// SPDX-License-Identifier: GPL-3.0-or-later

package library

import (
	"context"
	"time"

	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// UnreadableReason is why a file cannot be read now (D241 §3).
type UnreadableReason string

// The reasons a file cannot be read.
const (
	// NodeAway is a file whose node is not connected.
	NodeAway UnreadableReason = "node_away"
	// RootMissing is a file under a root its node cannot read, or no longer names (D241 §2).
	RootMissing UnreadableReason = "root_missing"
	// DiskMissing is a file on a disk missing from a mount point inside its root (D208).
	DiskMissing UnreadableReason = "disk_missing"
)

// Unreadable is why a node's file cannot be read now: its reason, the root or mount that is
// missing, and since when; Since is zero for a node not heard from since the core started.
type Unreadable struct {
	Reason UnreadableReason
	NodeID int64
	// Path is the missing root's or mount's path on the node; empty for a node away.
	Path  string
	Since time.Time
}

// NodeConnections is whether a node is connected, and when it was last heard: nodes.Service.
type NodeConnections interface {
	NodeConnection(ctx context.Context, nodeID int64) (nodes.Connection, time.Time)
}

// UnreadableFile is why the file a node reported at reportedPath cannot be read now, or false when
// it can: its node away, or its root or a disk inside it missing (D241 §3). It is the one answer
// to whether a file can be read, which a play asks before anything is queued (D241 §4).
func UnreadableFile(
	ctx context.Context, queries *sqlcgen.Queries, connections NodeConnections, nodeID int64, reportedPath []byte,
) (Unreadable, bool, error) {
	if connection, lastHeard := connections.NodeConnection(ctx, nodeID); connection != nodes.Connected {
		return Unreadable{Reason: NodeAway, NodeID: nodeID, Since: lastHeard}, true, nil
	}
	missing, isMissing, err := UnavailableUnder(ctx, queries, nodeID, reportedPath)
	if err != nil || !isMissing {
		return Unreadable{}, false, err
	}
	reason := RootMissing
	if missing.IsMount {
		reason = DiskMissing
	}
	return Unreadable{Reason: reason, NodeID: nodeID, Path: missing.Path, Since: missing.Since}, true, nil
}
