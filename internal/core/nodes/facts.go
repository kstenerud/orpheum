// SPDX-License-Identifier: GPL-3.0-or-later

package nodes

import (
	"context"
	"slices"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// SlowestReadBytesPerSecond is the slowest the node reads a whole file, as its last hello said;
// zero when it has not said hello since the core started.
func (service *Service) SlowestReadBytesPerSecond(nodeID int64) int64 {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	if node, found := service.connectedNodes[nodeID]; found {
		return node.hello.Capacity.SlowestReadBytesPerSecond
	}
	return 0
}

// Facts are what a node last said about itself since the core started: its hello, its last
// status (nil before the first) and when it was last heard, with its id (D225).
type Facts struct {
	NodeID    int64
	Hello     nodeproto.Hello
	Status    *nodeproto.Status
	LastHeard time.Time
}

// Facts is what the named node last said about itself; false when it has not said hello since
// the core started.
func (service *Service) Facts(name string) (Facts, bool) {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	for id, existing := range service.connectedNodes {
		if existing.name == name && !existing.lastHeard.IsZero() {
			return Facts{
				NodeID: id, Hello: existing.hello, Status: existing.status, LastHeard: existing.lastHeard,
			}, true
		}
	}
	return Facts{}, false
}

// keepStatus keeps what a node's status says about it. A prepared directory whose disk is within
// its limit, and keeps its floor of free space, again brings back the prepares parked on it for want
// of room (D231 §5).
func (service *Service) keepStatus(ctx context.Context, nodeID int64, status *nodeproto.Status) {
	service.mutex.Lock()
	onTheirWay := map[string]bool{}
	if existing, found := service.connectedNodes[nodeID]; found {
		kept := *status
		existing.status = &kept
		for token, seq := range existing.seqByToken {
			if seq > status.TakenCommandSeq {
				onTheirWay[token] = true
			} else {
				delete(existing.seqByToken, token)
			}
		}
	}
	service.mutex.Unlock()
	if service.resultSink != nil {
		service.resultSink.Square(ctx, nodeID, status.Held, onTheirWay)
	}
	if usage := status.PreparedUsage; usage != nil && usage.DiskUsedPercent <= usage.DiskUsedLimitPercent &&
		usage.DiskAvailableBytes >= usage.DiskMinFreeBytes {
		service.destinations.Unpark(ctx, jobs.PreparedDirectoryDestination(nodeID))
	}
}

// ConnectedNodeRoots are the media roots the named node configured, as its last hello said, with
// its id, while it is connected; false when it is not (D107: roots are configured on the node,
// never remotely).
func (service *Service) ConnectedNodeRoots(ctx context.Context, name string) (int64, []string, bool) {
	silentAfter := service.silentAfter(ctx)
	service.mutex.Lock()
	defer service.mutex.Unlock()
	for id, node := range service.connectedNodes {
		if node.name == name && service.isConnectedLocked(node, silentAfter) {
			return id, slices.Clone(node.hello.RootPaths), true
		}
	}
	return 0, nil, false
}
