// SPDX-License-Identifier: GPL-3.0-or-later

package library

import (
	"context"
	"encoding/json"
	"fmt"
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
	return NewReadability(queries, connections).File(ctx, nodeID, reportedPath)
}

// Readability answers UnreadableFile for many files, reading each node's connection and missing
// roots and disks once: a list's titles.
type Readability struct {
	queries     *sqlcgen.Queries
	connections NodeConnections
	nodes       map[int64]nodeReadability
}

// nodeReadability is what one node's files are judged by.
type nodeReadability struct {
	away    *Unreadable
	missing []Unavailable
}

// NewReadability is a Readability over queries, its nodes' connections asked of connections.
func NewReadability(queries *sqlcgen.Queries, connections NodeConnections) *Readability {
	return &Readability{queries: queries, connections: connections, nodes: map[int64]nodeReadability{}}
}

// File is why the file a node reported at reportedPath cannot be read now, or false when it can.
func (readability *Readability) File(
	ctx context.Context, nodeID int64, reportedPath []byte,
) (Unreadable, bool, error) {
	node, err := readability.node(ctx, nodeID)
	if err != nil {
		return Unreadable{}, false, err
	}
	if node.away != nil {
		return *node.away, true, nil
	}
	missing, isMissing := deepestUnder(node.missing, reportedPath)
	if !isMissing {
		return Unreadable{}, false, nil
	}
	reason := RootMissing
	if missing.IsMount {
		reason = DiskMissing
	}
	return Unreadable{Reason: reason, NodeID: nodeID, Path: missing.Path, Since: missing.Since}, true, nil
}

// node is what a node's files are judged by, read once.
func (readability *Readability) node(ctx context.Context, nodeID int64) (nodeReadability, error) {
	if node, isRead := readability.nodes[nodeID]; isRead {
		return node, nil
	}
	var node nodeReadability
	if connection, lastHeard := readability.connections.NodeConnection(ctx, nodeID); connection != nodes.Connected {
		node.away = &Unreadable{Reason: NodeAway, NodeID: nodeID, Since: lastHeard}
	} else {
		missing, err := unavailableOfNode(ctx, readability.queries, nodeID)
		if err != nil {
			return nodeReadability{}, fmt.Errorf("read what is missing on node %d: %w", nodeID, err)
		}
		node.missing = missing
	}
	readability.nodes[nodeID] = node
	return node, nil
}

// finishPage gives a page's entries their posters, and says of each whether its files can be read
// now: a series by its episodes', every other kind by its own.
func (browser *Browser) finishPage(ctx context.Context, kind string, page Page) (Page, error) {
	page, err := browser.withPosters(ctx, page)
	if err != nil {
		return Page{}, err
	}
	ids := make([]int64, 0, len(page.Entries))
	for _, entry := range page.Entries {
		ids = append(ids, entry.ID)
	}
	unreadable, err := browser.unreadableItems(ctx, ids, kind == ListSeries)
	if err != nil {
		return Page{}, err
	}
	for index := range page.Entries {
		if why, isUnreadable := unreadable[page.Entries[index].ID]; isUnreadable {
			page.Entries[index].Unreadable = &why
		}
	}
	return page, nil
}

// unreadableItems is, for each of itemIDs with live files none of which can be read now, why its
// first cannot (D241 §3): titles by their own files, series by their episodes'. An item with a file
// that can be read, or with none, is not in it.
func (browser *Browser) unreadableItems(ctx context.Context, itemIDs []int64, isSeries bool) (
	map[int64]Unreadable, error,
) {
	unreadable := map[int64]Unreadable{}
	if browser.connections == nil || len(itemIDs) == 0 {
		return unreadable, nil
	}
	encoded, err := json.Marshal(itemIDs)
	if err != nil {
		return nil, fmt.Errorf("encode %d item ids: %w", len(itemIDs), err)
	}
	queries := browser.database.Read()
	var locations []sqlcgen.ListLiveFileLocationsOfTitlesRow
	if isSeries {
		rows, listErr := queries.ListLiveFileLocationsOfSeries(ctx, string(encoded))
		err = listErr
		for _, row := range rows {
			locations = append(locations, sqlcgen.ListLiveFileLocationsOfTitlesRow(row))
		}
	} else {
		locations, err = queries.ListLiveFileLocationsOfTitles(ctx, string(encoded))
	}
	if err != nil {
		return nil, fmt.Errorf("read where %d items' files are: %w", len(itemIDs), err)
	}
	readability := NewReadability(queries, browser.connections)
	readable := map[int64]bool{}
	for _, location := range locations {
		if readable[location.ItemID] {
			continue
		}
		why, isUnreadable, fileErr := readability.File(ctx, location.NodeID, location.ReportedPathRaw)
		if fileErr != nil {
			return nil, fileErr
		}
		if !isUnreadable {
			readable[location.ItemID] = true
			delete(unreadable, location.ItemID)
			continue
		}
		if _, isNoted := unreadable[location.ItemID]; !isNoted {
			unreadable[location.ItemID] = why
		}
	}
	return unreadable, nil
}
