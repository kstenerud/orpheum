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

// EncodesOnAGPU is whether the node encodes on a GPU, Quick Sync or NVENC, as its last hello said; false
// when it encodes in software or has not said hello since the core started (D323 §3).
func (service *Service) EncodesOnAGPU(nodeID int64) bool {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	node, found := service.nodesHeardSinceStart[nodeID]
	if !found {
		return false
	}
	return isGPUEncoder(node.hello.Hardware.ChosenEncoder)
}

// isGPUEncoder is whether a node's chosen encoder is a GPU's: not software's, and named.
func isGPUEncoder(chosen string) bool {
	return chosen != "" && chosen != nodeproto.EncoderX264
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
	var previous *nodeproto.Status
	if existing, found := service.nodesHeardSinceStart[nodeID]; found {
		previous = existing.status
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
	service.markRootsNotAnswering(ctx, nodeID, previous, status)
	service.bringBackRootsAnswering(ctx, nodeID, previous, status)
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

// markRootsNotAnswering makes each library root whose disk a node's status newly says is not
// answering unavailable, as work stalled on it would: with nothing reading it, nothing else noticed,
// and its films were listed as playable until someone pressed play and waited (the owner's test,
// 2026-10-21, nas.example off). A root its previous status said so of already was.
func (service *Service) markRootsNotAnswering(
	ctx context.Context, nodeID int64, previous, status *nodeproto.Status,
) {
	if service.scans == nil {
		return
	}
	var newly []string
	for _, disk := range status.Disks {
		if disk.IsNotAnswering && !isNotAnswering(previous, disk.Path) {
			newly = append(newly, disk.Path)
		}
	}
	if len(newly) == 0 {
		return
	}
	roots, err := service.coveredRoots(ctx, nodeID, newly)
	if err != nil {
		service.logger.WarnContext(ctx, "could not read the libraries' roots", "nodeId", nodeID, "err", err)
		return
	}
	for _, root := range roots {
		if _, _, err := service.scans.MarkStalled(ctx, nodeID, root, root); err != nil {
			service.logger.WarnContext(ctx, "could not make a root whose disk is not answering unavailable",
				"nodeId", nodeID, "rootPath", root, "err", err)
		}
	}
}

// bringBackRootsAnswering brings back each root whose disk its node's previous status said was not
// answering and this one measures: a full scan of every root on the disk stood between a share's
// return and its films, a minute and more (staging, 2026-10-21). A root on another device than its
// last complete scan read stays unavailable until a scan reads it, as the empty directory an
// unmounted disk leaves would.
func (service *Service) bringBackRootsAnswering(
	ctx context.Context, nodeID int64, previous, status *nodeproto.Status,
) {
	if service.scans == nil {
		return
	}
	for _, disk := range status.Disks {
		if disk.Failure != "" || disk.Device == 0 || !isNotAnswering(previous, disk.Path) {
			continue
		}
		if _, err := service.scans.BringBackRoot(ctx, nodeID, disk.Path, disk.Device, disk.IsMountPoint); err != nil {
			service.logger.WarnContext(ctx, "could not bring back a root whose disk answers again",
				"nodeId", nodeID, "rootPath", disk.Path, "err", err)
		}
	}
}

// isNotAnswering is whether status, which may be nil, says the disk at path is not answering.
func isNotAnswering(status *nodeproto.Status, path string) bool {
	return status != nil && slices.ContainsFunc(status.Disks, func(disk nodeproto.Disk) bool {
		return disk.Path == path && disk.IsNotAnswering
	})
}
