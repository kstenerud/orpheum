// SPDX-License-Identifier: AGPL-3.0-or-later

package nodes

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
)

// fasterWorkKeepsTheLastReader is how long after faster work last came for a disk its last free
// reader stays kept for it (D255 §4): a scan's examining, an index read or artwork comes in bursts
// whose pieces are well under a second apart, and the bumps it caused came 50 to 400 ms after a
// reader freed (staging, 2026-10-23); five seconds catches them, and holds a read-through back no
// longer than that once the faster work ends.
const fasterWorkKeepsTheLastReader = 5 * time.Second

// noteFasterWork records that work above the slow tier came for a device of a node's, so slow work
// leaves it the device's last reader for a while (parkForFasterWork).
func (all *Exchanges) noteFasterWork(nodeID int64, device string, incoming tier) {
	if device == "" || incoming == tierSlow {
		return
	}
	all.mutex.Lock()
	defer all.mutex.Unlock()
	all.fasterWorkAt[deviceOfNode(nodeID, device)] = all.clock.Now()
}

// parkForFasterWork parks slow work that would take a device's last reader while faster work came
// for it within fasterWorkKeepsTheLastReader (D255 §4). Taken, it would be bumped by that work's next
// piece moments after it started, its reading thrown away: on staging's first scan, 2,654 times in
// half an hour (2026-10-23). It waits on a destination of its own, not the device's readers, which
// free each time a piece of the faster work ends, and asks again once that work has been quiet. It is
// nil for work above the slow tier, and while the device keeps a reader free after it.
func (all *Exchanges) parkForFasterWork(nodeID int64, device string, incoming tier, onDevice, readers int) error {
	if device == "" || incoming != tierSlow || onDevice+1 < readers {
		return nil
	}
	all.mutex.Lock()
	lastCame, hasCome := all.fasterWorkAt[deviceOfNode(nodeID, device)]
	all.mutex.Unlock()
	keptUntil := lastCame.Add(fasterWorkKeepsTheLastReader)
	if !hasCome || !all.clock.Now().Before(keptUntil) {
		return nil
	}
	return jobs.Park(jobs.Parking{
		Destination: jobs.LastReaderDestination(nodeID, device), Reason: "quicker work on the disk comes first",
		ProbeAt: keptUntil, IsBackPressure: true,
	}, fmt.Errorf("node %d's disk %s keeps its last reader for quicker work: %w", nodeID, device, errKeptForFasterWork))
}

// errKeptForFasterWork is slow work that leaves a disk's last reader to faster work.
var errKeptForFasterWork = errors.New("its last reader is kept for quicker work")

// deviceOfNode names a device of a node's, as the exchanges keep it.
func deviceOfNode(nodeID int64, device string) string {
	return strconv.FormatInt(nodeID, 10) + ":" + device
}
