// SPDX-License-Identifier: AGPL-3.0-or-later

package nodes

import (
	"context"
	"slices"
	"strings"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// NodeSelfUse is what a connected node last said it uses of its machine, with its FFmpeg runs.
type NodeSelfUse struct {
	NodeID int64
	Name   string
	// SentAt is when the node read it, by its own clock, in Unix milliseconds: the time between two
	// of its readings is measured on the clock that read them.
	SentAt int64
	Use    nodeproto.SelfUse
}

// SelfUses are what each connected node last said it uses of its machine, by name; a node whose
// status said nothing of it, or that is not connected, has none.
func (service *Service) SelfUses(ctx context.Context) []NodeSelfUse {
	silentAfter := service.silentAfter(ctx)
	service.mutex.Lock()
	defer service.mutex.Unlock()
	var uses []NodeSelfUse
	for id, node := range service.nodesHeardSinceStart {
		if node.status == nil || node.status.SelfUse == nil || !service.isConnectedLocked(node, silentAfter) {
			continue
		}
		uses = append(uses, NodeSelfUse{
			NodeID: id, Name: node.name, SentAt: node.status.SentAt, Use: *node.status.SelfUse,
		})
	}
	slices.SortFunc(uses, func(a, b NodeSelfUse) int { return strings.Compare(a.Name, b.Name) })
	return uses
}

// RunningKinds are, by node, how many of the attempts given to it of each kind of job its node last
// said run: not waiting for a slot, and not paused.
func (all *Exchanges) RunningKinds() map[int64]map[jobs.Kind]int {
	all.mutex.Lock()
	defer all.mutex.Unlock()
	running := map[int64]map[jobs.Kind]int{}
	for _, held := range all.attempts {
		if held.lastWord != nodeproto.ProgressRunning {
			continue
		}
		if running[held.NodeID] == nil {
			running[held.NodeID] = map[jobs.Kind]int{}
		}
		running[held.NodeID][held.Kind]++
	}
	return running
}
