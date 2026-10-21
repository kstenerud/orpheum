// SPDX-License-Identifier: AGPL-3.0-or-later

package agent

import (
	"slices"
	"sync"

	"github.com/kstenerud/orpheum/internal/node/diskwatch"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// stuckReadTool is what the status calls a listing's or an examining's own reading left stuck.
const stuckReadTool = "node"

// stuckReads are the listings' and examinings' readings the node answered as stalled that have not
// ended, by their attempt's token (D233 §10).
type stuckReads struct {
	mutex   sync.Mutex
	byToken map[string]nodeproto.Stuck // guarded by mutex
}

func (stuck *stuckReads) add(token string, reading nodeproto.Stuck) {
	stuck.mutex.Lock()
	defer stuck.mutex.Unlock()
	if stuck.byToken == nil {
		stuck.byToken = map[string]nodeproto.Stuck{}
	}
	stuck.byToken[token] = reading
}

func (stuck *stuckReads) remove(token string) {
	stuck.mutex.Lock()
	defer stuck.mutex.Unlock()
	delete(stuck.byToken, token)
}

// all is every stuck reading, the oldest first.
func (stuck *stuckReads) all() []nodeproto.Stuck {
	stuck.mutex.Lock()
	defer stuck.mutex.Unlock()
	readings := make([]nodeproto.Stuck, 0, len(stuck.byToken))
	for _, reading := range stuck.byToken {
		readings = append(readings, reading)
	}
	slices.SortFunc(readings, func(a, b nodeproto.Stuck) int { return int(a.Since - b.Since) })
	return readings
}

// stuckOfGate are the gate's stuck readings as the status says them, but those listed already, a
// listing's reading of its root.
func stuckOfGate(gated []diskwatch.Stuck, listed []nodeproto.Stuck) []nodeproto.Stuck {
	stuck := make([]nodeproto.Stuck, 0, len(gated))
	for _, each := range gated {
		isListed := slices.ContainsFunc(listed, func(other nodeproto.Stuck) bool {
			return slices.Contains(other.Paths, each.Path)
		})
		if !isListed {
			stuck = append(stuck, nodeproto.Stuck{
				Tool: stuckReadTool, Paths: []string{each.Path},
				Since: each.Since.UnixMilli(),
			})
		}
	}
	return stuck
}
