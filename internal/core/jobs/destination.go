// SPDX-License-Identifier: GPL-3.0-or-later

package jobs

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/jobqueue"
)

// Destination is what a job needs that can be down (D231 §5): an outside host, a node, a root or
// mount a scan found missing, a node's prepared directory, or the core's own storage. Its name is
// kept beside the job's target, and a job parked on it waits, spending nothing (D231 §6).
type Destination = jobqueue.Destination

// The outside hosts and the core's storage. An outside host is probed on a back-off; the rest
// come back when something says so.
const (
	// DestinationTMDBAPI is TMDB's API. A missing or refused key parks it until the key changes.
	DestinationTMDBAPI Destination = "tmdb-api"
	// DestinationTMDBImages is TMDB's image server, which needs no key.
	DestinationTMDBImages Destination = "tmdb-images"
	// DestinationOpenSubtitles is OpenSubtitles.
	DestinationOpenSubtitles Destination = "opensubtitles"
	// DestinationCoreStorage is the core's own disk: cache.db and library.db, which fail for
	// want of room or with an I/O error.
	DestinationCoreStorage Destination = "core-storage"
	// DestinationAnyNode is work any node can do, such as re-encoding a downloaded image, while no
	// node is connected: any node's hello brings it back.
	DestinationAnyNode Destination = "any-node"
)

// The prefixes of the destinations named by a node.
const (
	nodePrefix     = "node:"
	rootPrefix     = "root:"
	preparedPrefix = "prepared:"
)

// NodeDestination is a node: down while it is not connected or silent; its hello brings it back.
func NodeDestination(nodeID int64) Destination {
	return Destination(nodePrefix + strconv.FormatInt(nodeID, 10))
}

// RootDestination is a root, or a disk mounted inside one, that a scan found missing on a node:
// the next scan that finds it brings it back.
func RootDestination(nodeID int64, path string) Destination {
	return Destination(rootPrefix + strconv.FormatInt(nodeID, 10) + ":" + path)
}

// PreparedDirectoryDestination is a node's prepared directory, where its prepares write: down
// while the node answers that its disk has no room, until its status shows room.
func PreparedDirectoryDestination(nodeID int64) Destination {
	return Destination(preparedPrefix + strconv.FormatInt(nodeID, 10))
}

// DestinationParts are what a destination's name says: which outside host or node it is.
type DestinationParts struct {
	// NodeID is the node a node's, a root's or a prepared directory's destination is on; 0 for
	// an outside host or the core's storage.
	NodeID int64
	// Path is a root's or a mount's path on its node.
	Path string
	// IsNode, IsRoot and IsPreparedDirectory say which of a node's destinations it is.
	IsNode              bool
	IsRoot              bool
	IsPreparedDirectory bool
}

// PartsOf reads a destination's name.
func PartsOf(destination Destination) DestinationParts {
	name := string(destination)
	switch {
	case strings.HasPrefix(name, nodePrefix):
		id, _ := strconv.ParseInt(strings.TrimPrefix(name, nodePrefix), 10, 64) //nolint:errcheck // 0 if not
		return DestinationParts{NodeID: id, IsNode: true}
	case strings.HasPrefix(name, preparedPrefix):
		id, _ := strconv.ParseInt(strings.TrimPrefix(name, preparedPrefix), 10, 64) //nolint:errcheck // 0 if not
		return DestinationParts{NodeID: id, IsPreparedDirectory: true}
	case strings.HasPrefix(name, rootPrefix):
		idText, path, _ := strings.Cut(strings.TrimPrefix(name, rootPrefix), ":")
		id, _ := strconv.ParseInt(idText, 10, 64) //nolint:errcheck // 0 if not
		return DestinationParts{NodeID: id, Path: path, IsRoot: true}
	}
	return DestinationParts{}
}

// DescribeDestination names a destination as a person reads it, with nodeName naming a node by
// its id: "TMDB", "node home", "/media/films on node home".
func DescribeDestination(destination Destination, nodeName func(nodeID int64) string) string {
	switch destination {
	case DestinationTMDBAPI:
		return "TMDB"
	case DestinationTMDBImages:
		return "TMDB's image server"
	case DestinationOpenSubtitles:
		return "OpenSubtitles"
	case DestinationCoreStorage:
		return "the server's own disk"
	case DestinationAnyNode:
		return "a node"
	}
	parts := PartsOf(destination)
	switch {
	case parts.IsNode:
		return "node " + nodeName(parts.NodeID)
	case parts.IsRoot:
		return parts.Path + " on node " + nodeName(parts.NodeID)
	case parts.IsPreparedDirectory:
		return "the prepared directory on node " + nodeName(parts.NodeID)
	}
	return string(destination)
}

// Parking is a destination found down (jobqueue.Parking).
type Parking = jobqueue.Parking

// DestinationState is a parked destination's state (jobqueue.DestinationState).
type DestinationState = jobqueue.DestinationState

// Park answers that a job's destination is down: it waits for it at no cost of an attempt
// (D231 §5, §6).
func Park(parking Parking, err error) error { return jobqueue.Park(parking, err) }

// ParkOnNode answers that a job's node is not connected: it waits for the node's hello, which
// brings the node back (D231 §11). The node went away at lastHeard, or at an unknown time when
// it is zero.
func ParkOnNode(nodeID int64, lastHeard time.Time, err error) error {
	return Park(Parking{
		Destination: NodeDestination(nodeID), Reason: "not connected", Since: lastHeard, AwaitsSignal: true,
	}, fmt.Errorf("node %d is not connected: %w", nodeID, err))
}

// ParkOnMissingRoot answers that a job reads a file under a root, or a disk mounted inside one,
// that the last scan found missing (since then): it waits for a scan that finds it (D231 §5).
func ParkOnMissingRoot(nodeID int64, path string, since time.Time, err error) error {
	return Park(Parking{
		Destination: RootDestination(nodeID, path), Reason: "missing", Since: since, AwaitsSignal: true,
	}, fmt.Errorf("%s on node %d is missing: %w", path, nodeID, err))
}

// Park marks a destination down, for a caller outside a job that found it so.
func (queue *Queue) Park(ctx context.Context, parking Parking) { queue.queue.Park(ctx, parking) }

// Unpark marks a destination up, and its work resumes, highest priority first.
func (queue *Queue) Unpark(ctx context.Context, destination Destination) {
	queue.queue.Unpark(ctx, destination)
}

// ProbeNow looks at a parked outside host at once, for a person waiting on its work.
func (queue *Queue) ProbeNow(destination Destination) { queue.queue.ProbeNow(destination) }

// DestinationState is a destination's state, and whether it is parked.
func (queue *Queue) DestinationState(destination Destination) (DestinationState, bool) {
	return queue.queue.DestinationState(destination)
}

// ParkedDestinations are every parked destination, by name.
func (queue *Queue) ParkedDestinations() []DestinationState { return queue.queue.ParkedDestinations() }
