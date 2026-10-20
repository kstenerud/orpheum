// SPDX-License-Identifier: AGPL-3.0-or-later

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
	if node, found := service.nodesHeardSinceStart[nodeID]; found {
		return node.hello.Capacity.SlowestReadBytesPerSecond
	}
	return 0
}

// byteSocketPath is where a node's last hello said its byte socket is; false before its first.
func (service *Service) byteSocketPath(nodeID int64) (string, bool) {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	if node, found := service.nodesHeardSinceStart[nodeID]; found {
		return node.hello.ByteSocketPath, true
	}
	return "", false
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
	for id, existing := range service.nodesHeardSinceStart {
		if existing.name == name && !existing.lastHeard.IsZero() {
			return Facts{
				NodeID: id, Hello: existing.hello, Status: existing.status, LastHeard: existing.lastHeard,
			}, true
		}
	}
	return Facts{}, false
}

// softwareEncoder is the encoder a node encodes with when it found no GPU's that works.
const softwareEncoder = "x264"

// EncodesOnAGPU is whether the node encodes on a GPU, Quick Sync or NVENC, as its last hello said; false
// when it encodes in software or has not said hello since the core started (D323 §3).
func (service *Service) EncodesOnAGPU(nodeID int64) bool {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	node, found := service.nodesHeardSinceStart[nodeID]
	if !found {
		return false
	}
	chosen := node.hello.Hardware.ChosenEncoder
	return chosen != "" && chosen != softwareEncoder
}

// RootWatches are whether a node watches each of its roots for changes, by root, as its last status
// said; none when it has said none since the core started (D272 §9).
func (service *Service) RootWatches(nodeID int64) map[string]nodeproto.RootWatch {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	node, found := service.nodesHeardSinceStart[nodeID]
	if !found || node.status == nil {
		return nil
	}
	watches := make(map[string]nodeproto.RootWatch, len(node.status.Watches))
	for _, watch := range node.status.Watches {
		watches[watch.RootPath] = watch
	}
	return watches
}

// keepStatus keeps what a node's status says about it. A prepared directory whose disk is within
// its limit, and keeps its floor of free space, again brings back the prepares parked on it for want
// of room (D231 §5).
func (service *Service) keepStatus(ctx context.Context, nodeID int64, status *nodeproto.Status) {
	service.mutex.Lock()
	onTheirWay := map[string]bool{}
	if existing, found := service.nodesHeardSinceStart[nodeID]; found {
		kept := *status
		existing.status = &kept
		for token, seq := range existing.seqByToken {
			if seq > status.TakenCommandSeq {
				onTheirWay[token] = true
			} else {
				delete(existing.seqByToken, token)
			}
		}
		now := service.clock.Now()
		for token, since := range existing.unsentSince {
			if now.Sub(since) < unsentGrace {
				onTheirWay[token] = true
			} else {
				delete(existing.unsentSince, token)
			}
		}
	}
	service.mutex.Unlock()
	if service.resultSink != nil {
		service.resultSink.Reconcile(ctx, nodeID, status.Held, onTheirWay)
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
	for id, node := range service.nodesHeardSinceStart {
		if node.name == name && service.isConnectedLocked(node, silentAfter) {
			return id, slices.Clone(node.hello.RootPaths), true
		}
	}
	return 0, nil, false
}
