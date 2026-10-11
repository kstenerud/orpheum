// SPDX-License-Identifier: AGPL-3.0-or-later

package nodes

import (
	"context"
	"fmt"
	"time"

	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// Reconcile reconciles the jobs given to a node with what its status says it holds (D234 §6), judging no
// attempt whose command, onTheirWay, the node has not taken yet. An attempt it holds is heard of, its
// job adopted (S82). One it does not hold, waiting, running or with its result on its way, is lost: one
// given to another start of the node was interrupted at its hello (Interrupt), so this is a broken
// invariant, logged and counted, and its job resolved as the tool's failure, tried again. One
// it holds that the core did not give it, or no longer wants, is a broken invariant too, and the
// node is told to drop it (S91, G4); one whose result is on its way is left to it. One the node says
// has not moved for MovementWindow is stopped, as a stall (ErrNotMoving): the core holds work only
// while it moves, waiting work moving as the work ahead of it does (D233 §5).
func (all *Exchanges) Reconcile(
	ctx context.Context, nodeID int64, held []nodeproto.HeldAttempt, onTheirWay map[string]bool,
) {
	heldByToken := make(map[string]nodeproto.HeldAttempt, len(held))
	for _, each := range held {
		heldByToken[each.AttemptToken] = each
	}
	now := all.clock.Now()
	all.mutex.Lock()
	var unknown []nodeproto.HeldAttempt
	for _, each := range held {
		if _, isGiven := all.attempts[each.AttemptToken]; !isGiven && each.State != nodeproto.HeldEnded &&
			!onTheirWay[each.AttemptToken] {
			unknown = append(unknown, each)
		}
	}
	for token, attempt := range all.attempts {
		if each, isHeld := heldByToken[token]; isHeld && attempt.NodeID == nodeID {
			attempt.lastHeard = now
			if word, isWord := wordOfHeld(each.State); isWord {
				attempt.lastWord = word
			}
		}
	}
	all.mutex.Unlock()
	all.unanswered(ctx, func(attempt heldAttempt) bool {
		_, isHeld := heldByToken[attempt.Token]
		return attempt.NodeID == nodeID && !isHeld && !onTheirWay[attempt.Token]
	}, func(attempt Attempt) error {
		all.brokeAnInvariant(ctx, nodeID, "a node does not hold work given to it", attempt.JobID, attempt.Token)
		return fmt.Errorf("node %d does not hold %s job %d: %w", nodeID, attempt.Kind, attempt.JobID, ErrWorkLost)
	}, false)
	all.stopNotMoving(ctx, nodeID, heldByToken)
	for _, each := range unknown {
		all.brokeAnInvariant(ctx, nodeID, "a node holds work no job was given, or no job wants", each.JobID,
			each.AttemptToken)
		all.sender.Enqueue(nodeID, CancelCommand(each.JobID, each.AttemptToken))
	}
}

// stopNotMoving stops each attempt the node holds that it says has not moved for MovementWindow,
// and resolves it as not moving (ErrNotMoving).
func (all *Exchanges) stopNotMoving(ctx context.Context, nodeID int64, heldByToken map[string]nodeproto.HeldAttempt) {
	all.unanswered(ctx, func(attempt heldAttempt) bool {
		each, isHeld := heldByToken[attempt.Token]
		return attempt.NodeID == nodeID && isHeld && each.State != nodeproto.HeldEnded &&
			time.Duration(each.SinceMovedMs)*time.Millisecond >= MovementWindow
	}, func(attempt Attempt) error {
		each := heldByToken[attempt.Token]
		return fmt.Errorf("node %d says %s job %d, %s, has not moved for %s: %w", nodeID, attempt.Kind,
			attempt.JobID, each.State, (time.Duration(each.SinceMovedMs) * time.Millisecond).Round(time.Second),
			ErrNotMoving)
	}, true)
}

// wordOfHeld is the node's word on an attempt its status lists, as its progress would say it, and
// whether it says one: an ended attempt's word is its result.
func wordOfHeld(state nodeproto.HeldState) (nodeproto.ProgressState, bool) {
	switch state {
	case nodeproto.HeldWaiting:
		return nodeproto.ProgressWaiting, true
	case nodeproto.HeldRunning:
		return nodeproto.ProgressRunning, true
	case nodeproto.HeldPaused:
		return nodeproto.ProgressPaused, true
	case nodeproto.HeldEnded:
	}
	return "", false
}

// brokeAnInvariant logs and counts a difference between what the core gave a node and what it holds,
// which is reconciled, never silently (D234 §4).
func (all *Exchanges) brokeAnInvariant(ctx context.Context, nodeID int64, what string, jobID int64, token string) {
	all.mutex.Lock()
	all.brokenInvariants[nodeID]++
	all.mutex.Unlock()
	all.logger.WarnContext(ctx, "a broken invariant", "invariant", what, "nodeId", nodeID, "jobId", jobID,
		"attemptToken", token)
}

// BrokenInvariants is how many differences between what the core gave a node and what it held this
// core has reconciled, for the System page (Phase 6).
func (all *Exchanges) BrokenInvariants(nodeID int64) int {
	all.mutex.Lock()
	defer all.mutex.Unlock()
	return all.brokenInvariants[nodeID]
}
