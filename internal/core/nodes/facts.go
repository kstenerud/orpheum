// SPDX-License-Identifier: GPL-3.0-or-later

package nodes

import (
	"context"
	"slices"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// PrepareSlotsSink is told, after each hello, how many prepare slots the nodes that said hello
// since the core started have in all: the core runs as many prepares at once (cut-a §5.3, as
// plan-deviations P36 records).
type PrepareSlotsSink func(prepareSlotCount int)

// SetPrepareSlotsSink names who hears the nodes' prepare slots. Set it before the channel serves.
func (service *Service) SetPrepareSlotsSink(sink PrepareSlotsSink) {
	service.prepareSlotsSink = sink
}

// prepareSlotCount is the encode and remux slots of every node that said hello since the core
// started, added up.
func (service *Service) prepareSlotCount() int {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	count := 0
	for _, node := range service.connectedNodes {
		count += node.hello.Capacity.EncodeSlotCount + node.hello.Capacity.RemuxSlotCount
	}
	return count
}

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
// its limit again brings back the prepares parked on it for want of room (D231 §5).
func (service *Service) keepStatus(ctx context.Context, nodeID int64, status *nodeproto.Status) {
	service.mutex.Lock()
	if existing, found := service.connectedNodes[nodeID]; found {
		kept := *status
		existing.status = &kept
	}
	service.mutex.Unlock()
	if usage := status.PreparedUsage; usage != nil && usage.DiskUsedPercent <= usage.DiskUsedLimitPercent {
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
