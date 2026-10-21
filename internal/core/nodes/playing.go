// SPDX-License-Identifier: AGPL-3.0-or-later

package nodes

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// Playing is not starved (D324 §5): a node that encodes in software and holds a played encode, a
// run of chunks or a prepare a person waits on, is given no other encode; and while its played
// encode is behind playback (HoldBackFor), it is given nothing that is not played. A film played as
// it is, remuxed, or encoded on a GPU leaves the CPU free, and work runs beside it.

// errBesidePlaying is encode work that waits for a node's played encode, in software, to end.
var errBesidePlaying = errors.New("the node encodes a film being played, in software")

// errBehindPlaying is work that waits while a node's played encode catches up with playback.
var errBehindPlaying = errors.New("the film playing there is behind playback")

// parkBesidePlaying parks work not played that would starve a node's played encode: any, while the
// node is held back, waiting on the node's playing destination until the hold ends; and an encode on
// a node encoding in software that holds a played encode, waiting on its encode slot, which the
// played encode's end brings back. It is nil otherwise.
func (all *Exchanges) parkBesidePlaying(nodeID int64, resources Resources, work nodeproto.Work) error {
	now := all.clock.Now()
	all.mutex.Lock()
	heldBackUntil := all.heldBackUntil[nodeID]
	isEncodePlayed := false
	for _, held := range all.attempts {
		if held.NodeID == nodeID && held.Priority >= jobs.PriorityPlayNow && held.Work.Slot() == nodeproto.SlotEncode {
			isEncodePlayed = true
			break
		}
	}
	all.mutex.Unlock()
	if now.Before(heldBackUntil) {
		return jobs.Park(jobs.Parking{
			Destination: jobs.PlayingDestination(nodeID), Reason: "the film playing there catches up",
			ProbeAt: heldBackUntil, IsBackPressure: true,
		}, fmt.Errorf("node %d: %w", nodeID, errBehindPlaying))
	}
	if resources.EncodesInSoftware && isEncodePlayed && work.Slot() == nodeproto.SlotEncode {
		return jobs.Park(jobs.Parking{
			Destination: jobs.SlotDestination(nodeID, string(nodeproto.SlotEncode)), Reason: "busy",
			AwaitsSignal: true, IsBackPressure: true,
		}, fmt.Errorf("node %d: %w", nodeID, errBesidePlaying))
	}
	return nil
}

// HoldBackFor stops the work on nodeID no one plays and gives it none before until (D324 §5): its
// played encode, in software, fell behind playback. Each stopped job goes back to the head of its
// queue at no cost, as a bump's does. It reports how many it stopped.
func (all *Exchanges) HoldBackFor(ctx context.Context, nodeID int64, until time.Time) int {
	all.mutex.Lock()
	if until.After(all.heldBackUntil[nodeID]) {
		all.heldBackUntil[nodeID] = until
	}
	var stopping []Attempt
	for _, held := range all.attempts {
		if held.NodeID != nodeID || held.isBeingBumped || held.Priority >= jobs.PriorityPlayNow {
			continue
		}
		if _, isHandled := all.assignees[held.Kind]; !isHandled {
			continue
		}
		held.isBeingBumped = true
		stopping = append(stopping, held.Attempt)
	}
	all.mutex.Unlock()
	for _, attempt := range stopping {
		all.bump(ctx, attempt, fmt.Errorf("node %d: %w", nodeID, errBehindPlaying))
	}
	return len(stopping)
}
