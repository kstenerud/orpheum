// SPDX-License-Identifier: AGPL-3.0-or-later

package nodes

import (
	"context"
	"time"

	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// connectedNode is what the core holds for one connected node.
type connectedNode struct {
	name string
	// hello is its last hello: its roots (D107), confinement (D112) and hardware (D225).
	hello nodeproto.Hello
	// status is its last status, nil before the first: what its prepared directory holds (D217
	// §5) and its disks (D225).
	status         *nodeproto.Status
	commands       []nodeproto.Command
	nextCommandSeq int64
	// deliveredThrough is the last command an answer to a long-poll carried: one up to it may be the
	// node's though it has not acknowledged it yet.
	deliveredThrough int64
	// seqByToken is the last command sent for each attempt, its work or its cancel, until the
	// node's status says it took it: the reconciliation judges no attempt whose command is on its way.
	seqByToken map[string]int64
	// unsentSince is when each attempt given to the node whose command is not queued yet was given:
	// its job is recorded assigned first, which a busy database makes slow, and a status meanwhile
	// cannot have taken it (WillSend).
	unsentSince map[string]time.Time
	wake        chan struct{}
	// areWatchedFoldersChanged is whether the library folders it watches may have changed since a poll
	// last answered it: the next answers at once, naming them (WatchedFoldersChanged).
	areWatchedFoldersChanged bool
	lastHeard                time.Time
	// openPollCount is how many long-polls for its commands are open: while one is, it is
	// connected however long ago it last spoke.
	openPollCount int
}

// Connection is whether a node is connected, as the core sees it (D231 §11).
type Connection string

// A node's connections.
const (
	// Connected is a node with a long-poll for its commands open, or heard within
	// nodeSilentAfterSeconds.
	Connected Connection = "connected"
	// Silent is a node that said hello since the core started and has not been heard since
	// nodeSilentAfterSeconds ago.
	Silent Connection = "silent"
	// AwaitingHello is a node enrolled that has not said hello since the core started.
	AwaitingHello Connection = "awaitingHello"
	// NeedsUpdate is a node whose last hello spoke another version of the node protocol than the
	// core's, so it was refused (D233 §9).
	NeedsUpdate Connection = "needsUpdate"
	// Locked is a node whose kernel lacks confinement its config requires, which reports and waits,
	// running no FFmpeg, until an admin lets it run unconfined (D450 §1).
	Locked Connection = "locked"
)

// NodeConnection is whether a node is connected and when it was last heard; zero when it has
// not said hello since the core started.
func (service *Service) NodeConnection(ctx context.Context, nodeID int64) (Connection, time.Time) {
	silentAfter := service.silentAfter(ctx)
	service.mutex.Lock()
	defer service.mutex.Unlock()
	node, found := service.nodesHeardSinceStart[nodeID]
	_, needsUpdate := service.protocolByNodeID[nodeID]
	locked, isLocked := service.lockedByNodeID[nodeID]
	switch {
	case needsUpdate:
		return NeedsUpdate, time.Time{}
	case isLocked && !found:
		return Locked, locked.lastHeard
	case !found:
		return AwaitingHello, time.Time{}
	case service.isConnectedLocked(node, silentAfter):
		return Connected, node.lastHeard
	}
	return Silent, node.lastHeard
}

// IsConnected is whether a node is connected now: a long-poll for its commands open, or heard
// within nodeSilentAfterSeconds. Work on its files can be sent to it.
func (service *Service) IsConnected(ctx context.Context, nodeID int64) bool {
	connection, _ := service.NodeConnection(ctx, nodeID)
	return connection == Connected
}

// ConnectedNodeID is the connected node heard from most recently, where work that any node can
// do is sent; false when none is connected.
func (service *Service) ConnectedNodeID(ctx context.Context) (int64, bool) {
	silentAfter := service.silentAfter(ctx)
	service.mutex.Lock()
	defer service.mutex.Unlock()
	var latest time.Time
	latestNodeID := int64(0)
	for id, node := range service.nodesHeardSinceStart {
		if service.isConnectedLocked(node, silentAfter) && (latestNodeID == 0 || node.lastHeard.After(latest)) {
			latestNodeID, latest = id, node.lastHeard
		}
	}
	return latestNodeID, latestNodeID != 0
}

// isConnectedLocked is whether node is connected; the caller holds the mutex.
func (service *Service) isConnectedLocked(node *connectedNode, silentAfter time.Duration) bool {
	return node.openPollCount > 0 || service.clock.Now().Sub(node.lastHeard) < silentAfter
}

// tuning is how the core's settings say its nodes behave now (nodeproto.Tuning).
func (service *Service) tuning(ctx context.Context) nodeproto.Tuning {
	progressEvery := settings.DurationOrStarting(ctx, service.settings, settings.NodeProgressEverySeconds,
		service.logger)
	return nodeproto.Tuning{
		StatusEverySeconds:   StatusEveryFor(service.silentAfter(ctx)),
		ProgressEverySeconds: int(progressEvery.Seconds()),
	}
}

// StatusEveryFor is how often a node posts its status when the core shows a node silent after
// silentAfter: three statuses within it (cut-a §4), so one lost or late status does not make it
// silent; at least a second.
func StatusEveryFor(silentAfter time.Duration) int {
	return max(1, int(silentAfter/time.Second)/3)
}

// silentAfter is nodeSilentAfterSeconds, or its starting value when it cannot be read, so a
// node's connection is still judged when the settings cannot be read.
func (service *Service) silentAfter(ctx context.Context) time.Duration {
	silentAfter, err := service.settings.Duration(ctx, settings.NodeSilentAfterSeconds)
	if err != nil {
		silentAfter = settings.StartingDuration(settings.NodeSilentAfterSeconds)
		service.logger.WarnContext(ctx, "could not read nodeSilentAfterSeconds; taking its starting value",
			"startingValue", silentAfter.String(), "err", err)
	}
	return silentAfter
}

// LastHeard is when the named node last said anything, and whether it ever has.
func (service *Service) LastHeard(name string) (time.Time, bool) {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	for _, node := range service.nodesHeardSinceStart {
		if node.name == name {
			return node.lastHeard, true
		}
	}
	return time.Time{}, false
}

// noteHeard notes that a node spoke.
func (service *Service) noteHeard(nodeID int64) {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	if node, found := service.nodesHeardSinceStart[nodeID]; found {
		node.lastHeard = service.clock.Now()
	}
}

// notePoll notes a long-poll for a node's commands opening (+1) or ending (-1): a node that holds
// one open is connected.
func (service *Service) notePoll(nodeID int64, change int) {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	if node, found := service.nodesHeardSinceStart[nodeID]; found {
		node.openPollCount += change
		node.lastHeard = service.clock.Now()
	}
}
