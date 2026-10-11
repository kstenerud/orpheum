// SPDX-License-Identifier: AGPL-3.0-or-later

package agent

import (
	"slices"
	"sync"

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
