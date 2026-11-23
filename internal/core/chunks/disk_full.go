// SPDX-License-Identifier: AGPL-3.0-or-later

package chunks

import (
	"errors"

	"github.com/kstenerud/orpheum/internal/core/jobs"
)

// ErrDiskFull is a chunk of a node whose prepared directory's disk has no room: the node refused a
// run for it, and its runs wait parked until its status shows room (D231 §5). A player is told at
// once rather than asked to wait, which could be days (the owner, 2026-11-23, D543); its next ask
// after the node reports room starts the run again.
var ErrDiskFull = errors.New("its node's disk is too full to prepare it")

// isDiskFull is whether the node's prepared directory is parked for want of room: a run it refused
// for its disk, not yet unparked by a status showing room (nodes.keepStatus).
func (service *Service) isDiskFull(nodeID int64) bool {
	_, isParked := service.destinations.DestinationState(jobs.PreparedDirectoryDestination(nodeID))
	return isParked
}
